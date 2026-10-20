package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Reseller allocation states. A reservation is open while a request is in
// flight, then becomes settled (paid at the actual retail charge) or released
// (the request failed and nothing was consumed).
const (
	ResellerReservationOpen     = "open"
	ResellerReservationSettled  = "settled"
	ResellerReservationReleased = "released"
)

var (
	// ErrResellerAllocationExhausted means the customer has no pool credit left.
	// The caller should fall back to the customer's own wallet.
	ErrResellerAllocationExhausted = errors.New("reseller allocation exhausted")
	// ErrResellerWalletInsufficient means the reseller cannot fund the pool.
	ErrResellerWalletInsufficient = errors.New("reseller wallet insufficient")
	// ErrResellerOwnershipChanged means the customer moved to another owner
	// while a request was being priced against the old owner.
	ErrResellerOwnershipChanged = errors.New("reseller ownership changed")
)

// ResellerAllocationReservation is the per-request record that keeps pool
// accounting idempotent. RequestID is the relay request id, so a retried
// pre-consume for the same request never reserves twice.
type ResellerAllocationReservation struct {
	RequestID      string `json:"request_id" gorm:"type:varchar(128);primaryKey"`
	CustomerUserId int    `json:"customer_user_id" gorm:"index:idx_reseller_reservation_customer,priority:1"`
	OwnerId        int    `json:"owner_id" gorm:"index:idx_reseller_reservation_owner,priority:1"`
	RetailQuota    int64  `json:"retail_quota" gorm:"bigint"`
	WholesaleQuota int64  `json:"wholesale_quota" gorm:"bigint"`
	State          string `json:"state" gorm:"type:varchar(16);index:idx_reseller_reservation_customer,priority:2"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint"`
}

// reservationEventKey is the server-owned ledger event key. Long request ids
// are hashed so the varchar(128) unique index can never be overflowed.
func reservationEventKey(action, requestID string) string {
	key := "reseller:" + action + ":" + requestID
	if len(key) <= 128 {
		return key
	}
	sum := sha256.Sum256([]byte(requestID))
	return "reseller:" + action + ":h:" + hex.EncodeToString(sum[:16])
}

// lockResellerAllocation returns the customer allocation row under a row lock,
// creating it first so a concurrent first request cannot miss it.
func lockResellerAllocation(tx *gorm.DB, customerUserId int) (*ResellerCustomerAllocation, error) {
	allocation := ResellerCustomerAllocation{CustomerUserId: customerUserId}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&allocation).Error; err != nil {
		return nil, err
	}
	if err := lockForUpdate(tx).Where("customer_user_id = ?", customerUserId).First(&allocation).Error; err != nil {
		return nil, err
	}
	return &allocation, nil
}

