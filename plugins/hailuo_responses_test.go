package plugins_test

import (
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay/channel"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHailuoResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "hailuo",
		model:     "MiniMax-Hailuo-2.3",
		requestBody: map[string]any{
			"model": "MiniMax-Hailuo-2.3",
			"input": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "ocean at sunset"},
				map[string]any{"type": "input_image", "image_url": "https://cdn.example/frame.png"},
			}}},
			"seconds": 10,
			"size":    "1920x1080",
		},
		wantAction: "image_to_video",
		wantRequest: map[string]any{
			"model":    "MiniMax-Hailuo-2.3",
			"prompt":   "ocean at sunset",
			"images":   []any{"https://cdn.example/frame.png"},
			"duration": float64(10),
			"size":     "1920x1080",
			"metadata": map[string]any{"first_frame_image": "https://cdn.example/frame.png"},
		},
		wantUsageKeys:  []string{"input_images", "input_video_seconds", "resolution", "seconds"},
		wantVendorName: "hailuo",
	})
}

func TestHailuoArtifactContentProxy(t *testing.T) {
	source, err := builtinplugins.Source("hailuo")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "hailuo"})
	require.NoError(t, err)
	adaptor := taskplugin.New(plugin)
	adaptor.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:         "test-ak",
			ChannelBaseUrl: "https://api.minimax.example",
		},
	})
	data, err := common.Marshal(map[string]any{"file_id": "file/with space"})
	require.NoError(t, err)
	task := &model.Task{TaskID: "task-public", Status: model.TaskStatusSuccess, Data: data}

	artifacts, err := adaptor.ListArtifacts(task)
	require.NoError(t, err)
	assert.Equal(t, []channel.TaskArtifact{{Key: "video", Type: "video", MimeType: "video/mp4"}}, artifacts)

	descriptor, err := adaptor.BuildContentRequest(task, "video", channel.TaskArtifactClientRequest{Method: http.MethodHead})
	require.NoError(t, err)
	require.NotNil(t, descriptor)
	assert.Equal(t, "https://api.minimax.example/v1/files/download?file_id=file%2Fwith%20space", descriptor.URL)
	assert.Equal(t, http.MethodHead, descriptor.Method)
	assert.Equal(t, map[string]string{"Accept": "video/*", "Authorization": "Bearer test-ak"}, descriptor.Headers)
	assert.False(t, descriptor.Credentialless)
}

func loadHailuoPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("hailuo")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "hailuo"})
	require.NoError(t, err)
	return plugin
}

func callHailuoHook(t *testing.T, plugin *jsplugin.LoadedPlugin, hook string, args ...any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), hook, args...)
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	return decoded
}

func hailuoH3SubmitContext(requestBody map[string]any) map[string]any {
	return map[string]any{
		"requestBody":   requestBody,
		"model":         "MiniMax-H3",
		"upstreamModel": "MiniMax-H3",
		"baseUrl":       "https://api.minimax.example",
		"apiKey":        "test-ak",
	}
}

// Input media is a billing dimension for MiniMax-H3 only: every image counts,
// and an input video reserves the documented 15-second maximum at submit.
func TestHailuoH3InputMediaUsageFacts(t *testing.T) {
	plugin := loadHailuoPlugin(t)
	testCases := []struct {
		name    string
		request map[string]any
		want    map[string]any
	}{
		{
			name:    "text only",
			request: map[string]any{"prompt": "p", "duration": 6, "size": "2K"},
			want:    map[string]any{"seconds": float64(6), "resolution": "2K", "input_images": float64(0), "input_video_seconds": float64(0)},
		},
		{
			name:    "two frame images",
			request: map[string]any{"prompt": "p", "images": []any{"a.png", "b.png"}},
			want:    map[string]any{"seconds": float64(5), "resolution": "768P", "input_images": float64(2), "input_video_seconds": float64(0)},
		},
		{
			name:    "reference video",
			request: map[string]any{"prompt": "p", "metadata": map[string]any{"reference_video": "ref.mp4"}},
			want:    map[string]any{"seconds": float64(5), "resolution": "768P", "input_images": float64(0), "input_video_seconds": float64(15)},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := hailuoH3SubmitContext(testCase.request)
			ctx["usagePurpose"] = "facts"
			assert.Equal(t, testCase.want, callHailuoHook(t, plugin, "extractUsage", ctx))
		})
	}
}

func TestHailuoH3SubmitsToV2(t *testing.T) {
	plugin := loadHailuoPlugin(t)
	descriptor := callHailuoHook(t, plugin, "buildSubmitRequest", hailuoH3SubmitContext(map[string]any{"prompt": "a boy playing basketball"}))
	assert.Equal(t, "https://api.minimax.example/v2/video_generation", descriptor["url"])
	assert.Equal(t, "text_to_video", descriptor["action"])
	body, err := common.Marshal(descriptor["body"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"MiniMax-H3","content":[{"type":"text","text":"a boy playing basketball"}],"resolution":"768P","duration":5,"ratio":"16:9"}`, string(body))
}

// Each model family bills only the dimensions it renders, so MiniMax-H3 shows
// its 768P/1080P/2K tiers and input media while the 01 series bills seconds alone.
func TestHailuoUsageProfilesPerModel(t *testing.T) {
	plugin := loadHailuoPlugin(t)
	families := []struct {
		models      []string
		fields      []string
		resolutions []string
	}{
		{[]string{"MiniMax-H3"}, []string{"input_images", "input_video_seconds", "resolution", "seconds"}, []string{"768P", "1080P", "2K"}},
		{[]string{"MiniMax-Hailuo-2.3", "MiniMax-Hailuo-2.3-Fast"}, []string{"resolution", "seconds"}, []string{"768P", "1080P"}},
		{[]string{"MiniMax-Hailuo-02"}, []string{"resolution", "seconds"}, []string{"512P", "768P", "1080P"}},
		{[]string{"T2V-01-Director", "T2V-01", "I2V-01-Director", "I2V-01-live", "I2V-01", "S2V-01"}, []string{"seconds"}, nil},
	}
	profiled := make([]string, 0, len(plugin.Meta.Models))
	for _, family := range families {
		for _, name := range family.models {
			profiled = append(profiled, name)
			t.Run(name, func(t *testing.T) {
				schema, examples := plugin.Meta.UsageForModels(name)
				assert.Equal(t, family.fields, slices.Sorted(maps.Keys(schema)))
				assert.Equal(t, family.resolutions, schema["resolution"].Enum)
				assert.NotEmpty(t, examples)
			})
		}
	}
	assert.ElementsMatch(t, plugin.Meta.Models, profiled, "every declared model must select a usage profile")

	// Lookup is case-insensitive and an undeclared model keeps the defaults.
	schema, _ := plugin.Meta.UsageForModels("minimax-h3")
	assert.Equal(t, []string{"768P", "1080P", "2K"}, schema["resolution"].Enum)
	schema, _ = plugin.Meta.UsageForModels("unknown-model")
	assert.Equal(t, plugin.Meta.UsageSchema, schema)
}
