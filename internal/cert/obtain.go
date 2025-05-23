package cert

import (
	"github.com/0xJacky/Nginx-UI/internal/translation"
	"github.com/0xJacky/Nginx-UI/model"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/pkg/errors"
)

func obtain(payload *ConfigPayload, client *lego.Client, l *Logger, errChan chan error) {
	request := certificate.ObtainRequest{
		Domains:    payload.ServerName,
		Bundle:     true,
		MustStaple: payload.MustStaple,
	}

	l.Info(translation.C("[Nginx UI] Obtaining certificate"))
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		errChan <- errors.Wrap(err, "obtain certificate error")
		return
	}
	payload.Resource = &model.CertificateResource{
		Resource:          certificates,
		PrivateKey:        certificates.PrivateKey,
		Certificate:       certificates.Certificate,
		IssuerCertificate: certificates.IssuerCertificate,
		CSR:               certificates.CSR,
	}

	payload.WriteFile(l, errChan)
}
