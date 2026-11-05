package model

import (
	"errors"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResellerQuotaAlert is one reseller's standing low-balance rule for its owned
// customers. The rule is evaluated by a periodic scan; it never runs inside the
// billing path, so a slow mailbox can never delay a request.
type ResellerQuotaAlert struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	ResellerId     int    `json:"reseller_id" gorm:"uniqueIndex"`
	Enabled        bool   `json:"enabled"`
	ThresholdQuota int64  `json:"threshold_quota" gorm:"bigint"`
	NotifyEmail    string `json:"notify_email" gorm:"type:varchar(128)"`
	// CooldownHours stops a customer that stays below the threshold from
	// mailing the reseller on every scan.
	CooldownHours int   `json:"cooldown_hours"`
	UpdatedAt     int64 `json:"updated_at" gorm:"bigint"`
}

// ResellerQuotaAlertSent is the per-customer watermark of the last alert, keyed
// by the crossing so re-entering the same range after a top-up alerts again.
type ResellerQuotaAlertSent struct {
	Id             int64 `json:"id" gorm:"primaryKey"`
	ResellerId     int   `json:"reseller_id" gorm:"uniqueIndex:idx_reseller_alert_sent,priority:1"`
	CustomerUserId int   `json:"customer_user_id" gorm:"uniqueIndex:idx_reseller_alert_sent,priority:2"`
	LastSentAt     int64 `json:"last_sent_at" gorm:"bigint"`
	LastBalance    int64 `json:"last_balance" gorm:"bigint"`
}

var ErrResellerAlertInvalid = errors.New("invalid quota alert rule")

