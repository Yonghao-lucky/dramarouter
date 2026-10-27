package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestSelfUserDataCarriesResellerCapability locks the invariant that the one
// dashboard-user DTO used by /api/user/self, login and refresh all report the
// reseller flag. It previously lived only in GetSelf, so a reseller signed in
// and saw no console until something reloaded the user from /api/user/self.
func TestSelfUserDataCarriesResellerCapability(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.ResellerProfile{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	enabled := &model.User{Username: "reseller-enabled", Password: "unused", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "rsl-enabled", AuthVersion: 1}
	disabled := &model.User{Username: "reseller-disabled", Password: "unused", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "rsl-disabled", AuthVersion: 1}
	plain := &model.User{Username: "plain-user", Password: "unused", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "rsl-plain", AuthVersion: 1}
	for _, user := range []*model.User{enabled, disabled, plain} {
		require.NoError(t, db.Create(user).Error)
	}
	require.NoError(t, db.Create(&model.ResellerProfile{UserId: enabled.Id, Status: model.ResellerStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.ResellerProfile{UserId: disabled.Id, Status: model.ResellerStatusDisabled}).Error)

	assert.Equal(t, true, buildSelfUserData(enabled)["reseller_enabled"])
	assert.Equal(t, false, buildSelfUserData(disabled)["reseller_enabled"])
	assert.Equal(t, false, buildSelfUserData(plain)["reseller_enabled"])
}
