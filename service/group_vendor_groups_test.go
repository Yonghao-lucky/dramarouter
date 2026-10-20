package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestGetRequestVendorGroups verifies that a token's per-vendor order wins over
// the flat Auto list, and that every fallback path stays identical to the
// pre-existing behaviour (important: old tokens must not change).
func TestGetRequestVendorGroups(t *testing.T) {
	originalDB := model.DB
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalMaxTokenAutoGroups := setting.GetMaxTokenAutoGroups()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Vendor{}, &model.Ability{}, &model.Channel{}))
	model.DB = db

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","volc":"Volc","official":"Official"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"volc":1,"official":1}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("5"))

	t.Cleanup(func() {
		model.DB = originalDB
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", originalMaxTokenAutoGroups)))
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	vendor := &model.Vendor{Name: "Anthropic", Status: 1}
	require.NoError(t, db.Create(vendor).Error)
	require.NoError(t, db.Create(&model.Model{
		ModelName: "vendor-groups-test-model",
		VendorID:  vendor.Id,
		Status:    1,
		NameRule:  model.NameRuleExact,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "volc",
		Model:     "vendor-groups-test-model",
		ChannelId: 9001,
		Enabled:   true,
	}).Error)
	// Force the pricing cache to rebuild from the seeded fixtures.
	model.InvalidatePricingCache()
	require.Equal(t, vendor.Id, model.GetModelVendorID("vendor-groups-test-model"))

	gin.SetMode(gin.TestMode)
	newContext := func() *gin.Context {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"default", "volc"})
		return ctx
	}

	t.Run("vendor order wins over the flat list", func(t *testing.T) {
		ctx := newContext()
		common.SetContextKey(ctx, constant.ContextKeyTokenVendorAutoGroups, []model.VendorAutoGroupOrder{
			{VendorId: vendor.Id, Groups: []string{"volc", "official"}},
		})
		assert.Equal(t,
			[]string{"volc", "official"},
			GetRequestVendorGroups(ctx, "default", "vendor-groups-test-model"),
		)
	})

	t.Run("vendor without a configured order falls back to the flat list", func(t *testing.T) {
		ctx := newContext()
		common.SetContextKey(ctx, constant.ContextKeyTokenVendorAutoGroups, []model.VendorAutoGroupOrder{
			{VendorId: vendor.Id + 1000, Groups: []string{"official"}},
		})
		assert.Equal(t,
			[]string{"default", "volc"},
			GetRequestVendorGroups(ctx, "default", "vendor-groups-test-model"),
		)
	})

	t.Run("unresolved vendor falls back to the flat list", func(t *testing.T) {
		ctx := newContext()
		common.SetContextKey(ctx, constant.ContextKeyTokenVendorAutoGroups, []model.VendorAutoGroupOrder{
			{VendorId: vendor.Id, Groups: []string{"volc", "official"}},
		})
		assert.Equal(t,
			[]string{"default", "volc"},
			GetRequestVendorGroups(ctx, "default", "unknown-model-without-vendor"),
		)
	})

	t.Run("groups the user can no longer access fall back to the flat list", func(t *testing.T) {
		ctx := newContext()
		common.SetContextKey(ctx, constant.ContextKeyTokenVendorAutoGroups, []model.VendorAutoGroupOrder{
			{VendorId: vendor.Id, Groups: []string{"revoked-group"}},
		})
		assert.Equal(t,
			[]string{"default", "volc"},
			GetRequestVendorGroups(ctx, "default", "vendor-groups-test-model"),
		)
	})

	t.Run("token without vendor orders keeps the legacy path", func(t *testing.T) {
		ctx := newContext()
		assert.Equal(t,
			[]string{"default", "volc"},
			GetRequestVendorGroups(ctx, "default", "vendor-groups-test-model"),
		)
	})
}
