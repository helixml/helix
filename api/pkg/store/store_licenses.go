package store

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (d *PostgresStore) GetLicenseKey(ctx context.Context) (*types.LicenseKey, error) {
	var license types.LicenseKey
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
	// Delete any existing keys first
	if err := d.gdb.WithContext(ctx).Where("1=1").Delete(&types.LicenseKey{}).Error; err != nil {
		return err
	}
	// Create new key
	return d.gdb.WithContext(ctx).Create(&types.LicenseKey{
		LicenseKey: licenseKey,
	}).Error
}
