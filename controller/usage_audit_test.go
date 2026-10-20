package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUsageAuditRequiresAdministratorAndFinancialPermission(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	setupBillingPermissions(t, db)
	require.NoError(t, authz.SetUserPermissions(2, authz.PermissionsMap{authz.ResourceFinancialAccounting: {authz.ActionFinancialView: false}}))
	require.NoError(t, authz.SetUserPermissions(3, authz.PermissionsMap{authz.ResourceFinancialAccounting: {authz.ActionFinancialView: true}}))
	for _, scenario := range []struct{ user, role, status int }{
		{1, common.RoleAdminUser, http.StatusBadRequest},
		{2, common.RoleAdminUser, http.StatusForbidden},
		{3, common.RoleCommonUser, http.StatusForbidden},
		{4, common.RoleRootUser, http.StatusBadRequest},
	} {
		for _, endpoint := range []struct {
			handler       gin.HandlerFunc
			invalidStatus int
		}{
			{QueryUsageAudit, http.StatusBadRequest}, {SearchUsageAuditKeys, http.StatusBadRequest}, {DownloadUsageAuditPDF, http.StatusConflict},
		} {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("id", scenario.user)
			ctx.Set("role", scenario.role)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/?user_id=-1", strings.NewReader(`{"start_at":-1}`))
			endpoint.handler(ctx)
			want := scenario.status
			if scenario.status == http.StatusBadRequest {
				want = endpoint.invalidStatus
			}
			assert.Equal(t, want, recorder.Code)
		}
	}
}
