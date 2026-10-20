package volcnative

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVolcNativeSeedance20RequestResolutionAndVideoBilling(t *testing.T) {
	tiers := []struct {
		resolution string
		noVideo    float64
		withVideo  float64
	}{
		{resolution: "480p", noVideo: 46, withVideo: 28},
		{resolution: "720p", noVideo: 46, withVideo: 28},
		{resolution: "1080p", noVideo: 51, withVideo: 31},
		{resolution: "4k", noVideo: 26, withVideo: 16},
	}
	for _, modelName := range []string{"doubao-seedance-2-0", "doubao-seedance-2-0-260128"} {
		for _, tier := range tiers {
			for _, hasVideo := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/video=%t", modelName, tier.resolution, hasVideo), func(t *testing.T) {
					content := []interface{}{map[string]interface{}{"type": "text", "text": "animate"}}
					wantPrice := tier.noVideo
					if hasVideo {
						content = append(content, map[string]interface{}{"type": "video_url", "video_url": map[string]interface{}{"url": "https://example.com/input.mp4"}})
						wantPrice = tier.withVideo
					}
					data, err := common.Marshal(map[string]interface{}{"model": modelName, "resolution": tier.resolution, "content": content})
					require.NoError(t, err)
					ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
					ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewReader(data))
					info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
					adaptor := &TaskAdaptor{}
					require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
					assert.Equal(t, modelName, info.OriginModelName)
					ratios := adaptor.EstimateBilling(ctx, info)
					if wantPrice == 46 {
						assert.Empty(t, ratios, "base-tier native requests must not add a second price multiplier")
					} else {
						require.Len(t, ratios, 1)
						assert.InDelta(t, wantPrice/46, ratios["video_input"], 1e-12)
					}

					body, err := adaptor.BuildRequestBody(ctx, info)
					require.NoError(t, err)
					upstream, err := io.ReadAll(body)
					require.NoError(t, err)
					assert.Equal(t, data, upstream, "native resolution and media content must be forwarded without generic conversion")
				})
			}
		}
	}
}

func TestVolcNativeSeedance25RejectsUnsupportedPricingTiers(t *testing.T) {
	tests := []struct {
		name       string
		resolution interface{}
		hasVideo   bool
		wantRatio  float64
		invalid    bool
	}{
		{name: "default", wantRatio: 1},
		{name: "480p", resolution: "480p", wantRatio: 1},
		{name: "720p video", resolution: "720p", hasVideo: true, wantRatio: 42.0 / 70},
		{name: "1080p", resolution: "1080p", wantRatio: 77.0 / 70},
		{name: "1080p video", resolution: "1080p", hasVideo: true, wantRatio: 46.0 / 70},
		{name: "unsupported 4k", resolution: "4k", invalid: true},
		{name: "unknown resolution", resolution: "1440p", invalid: true},
		{name: "wrong resolution type", resolution: 1080, invalid: true},
	}
	for _, model := range []string{"doubao-seedance-2-5", "doubao-seedance-2-5-260628"} {
		for _, tt := range tests {
			t.Run(model+"/"+tt.name, func(t *testing.T) {
				body := map[string]interface{}{"model": model, "content": []interface{}{map[string]interface{}{"type": "text", "text": "animate"}}}
				if tt.resolution != nil {
					body["resolution"] = tt.resolution
				}
				if tt.hasVideo {
					body["content"] = []interface{}{map[string]interface{}{"type": "video_url", "video_url": map[string]interface{}{"url": "https://example.com/input.mp4"}}}
				}
				data, err := common.Marshal(body)
				require.NoError(t, err)
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewReader(data))
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
				adaptor := &TaskAdaptor{}
				taskErr := adaptor.ValidateRequestAndSetAction(ctx, info)
				if tt.invalid {
					require.NotNil(t, taskErr)
					assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
					return
				}
				require.Nil(t, taskErr)
				ratio := adaptor.EstimateBilling(ctx, info)["video_input"]
				if ratio == 0 {
					ratio = 1
				}
				assert.InDelta(t, tt.wantRatio, ratio, 1e-12)
			})
		}
	}
}

