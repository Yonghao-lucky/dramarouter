package helper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type responsesFailingWriter struct {
	gin.ResponseWriter
	err    error
	writes int
}

func (w *responsesFailingWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, w.err
}

func TestStringDataPreservesEventFraming(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	require.NoError(t, StringData(c, "first\rsecond"))
	assert.Equal(t, "data: first\\rsecond\n\n", w.Body.String())
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
}

func TestWriteResponsesStreamErrorUsesResponsesSchema(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	require.NoError(t, WriteResponsesStreamError(c, "upstream_stream_incomplete", "The upstream response stream ended before completion.", 9))
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.True(t, strings.HasPrefix(w.Body.String(), "event: error\ndata: "))
	assert.True(t, strings.HasSuffix(w.Body.String(), "\n\n"))
	var payload map[string]any
	require.NoError(t, common.UnmarshalJsonStr(strings.TrimSpace(strings.TrimPrefix(w.Body.String(), "event: error\ndata: ")), &payload))
	assert.Equal(t, map[string]any{
		"type":            "error",
		"code":            "upstream_stream_incomplete",
		"message":         "The upstream response stream ended before completion.",
		"param":           nil,
		"sequence_number": float64(9),
	}, payload)
}
