package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type LicenseKey struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	LicenseKey string    `json:"license_key"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (d *PostgresStore) GetLicenseKey(ctx context.Context) (*LicenseKey, error) {
	var license LicenseKey
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
	if err := d.gdb.WithContext(ctx).Where("1=1").Delete(&LicenseKey{}).Error; err != nil {
		return err
	}
	// Create new key
	return d.gdb.WithContext(ctx).Create(&LicenseKey{
		LicenseKey: licenseKey,
	}).Error
}
