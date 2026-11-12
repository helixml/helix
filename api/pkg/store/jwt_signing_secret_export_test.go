package store

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

// ClearJWTSigningSecretForTest empties the stored JWT signing secret so tests
// in package store_test can observe a first boot.
func (s *PostgresStore) ClearJWTSigningSecretForTest(ctx context.Context) error {
	return s.gdb.WithContext(ctx).Model(&types.SystemSettings{}).
		Where("id = ?", types.SystemSettingsID).
		UpdateColumn("jwt_signing_secret", "").Error
}

// DeleteSystemSettingsForTest removes the settings row so tests can observe a
// first boot on an empty database.
func (s *PostgresStore) DeleteSystemSettingsForTest(ctx context.Context) error {
	return s.gdb.WithContext(ctx).Where("id = ?", types.SystemSettingsID).
		Delete(&types.SystemSettings{}).Error
}