func TestTaskAdaptorParseResponsePreservesNativeFieldsAndHidesUpstreamID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	raw := []byte(`{"id":"upstream-task-id","status":"queued","model":"doubao-seedance-2-0-260128","stream":false,"sequential_image_generation":true,"watermark":false}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public_id"}}

	parsed, taskErr := (&TaskAdaptor{}).ParseResponse(ctx, resp, info)

	require.Nil(t, taskErr)
	require.NotNil(t, parsed)
	require.Equal(t, "upstream-task-id", parsed.UpstreamTaskID)
	require.Equal(t, raw, parsed.TaskData)
	// Parsing must not write to the client; the controller presents after the durable barrier.
	require.False(t, ctx.Writer.Written())
	clientBody, err := common.Marshal(parsed.ClientResponse)
	require.NoError(t, err)
	require.Contains(t, string(clientBody), `"id":"task_public_id"`)
	require.NotContains(t, string(clientBody), "upstream-task-id")
	require.Contains(t, string(clientBody), `"stream":false`)
	require.Contains(t, string(clientBody), `"sequential_image_generation":true`)
	require.Contains(t, string(clientBody), `"watermark":false`)
}

func TestVolcNativeRequestPreservesBodyAndBillingContext(t *testing.T) {
	raw := `{"model":"doubao-seedance-2-0-260128","content":[{"type":"video_url","video_url":{"url":"https://example.com/input.mp4"},"role":"reference_video"}],"resolution":"1080p","duration":5,"watermark":false,"seed":0,"extra":9007199254740993}`
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(raw))
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
	request, err := relaycommon.GetTaskRequest(ctx)
	require.NoError(t, err)
	assert.Equal(t, false, request.Metadata["watermark"])
	assert.InDelta(t, 31.0/46.0, adaptor.EstimateBilling(ctx, info)["video_input"], 1e-12)
	body, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, raw, string(got))
}

func TestVolcNativeRequestMapsOnlyTopLevelModel(t *testing.T) {
	raw := `{"model":"public-seedance","content":[{"type":"text","text":"hello"}],"watermark":false,"seed":0,"extra":9007199254740993}`
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(raw))
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			IsModelMapped:     true,
			UpstreamModelName: "doubao-seedance-2-0-260128",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))

	body, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"hello"}],"watermark":false,"seed":0,"extra":9007199254740993}`, string(got))
	assert.Equal(t, string(got), common.GetContextKeyString(ctx, constant.ContextKeyVideoTaskUpstreamRequestBody))
}

func TestVolcNativeRequestRecordsCachedBase64StagingConversion(t *testing.T) {
	raw := `{"model":"seedance","content":[{"type":"image_url","image_url":{"url":"data:image/webp;base64,AAAA"}}]}`
	staged := []byte(`{"model":"seedance","content":[{"type":"image_url","image_url":{"url":"https://storage.example.com/staged.webp"}}]}`)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(raw))
	ctx.Set("volc_native_base64_staging:relay_media_temp", stagedRequestBody{body: staged, convertedCount: 1})
	info := &relaycommon.RelayInfo{
		OriginModelName: "seedance",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: relaykitdto.ChannelOtherSettings{
				Base64Staging: &relaykitdto.Base64StagingSettings{Enabled: true},
			},
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	body, err := (&TaskAdaptor{}).BuildRequestBody(ctx, info)
	require.NoError(t, err)
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, staged, got)
	assert.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyTemporaryMediaConverted))
	assert.Equal(t, 1, common.GetContextKeyInt(ctx, constant.ContextKeyTemporaryMediaConvertedCount))
	assert.Equal(t, string(staged), common.GetContextKeyString(ctx, constant.ContextKeyVideoTaskUpstreamRequestBody))
}

func TestVolcNativeRequestStillRejectsParameterOverrides(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"seedance","content":[{"type":"text","text":"hello"}]}`))
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]interface{}{"watermark": true},
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, info)

	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "does not support parameter overrides")
}

func TestTaskAdaptorDoResponseRestoresMappedAlias(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	raw := []byte(`{"id":"upstream-task-id","status":"queued","model":"doubao-seedance-2-0-260128","watermark":false}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "public-seedance",
		ChannelMeta: &relaycommon.ChannelMeta{
			IsModelMapped:     true,
			UpstreamModelName: "doubao-seedance-2-0-260128",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public_id"},
	}

	parsed, taskErr := (&TaskAdaptor{}).ParseResponse(ctx, resp, info)

	require.Nil(t, taskErr)
	require.NotNil(t, parsed)
	assert.Equal(t, "upstream-task-id", parsed.UpstreamTaskID)
	assert.Equal(t, raw, parsed.TaskData)
	clientBody, err := common.Marshal(parsed.ClientResponse)
	require.NoError(t, err)
	assert.Contains(t, string(clientBody), `"model":"public-seedance"`)
	assert.NotContains(t, string(clientBody), "doubao-seedance-2-0-260128")
}

func TestVolcNativeRejectsInvalidContentAndExcessiveDuration(t *testing.T) {
	for _, raw := range []string{`{"model":"seedance"}`, `{"model":"seedance","content":null}`, `{"model":"seedance","content":[]}`, `{"model":"seedance","content":[{"type":"text","text":"hello"}],"duration":18446744073709551615}`} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(raw))
		info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
		err := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, info)
		require.NotNil(t, err)
		assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	}
}

func TestVolcNativePollingRecognizesTerminalStates(t *testing.T) {
	for _, status := range []string{"expired", "cancelled"} {
		body, err := common.Marshal(map[string]any{"status": status})
		require.NoError(t, err)
		result, err := (&TaskAdaptor{}).ParseTaskResult(body)
		require.NoError(t, err)
		assert.Equal(t, string(model.TaskStatusFailure), result.Status)
		assert.Equal(t, status, result.Reason)
	}
}