// ResellerAllocationAffordable reports whether the customer's pool can cover a
// charge. It is the cheap read used before a request decides to bill the pool.
func ResellerAllocationAffordable(customerUserId int, retailQuota int64) (bool, error) {
	if retailQuota <= 0 {
		return true, nil
	}
	if DB == nil {
		return false, nil
	}
	allocation := ResellerCustomerAllocation{CustomerUserId: customerUserId}
	err := DB.Where("customer_user_id = ?", customerUserId).First(&allocation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return allocation.AvailableQuota() >= retailQuota, nil
}

// chargeResellerWallet moves the owner wallet and appends the ledger evidence
// for one pool movement. Callers must already hold the owner row lock.
func chargeResellerWallet(tx *gorm.DB, owner *User, delta int64, action, requestID string) error {
	if delta == 0 {
		return nil
	}
	if err := changeBillingWallet(tx, owner, delta); err != nil {
		return err
	}
	entry := BillingEntry{
		EventKey:    reservationEventKey(action, requestID),
		Kind:        "funding",
		WalletDelta: delta,
		RequestID:   requestID,
	}
	return appendBillingEntry(tx, owner, &entry)
}

// ReserveResellerAllocation reserves retail quota from the customer's pool and
// charges the owner wallet the matching wholesale amount in one transaction.
// It returns ok=false (and no error) when the pool or the owner wallet cannot
// cover the request, so the caller can fall back to the customer's own wallet.
func ReserveResellerAllocation(customerUserId, ownerId int, requestID string, retailQuota int64, retailRatio, costRatio float64) (int64, bool, error) {
	if retailQuota <= 0 {
		return 0, true, nil
	}
	if retailQuota > int64(common.MaxQuota) {
		return 0, false, errors.New("reseller reservation exceeds the single-request bound")
	}
	wholesale := ResellerWholesaleQuota(retailQuota, retailRatio, costRatio)
	var result int64
	var reserved bool

	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing ResellerAllocationReservation
		err := tx.Where("request_id = ?", requestID).First(&existing).Error
		if err == nil {
			result, reserved = existing.WholesaleQuota, true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var binding ResellerCustomerBinding
		if err := tx.Where("customer_user_id = ?", customerUserId).First(&binding).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				reserved = false
				return nil
			}
			return err
		}
		if binding.ResellerId != ownerId {
			return ErrResellerOwnershipChanged
		}

		allocation, err := lockResellerAllocation(tx, customerUserId)
		if err != nil {
			return err
		}
		if allocation.AvailableQuota() < retailQuota {
			reserved = false
			return nil
		}

		var owner User
		if err := lockForUpdate(tx).First(&owner, ownerId).Error; err != nil {
			return err
		}
		if int64(owner.Quota) < wholesale {
			return ErrResellerWalletInsufficient
		}
		if err := chargeResellerWallet(tx, &owner, -wholesale, "reserve", requestID); err != nil {
			return err
		}
		now := common.GetTimestamp()
		allocation.ReservedQuota += retailQuota
		allocation.UpdatedAt = now
		if err := tx.Model(allocation).Updates(map[string]any{
			"reserved_quota": allocation.ReservedQuota,
			"updated_at":     now,
		}).Error; err != nil {
			return err
		}
		reservation := ResellerAllocationReservation{
			RequestID:      requestID,
			CustomerUserId: customerUserId,
			OwnerId:        ownerId,
			RetailQuota:    retailQuota,
			WholesaleQuota: wholesale,
			State:          ResellerReservationOpen,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&reservation).Error; err != nil {
			return err
		}
		result, reserved = wholesale, true
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	if reserved {
		_ = invalidateUserCache(ownerId)
	}
	return result, reserved, nil
}

// ExtendResellerAllocation grows an open reservation, used when a streaming
// request reserves additional quota before settling.
func ExtendResellerAllocation(requestID string, additionalRetail int64, retailRatio, costRatio float64) error {
	if additionalRetail <= 0 {
		return nil
	}
	if additionalRetail > int64(common.MaxQuota) {
		return errors.New("reseller reservation exceeds the single-request bound")
	}
	additionalWholesale := ResellerWholesaleQuota(additionalRetail, retailRatio, costRatio)
	var ownerId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var reservation ResellerAllocationReservation
		if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&reservation).Error; err != nil {
			return err
		}
		if reservation.State != ResellerReservationOpen {
			return nil
		}
		ownerId = reservation.OwnerId
		allocation, err := lockResellerAllocation(tx, reservation.CustomerUserId)
		if err != nil {
			return err
		}
		if allocation.AvailableQuota() < additionalRetail {
			return ErrResellerAllocationExhausted
		}
		var owner User
		if err := lockForUpdate(tx).First(&owner, reservation.OwnerId).Error; err != nil {
			return err
		}
		if int64(owner.Quota) < additionalWholesale {
			return ErrResellerWalletInsufficient
		}
		if err := chargeResellerWallet(tx, &owner, -additionalWholesale, "extend", requestID+":"+strconv.FormatInt(reservation.RetailQuota+additionalRetail, 10)); err != nil {
			return err
		}
		now := common.GetTimestamp()
		allocation.ReservedQuota += additionalRetail
		allocation.UpdatedAt = now
		if err := tx.Model(allocation).Updates(map[string]any{
			"reserved_quota": allocation.ReservedQuota,
			"updated_at":     now,
		}).Error; err != nil {
			return err
		}
		reservation.RetailQuota += additionalRetail
		reservation.WholesaleQuota += additionalWholesale
		reservation.UpdatedAt = now
		return tx.Model(&reservation).Updates(map[string]any{
			"retail_quota":    reservation.RetailQuota,
			"wholesale_quota": reservation.WholesaleQuota,
			"updated_at":      now,
		}).Error
	})
	if err == nil && ownerId > 0 {
		_ = invalidateUserCache(ownerId)
	}
	return err
}

