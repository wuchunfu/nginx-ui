package upgrader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"code.pfad.fr/risefront"
	_github "github.com/0xJacky/Nginx-UI/.github"
	"github.com/0xJacky/Nginx-UI/internal/helper"
	"github.com/0xJacky/Nginx-UI/internal/version"
	"github.com/minio/selfupdate"
	"github.com/pkg/errors"
	"github.com/uozi-tech/cosy/logger"
)

const (
	UpgradeStatusInfo     = "info"
	UpgradeStatusError    = "error"
	UpgradeStatusProgress = "progress"
)

type CoreUpgradeResp struct {
	Status   string  `json:"status"`
	Progress float64 `json:"progress"`
	Message  string  `json:"message"`
}

type Upgrader struct {
	Channel string
	Release version.TRelease
	version.RuntimeInfo
}

func NewUpgrader(channel string) (u *Upgrader, err error) {
	data, err := version.GetRelease(channel)
	if err != nil {
		return
	}
	runtimeInfo, err := version.GetRuntimeInfo()
	if err != nil {
		return
	}
	u = &Upgrader{
		Channel:     channel,
		Release:     data,
		RuntimeInfo: runtimeInfo,
	}
	return
}

type ProgressWriter struct {
	io.Writer
	totalSize    int64
	currentSize  int64
	progressChan chan<- float64
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.Writer.Write(p)
	pw.currentSize += int64(n)
	progress := float64(pw.currentSize) / float64(pw.totalSize) * 100
	pw.progressChan <- progress
	return n, err
}

func downloadRelease(url string, dir string, progressChan chan float64) (tarName string, err error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	totalSize, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		return
	}

	file, err := os.CreateTemp(dir, "nginx-ui-temp-*.tar.gz")
	if err != nil {
		err = errors.Wrap(err, "service.DownloadLatestRelease CreateTemp error")
		return
	}
	defer file.Close()

	progressWriter := &ProgressWriter{Writer: file, totalSize: totalSize, progressChan: progressChan}
	multiWriter := io.MultiWriter(progressWriter)

	_, err = io.Copy(multiWriter, resp.Body)

	tarName = file.Name()
	return
}

func (u *Upgrader) DownloadLatestRelease(progressChan chan float64) (tarName string, err error) {
	bytes, err := _github.DistFS.ReadFile("build/build_info.json")
	if err != nil {
		err = errors.Wrap(err, "service.DownloadLatestRelease Read build_info.json error")
		return
	}
	type buildArch struct {
		Arch string `json:"arch"`
		Name string `json:"name"`
	}
	var buildJson map[string]map[string]buildArch

	_ = json.Unmarshal(bytes, &buildJson)

	build, ok := buildJson[u.OS]
	if !ok {
		err = errors.Wrap(err, "os not support upgrade")
		return
	}
	arch, ok := build[u.Arch]
	if !ok {
		err = errors.Wrap(err, "arch not support upgrade")
		return
	}

	assetsMap := u.Release.GetAssetsMap()

	// asset
	asset, ok := assetsMap[fmt.Sprintf("nginx-ui-%s.tar.gz", arch.Name)]

	if !ok {
		err = errors.Wrap(err, "upgrader core asset is empty")
		return
	}

	downloadUrl := asset.BrowserDownloadUrl
	if downloadUrl == "" {
		err = errors.New("upgrader core downloadUrl is empty")
		return
	}

	// digest
	digest, ok := assetsMap[fmt.Sprintf("nginx-ui-%s.tar.gz.digest", arch.Name)]
	if !ok || digest.BrowserDownloadUrl == "" {
		err = errors.New("upgrader core digest is empty")
		return
	}

	if u.Channel != string(version.ReleaseTypeDev) {
		digest.BrowserDownloadUrl = version.GetUrl(digest.BrowserDownloadUrl)
	}

	resp, err := http.Get(digest.BrowserDownloadUrl)
	if err != nil {
		err = errors.Wrap(err, "upgrader core download digest fail")
		return
	}

	defer resp.Body.Close()

	dir := filepath.Dir(u.ExPath)

	if u.Channel != string(version.ReleaseTypeDev) {
		downloadUrl = version.GetUrl(downloadUrl)
	}

	tarName, err = downloadRelease(downloadUrl, dir, progressChan)
	if err != nil {
		err = errors.Wrap(err, "service.DownloadLatestRelease downloadFile error")
		return
	}

	// check tar digest
	digestFileBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		err = errors.Wrap(err, "digest file content read error")
		return
	}

	digestFileContent := strings.TrimSpace(string(digestFileBytes))

	logger.Debug("DownloadLatestRelease tar digest", helper.DigestSHA512(tarName))
	logger.Debug("DownloadLatestRelease digestFileContent", digestFileContent)

	if digestFileContent == "" {
		err = errors.New("digest file content is empty")
		return
	}

	exeSHA512 := helper.DigestSHA512(tarName)
	if exeSHA512 == "" {
		err = errors.New("executable binary file is empty")
		return
	}

	if digestFileContent != exeSHA512 {
		err = errors.Wrap(err, "digest not equal")
		return
	}

	return
}

var updateInProgress atomic.Bool

func (u *Upgrader) PerformCoreUpgrade(tarPath string) (err error) {
	if !updateInProgress.CompareAndSwap(false, true) {
		return errors.New("update already in progress")
	}
	defer updateInProgress.Store(false)

	oldExe := ""
	if runtime.GOOS == "windows" {
		oldExe = filepath.Join(filepath.Dir(u.ExPath), ".nginx-ui.old."+strconv.FormatInt(time.Now().Unix(), 10))
	}

	opts := selfupdate.Options{
		OldSavePath: oldExe,
	}

	if err = opts.CheckPermissions(); err != nil {
		return err
	}

	tempDir, err := os.MkdirTemp("", "nginx-ui-upgrade-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	err = helper.UnTar(tempDir, tarPath)
	if err != nil {
		err = errors.Wrap(err, "PerformCoreUpgrade unTar error")
		return
	}

	nginxUIExName := "nginx-ui"

	if u.OS == "windows" {
		nginxUIExName = "nginx-ui.exe"
	}

	f, err := os.Open(filepath.Join(tempDir, nginxUIExName))
	if err != nil {
		err = errors.Wrap(err, "PerformCoreUpgrade open error")
		return
	}
	defer f.Close()

	if err = selfupdate.PrepareAndCheckBinary(f, opts); err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			return pathErr.Err
		}
		return err
	}

	if err = selfupdate.CommitBinary(opts); err != nil {
		if rerr := selfupdate.RollbackError(err); rerr != nil {
			return rerr
		}
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			return pathErr.Err
		}
		return err
	}

	// wait for the file to be written
	time.Sleep(1 * time.Second)

	// gracefully restart
	risefront.Restart()
	return
}
