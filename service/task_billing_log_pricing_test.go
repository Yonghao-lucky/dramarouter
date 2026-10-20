package service

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskConsumptionLogPreservesStructuredPricingRatios(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		ratios map[string]float64
		want   map[string]float64
	}{
		{name: "Seedance 2.0 1080p", model: "doubao-seedance-2-0", ratios: map[string]float64{"video_input": 51.0 / 46}, want: map[string]float64{"video_input": 51.0 / 46}},
		{name: "Seedance 2.0 dated 4k video", model: "doubao-seedance-2-0-260128", ratios: map[string]float64{"video_input": 16.0 / 46}, want: map[string]float64{"video_input": 16.0 / 46}},
		{name: "Seedance 2.5 1080p", model: "doubao-seedance-2-5", ratios: map[string]float64{"video_input": 77.0 / 70}, want: map[string]float64{"video_input": 77.0 / 70}},
		{name: "Seedance 2.5 dated 1080p video", model: "doubao-seedance-2-5-260628", ratios: map[string]float64{"video_input": 46.0 / 70}, want: map[string]float64{"video_input": 46.0 / 70}},
		{name: "base tier", model: "doubao-seedance-2-5", want: map[string]float64{}},
		{name: "explicit unit multiplier", model: "doubao-seedance-2-0", ratios: map[string]float64{"video_input": 1}, want: map[string]float64{"video_input": 1}},
		{name: "invalid multipliers", model: "doubao-seedance-2-5", ratios: map[string]float64{"zero": 0, "negative": -1, "nan": math.NaN(), "infinite": math.Inf(1)}, want: map[string]float64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 41, 1000)
			seedChannel(t, 8)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", nil)
			ctx.Set("username", "test_user")
			ctx.Set("token_name", "test_token")
			info := &relaycommon.RelayInfo{
				UserId: 41, UsingGroup: "default", OriginModelName: tt.model,
				PriceData:     types.PriceData{Quota: 123456, ModelPrice: -1, ModelRatio: 5, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: .86}},
				ChannelMeta:   &relaycommon.ChannelMeta{ChannelId: 8, ChannelType: constant.ChannelTypeDoubaoVideo},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: constant.TaskActionGenerate, PublicTaskID: "task_pricing_log"},
			}
			for name, ratio := range tt.ratios {
				info.PriceData.AddOtherRatio(name, ratio)
			}
			LogTaskConsumption(ctx, info, nil)

			var log model.Log
			require.NoError(t, model.DB.Order("id desc").First(&log).Error)
			var other struct {
				IsTask      bool               `json:"is_task"`
				OtherRatios map[string]float64 `json:"other_ratios"`
			}
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
			assert.True(t, other.IsTask)
			require.NotNil(t, other.OtherRatios, "unit pricing must serialize an empty object, not omit the pricing snapshot")
			assert.Equal(t, tt.want, other.OtherRatios, "preserve full multiplier precision instead of the rounded Content text")
			assert.Equal(t, 123456, log.Quota)
			assert.Equal(t, 123456, info.PriceData.Quota)
			var user model.User
			require.NoError(t, model.DB.First(&user, 41).Error)
			assert.Equal(t, 1000, user.Quota, "writing pricing metadata must not debit the wallet")
		})
	}
}

func TestTaskSettlementLogPreservesStructuredPricingAndLegacyRatios(t *testing.T) {
	tests := []struct {
		name string
		bc   *model.TaskBillingContext
		want map[string]float64
	}{
		{name: "Seedance 2.0", bc: &model.TaskBillingContext{OriginModelName: "doubao-seedance-2-0", OtherRatios: map[string]float64{"video_input": 31.0 / 46}}, want: map[string]float64{"video_input": 31.0 / 46}},
		{name: "Seedance 2.5", bc: &model.TaskBillingContext{OriginModelName: "doubao-seedance-2-5", OtherRatios: map[string]float64{"video_input": 77.0 / 70}}, want: map[string]float64{"video_input": 77.0 / 70}},
		{name: "base tier", bc: &model.TaskBillingContext{}, want: map[string]float64{}},
		{name: "missing legacy context", want: map[string]float64{}},
		{name: "filters invalid historical multipliers", bc: &model.TaskBillingContext{OtherRatios: map[string]float64{"seconds": 2, "identity": 1, "zero": 0, "negative": -1, "nan": math.NaN(), "infinite": math.Inf(1)}}, want: map[string]float64{"seconds": 2, "identity": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &model.Task{Quota: 123456, PrivateData: model.TaskPrivateData{BillingContext: tt.bc}}
			other := taskBillingOther(task)
			assert.Equal(t, true, other["is_task"])
			ratios, ok := other["other_ratios"].(map[string]float64)
			require.True(t, ok, "completion and refund logs must use the same structured pricing contract as submission and correction logs")
			require.NotNil(t, ratios)
			assert.Equal(t, tt.want, ratios)
			for name, ratio := range tt.want {
				assert.Equal(t, ratio, other[name], "retain legacy top-level ratios for existing consumers")
			}
			for _, name := range []string{"zero", "negative", "nan", "infinite"} {
				assert.NotContains(t, other, name)
			}
			_, err := common.Marshal(other)
			assert.NoError(t, err, "invalid historical multipliers must not make log JSON unwritable")
			assert.Equal(t, 123456, task.Quota)
		})
	}
}
