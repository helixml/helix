package store

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm/clause"

	"github.com/helixml/helix/api/pkg/types"
)

// RedeemIdentityAttestation inserts the jti unless it is already present, in
// one statement, so two concurrent verifications cannot both succeed.
func (s *PostgresStore) RedeemIdentityAttestation(ctx context.Context, jti string, expiresAt time.Time) (bool, error) {
	if jti == "" {
		return false, fmt.Errorf("attestation jti is required")
	}
	db := s.gdb.WithContext(ctx)
	if err := db.Where("expires_at < ?", time.Now()).Delete(&types.IdentityAttestationRedemption{}).Error; err != nil {
		return false, fmt.Errorf("failed to drop expired attestation redemptions: %w", err)
	}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&types.IdentityAttestationRedemption{
		JTI:       jti,
		ExpiresAt: expiresAt,
	})
	if result.Error != nil {
		return false, fmt.Errorf("failed to record attestation redemption: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}
