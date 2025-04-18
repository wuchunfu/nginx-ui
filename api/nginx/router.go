package nginx

import (
	"github.com/0xJacky/Nginx-UI/api/nginx_log"
	"github.com/gin-gonic/gin"
)

func InitRouter(r *gin.RouterGroup) {
	r.POST("ngx/build_config", BuildNginxConfig)
	r.POST("ngx/tokenize_config", TokenizeNginxConfig)
	r.POST("ngx/format_code", FormatNginxConfig)
	r.POST("nginx/reload", Reload)
	r.POST("nginx/restart", Restart)
	r.POST("nginx/test", Test)
	r.GET("nginx/status", Status)
	r.POST("nginx_log", nginx_log.GetNginxLogPage)
	r.GET("nginx/directives", GetDirectives)
}
