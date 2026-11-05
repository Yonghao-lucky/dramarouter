package router

import (
	"embed"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")

	router.NoRoute(
		pluginDispatcher,
		middleware.RouteTag("web"),
		gzip.Gzip(gzip.DefaultCompression),
		middleware.GlobalWebRateLimit(),
		middleware.Cache(),
		static.Serve("/", frontendFS),
		func(c *gin.Context) {
			if isBackendRoute(c.Request.URL.Path) {
				c.Header("Cache-Control", "no-store")
				controller.RelayNotFound(c)
				return
			}
			// Hashed asset chunks are immutable build artifacts. Serving index.html for
			// a missing one makes the browser parse HTML as a module and crash the SPA
			// after every redeploy, so asset paths must 404 instead of falling back.
			if strings.HasPrefix(c.Request.URL.Path, "/static/") {
				c.Header("Cache-Control", "no-store")
				c.Status(http.StatusNotFound)
				return
			}
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/html; charset=utf-8", assets.IndexPage)
		},
	)
}

// backendPathPrefixes are API paths that exist without the /v1 prefix. A
// request to one of them must answer with the relay's JSON error instead of the
// SPA shell: an HTML 200 makes client endpoint auto-detection conclude the host
// is a website rather than a gateway. Frontend page routes are deliberately
// excluded (for example /models and /chat are real pages).
var backendPathPrefixes = []string{
	"/chat/completions", "/completions", "/messages", "/responses",
	"/embeddings", "/moderations", "/rerank", "/images/", "/audio/",
	"/fine-tunes",
}

func isBackendRoute(path string) bool {
	if path == "/v1" || strings.HasPrefix(path, "/v1/") || path == "/api" || strings.HasPrefix(path, "/api/") {
		return true
	}
	for _, prefix := range backendPathPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
