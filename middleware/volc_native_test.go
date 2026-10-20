package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

// channelServesPath mirrors the request-path filter Distribute installs for
// every request; Volc Native isolation is enforced by that shared predicate.
func channelServesPath(channel *model.Channel, requestPath string, requestModel string) bool {
	ok, _ := model.ChannelSatisfiesFilters(channel, requestModel, []dto.ChannelFilter{{
		Kind:        dto.FilterRequestPath,
		RequestPath: playgroundRelayRequestPath(requestPath),
	}})
	return ok
}

func TestChannelSupportsRequestPathVolcNativeOnlyAllowsNativeRoutes(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeVolcNative}

	require.True(t, channelServesPath(channel, "/api/v3/images/generations", "doubao-seedream-4-0"))
	require.True(t, channelServesPath(channel, "/api/v3/contents/generations/tasks", "doubao-seedance-2-0-260128"))
	require.False(t, channelServesPath(channel, "/v1/chat/completions", "doubao-seed-1-6"))
	require.False(t, channelServesPath(channel, "/v1/images/generations", "doubao-seedream-4-0"))
}

func TestVolcNativeRouteRejectsExistingChannelsAndPrefixLookalikes(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine, constant.ChannelTypeOpenAI} {
		require.False(t, channelServesPath(&model.Channel{Type: channelType}, "/api/v3/contents/generations/tasks", "seedance"))
	}
	require.False(t, channelServesPath(&model.Channel{Type: constant.ChannelTypeVolcNative}, "/api/v3/images/generations-invalid", "seedream"))
}