// SettleResellerAllocation converts a reservation into consumption at the
// actual retail charge and charges or refunds the wholesale difference.
func SettleResellerAllocation(requestID string, actualRetailQuota int64, retailRatio, costRatio float64) error {
	if actualRetailQuota < 0 {
		actualRetailQuota = 0
	}
	var ownerId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var reservation ResellerAllocationReservation
		err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&reservation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if reservation.State != ResellerReservationOpen {
			return nil
		}
		ownerId = reservation.OwnerId
		allocation, err := lockResellerAllocation(tx, reservation.CustomerUserId)
		if err != nil {
			return err
		}
		newWholesale := ResellerWholesaleQuota(actualRetailQuota, retailRatio, costRatio)
		delta := newWholesale - reservation.WholesaleQuota
		now := common.GetTimestamp()

		reserved := allocation.ReservedQuota - reservation.RetailQuota
		if reserved < 0 {
			reserved = 0
		}
		allocation.ReservedQuota = reserved
		allocation.UsedQuota += actualRetailQuota
		allocation.UpdatedAt = now
		if err := tx.Model(allocation).Updates(map[string]any{
			"reserved_quota": allocation.ReservedQuota,
			"used_quota":     allocation.UsedQuota,
			"updated_at":     now,
		}).Error; err != nil {
			return err
		}

		// A settle difference is never refused: the request already ran, so the
		// owner wallet absorbs it exactly like a wallet post-settle overdraft.
		var owner User
		if err := lockForUpdate(tx).First(&owner, reservation.OwnerId).Error; err != nil {
			return err
		}
		if err := chargeResellerWallet(tx, &owner, -delta, "settle", requestID); err != nil {
			return err
		}
		return tx.Model(&reservation).Updates(map[string]any{
			"state":           ResellerReservationSettled,
			"retail_quota":    actualRetailQuota,
			"wholesale_quota": newWholesale,
			"updated_at":      now,
		}).Error
	})
	if err == nil && ownerId > 0 {
		_ = invalidateUserCache(ownerId)
	}
	return err
}

// ReleaseResellerAllocation reverses an open reservation without consumption.
func ReleaseResellerAllocation(requestID string) error {
	var ownerId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var reservation ResellerAllocationReservation
		err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&reservation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if reservation.State != ResellerReservationOpen {
			return nil
		}
		ownerId = reservation.OwnerId
		allocation, err := lockResellerAllocation(tx, reservation.CustomerUserId)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		reserved := allocation.ReservedQuota - reservation.RetailQuota
		if reserved < 0 {
			reserved = 0
		}
		allocation.ReservedQuota = reserved
		allocation.UpdatedAt = now
		if err := tx.Model(allocation).Updates(map[string]any{
			"reserved_quota": allocation.ReservedQuota,
			"updated_at":     now,
		}).Error; err != nil {
			return err
		}
		var owner User
		if err := lockForUpdate(tx).First(&owner, reservation.OwnerId).Error; err != nil {
			return err
		}
		if err := chargeResellerWallet(tx, &owner, reservation.WholesaleQuota, "release", requestID); err != nil {
			return err
		}
		return tx.Model(&reservation).Updates(map[string]any{
			"state":      ResellerReservationReleased,
			"updated_at": now,
		}).Error
	})
	if err == nil && ownerId > 0 {
		_ = invalidateUserCache(ownerId)
	}
	return err
}
