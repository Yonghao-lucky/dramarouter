package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsBackendRouteAllowsAssetLibrarySPARefresh(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{path: "/assets", expected: false},
		{path: "/assets/", expected: false},
		{path: "/api/asset-library/assets", expected: true},
		{path: "/v1/chat/completions", expected: true},
		{path: "/dashboard/overview", expected: false},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			assert.Equal(t, test.expected, isBackendRoute(test.path))
		})
	}
}

func TestMissingBackendRoutesAreNotCachedAsFrontendAssets(t *testing.T) {
	engine := gin.New()
	SetWebRouter(engine, WebAssets{IndexPage: []byte("<html>app</html>")}, func(c *gin.Context) { c.Next() })
	for _, path := range []string{"/api/docs", "/v1/missing"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusNotFound, recorder.Code)
			assert.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
		})
	}
}
