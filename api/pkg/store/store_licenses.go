package store

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (d *PostgresStore) GetLicenseKey(ctx context.Context) (*types.LicenseKey, error) {
	var license types.LicenseKey
	// Get the most recent license key by created_at
	result := d.gdb.WithContext(ctx).Order("created_at DESC").First(&license)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &license, nil
}

func (d *PostgresStore) SetLicenseKey(ctx context.Context, licenseKey string) error {
	// Simply create a new license key record
	return d.gdb.WithContext(ctx).Create(&types.LicenseKey{
		LicenseKey: licenseKey,
	}).Error
}