// GetResellerQuotaAlert returns the rule for one reseller, or a disabled default.
func GetResellerQuotaAlert(resellerId int) (*ResellerQuotaAlert, error) {
	if resellerId <= 0 {
		return nil, ErrResellerAlertInvalid
	}
	rule := ResellerQuotaAlert{ResellerId: resellerId}
	err := DB.Where("reseller_id = ?", resellerId).First(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &rule, nil
	}
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// SetResellerQuotaAlert stores the rule. A zero threshold disables it so an
// operator can turn alerts off without deleting the row.
func SetResellerQuotaAlert(resellerId int, enabled bool, thresholdQuota int64, notifyEmail string, cooldownHours int) (*ResellerQuotaAlert, error) {
	if resellerId <= 0 || thresholdQuota < 0 {
		return nil, ErrResellerAlertInvalid
	}
	if thresholdQuota == 0 {
		enabled = false
	}
	if cooldownHours <= 0 {
		cooldownHours = 24
	}
	if cooldownHours > 24*30 {
		cooldownHours = 24 * 30
	}
	now := common.GetTimestamp()
	rule := ResellerQuotaAlert{
		ResellerId:     resellerId,
		Enabled:        enabled,
		ThresholdQuota: thresholdQuota,
		NotifyEmail:    truncateResellerText(notifyEmail, 128),
		CooldownHours:  cooldownHours,
		UpdatedAt:      now,
	}
	err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "reseller_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "threshold_quota", "notify_email", "cooldown_hours", "updated_at"}),
	}).Create(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// ResellerLowBalanceCustomer is one customer below the rule threshold.
type ResellerLowBalanceCustomer struct {
	UserId      int    `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Group       string `json:"group" gorm:"column:group_name"`
	Balance     int64  `json:"balance_quota"`
}

// ListResellerLowBalanceCustomers returns the owned customers below a
// threshold. It joins on the ownership binding that already has a reseller_id
// index, so the scan stays one indexed range per rule.
func ListResellerLowBalanceCustomers(resellerId int, threshold int64, limit int) ([]ResellerLowBalanceCustomer, error) {
	if resellerId <= 0 || threshold <= 0 {
		return nil, ErrResellerAlertInvalid
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows := make([]ResellerLowBalanceCustomer, 0, limit)
	columns := "users.id AS user_id, users.username, users.display_name, users." + commonGroupCol + " AS group_name, users.quota AS balance_quota"
	err := DB.Model(&User{}).
		Select(columns).
		Joins("JOIN reseller_customer_bindings b ON b.customer_user_id = users.id").
		Where("b.reseller_id = ? AND users.quota < ? AND users.status = ?", resellerId, threshold, common.UserStatusEnabled).
		Order("users.quota asc").
		Limit(limit).
		Scan(&rows).Error
	return rows, err
}

// RunResellerQuotaAlerts evaluates every enabled rule once and mails the
// matching resellers. Delivery failures are logged, never returned: a broken
// mailbox must not stop the scan for other resellers.
func RunResellerQuotaAlerts() {
	if DB == nil {
		return
	}
	rules := make([]ResellerQuotaAlert, 0)
	if err := DB.Where("enabled = ? AND threshold_quota > 0", true).Limit(500).Find(&rules).Error; err != nil {
		common.SysError("failed to load reseller quota alert rules: " + err.Error())
		return
	}
	for _, rule := range rules {
		customers, err := ListResellerLowBalanceCustomers(rule.ResellerId, rule.ThresholdQuota, 100)
		if err != nil {
			common.SysError("failed to scan reseller low balance: " + err.Error())
			continue
		}
		if len(customers) == 0 {
			continue
		}
		email := rule.NotifyEmail
		if email == "" {
			var user User
			if err := DB.Select("email").First(&user, rule.ResellerId).Error; err == nil {
				email = user.Email
			}
		}
		if email == "" {
			continue
		}
		due := make([]ResellerLowBalanceCustomer, 0, len(customers))
		for _, customer := range customers {
			sent := ResellerQuotaAlertSent{ResellerId: rule.ResellerId, CustomerUserId: customer.UserId}
			err := DB.Where("reseller_id = ? AND customer_user_id = ?", rule.ResellerId, customer.UserId).First(&sent).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			cooldown := time.Duration(rule.CooldownHours) * time.Hour
			if sent.LastSentAt > 0 && time.Since(time.Unix(sent.LastSentAt, 0)) < cooldown {
				continue
			}
			due = append(due, customer)
		}
		if len(due) == 0 {
			continue
		}
		if err := common.SendEmail(resellerAlertSubject(), email, resellerAlertBody(due, rule.ThresholdQuota)); err != nil {
			common.SysError("failed to send reseller quota alert: " + err.Error())
			continue
		}
		now := common.GetTimestamp()
		for _, customer := range due {
			_ = DB.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "reseller_id"}, {Name: "customer_user_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"last_sent_at", "last_balance"}),
			}).Create(&ResellerQuotaAlertSent{
				ResellerId: rule.ResellerId, CustomerUserId: customer.UserId,
				LastSentAt: now, LastBalance: customer.Balance,
			}).Error
		}
	}
}

func resellerAlertSubject() string {
	return "Customer balance running low"
}

// resellerAlertBody renders a plain-text-and-table mail body. Every interpolated
// value is HTML-escaped because customer names are user-controlled.
func resellerAlertBody(customers []ResellerLowBalanceCustomer, threshold int64) string {
	var b strings.Builder
	b.WriteString("<p>The following customers are below your low-balance threshold ")
	b.WriteString("(")
	b.WriteString(html.EscapeString(formatQuotaDollars(threshold)))
	b.WriteString("):</p><table cellpadding=\"6\" border=\"1\" style=\"border-collapse:collapse\">")
	b.WriteString("<tr><th>ID</th><th>Username</th><th>Group</th><th>Balance</th></tr>")
	for _, customer := range customers {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(strconv.Itoa(customer.UserId)))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(customer.Username))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(customer.Group))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(formatQuotaDollars(customer.Balance)))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</table>")
	return b.String()
}

func formatQuotaDollars(quota int64) string {
	value := strconv.FormatFloat(float64(quota)/common.QuotaPerUnit, 'f', 4, 64)
	if strings.Contains(value, ".") {
		value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
	}
	return "$" + value
}
