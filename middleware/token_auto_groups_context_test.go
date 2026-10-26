package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTokenAutoGroupsContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}

func TestSetupContextForTokenPreservesCustomAutoGroupsOrder(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	token := &model.Token{Id: 1, UserId: 2, AutoGroups: `["vip","default"]`}

	require.NoError(t, SetupContextForToken(ctx, token))
	value, ok := common.GetContextKey(ctx, constant.ContextKeyTokenAutoGroups)
	require.True(t, ok)
	assert.Equal(t, []string{"vip", "default"}, value)
}

func TestSetupContextForTokenTreatsStoredEmptyArrayAsInheritance(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	token := &model.Token{Id: 1, UserId: 2, AutoGroups: `[]`}

	require.NoError(t, SetupContextForToken(ctx, token))
	_, ok := common.GetContextKey(ctx, constant.ContextKeyTokenAutoGroups)
	assert.False(t, ok)
}

func TestSetupContextForTokenMalformedAutoGroupsFailsClosed(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	token := &model.Token{Id: 1, UserId: 2, AutoGroups: `not-json`}

	require.NoError(t, SetupContextForToken(ctx, token))
	value, ok := common.GetContextKey(ctx, constant.ContextKeyTokenAutoGroups)
	require.True(t, ok)
	assert.Equal(t, []string{}, value)
}

// 只配置厂商顺序的聚合 Key 必须携带空 Auto 快照。否则 GetRequestAutoGroups 会
// 认为 Key 继承全局 Auto 列表，模型列表和路由都会扩散到 Key 未选择的分组。
func TestSetupContextForTokenVendorOnlyTokenDisablesGlobalAutoFallback(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	token := &model.Token{
		Id:               1,
		UserId:           2,
		Group:            "auto",
		VendorAutoGroups: `[{"vendor_id":9,"groups":["moonshot-a"]}]`,
	}

	require.NoError(t, SetupContextForToken(ctx, token))
	value, ok := common.GetContextKey(ctx, constant.ContextKeyTokenAutoGroups)
	require.True(t, ok, "vendor-only token must publish an explicit empty Auto snapshot")
	assert.Equal(t, []string{}, value)
}

func TestSetupContextForTokenVendorOrdersKeepStoredAutoGroups(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	token := &model.Token{
		Id:               1,
		UserId:           2,
		Group:            "auto",
		AutoGroups:       `["vip"]`,
		VendorAutoGroups: `[{"vendor_id":9,"groups":["moonshot-a"]}]`,
	}

	require.NoError(t, SetupContextForToken(ctx, token))
	value, ok := common.GetContextKey(ctx, constant.ContextKeyTokenAutoGroups)
	require.True(t, ok)
	assert.Equal(t, []string{"vip"}, value)
}

func TestSetupContextForTokenWithoutVendorOrdersInheritsGlobalAuto(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	token := &model.Token{Id: 1, UserId: 2, Group: "auto"}

	require.NoError(t, SetupContextForToken(ctx, token))
	_, ok := common.GetContextKey(ctx, constant.ContextKeyTokenAutoGroups)
	assert.False(t, ok, "legacy tokens must keep inheriting the global Auto list")
}
