package store

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/license"
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
	// Create a license validator
	validator := license.NewLicenseValidator()

	// Validate the license key before storing
	_, err := validator.Validate(licenseKey)
	if err != nil {
		return fmt.Errorf("invalid license key: %w", err)
	}

	// Create a new license key record if validation passed
	return d.gdb.WithContext(ctx).Create(&types.LicenseKey{
		LicenseKey: licenseKey,
	}).Error
}

// GetDecodedLicense returns the decoded license information for the current license key
func (d *PostgresStore) GetDecodedLicense(ctx context.Context) (*license.License, error) {
	licenseKey, err := d.GetLicenseKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting license key: %w", err)
	}
	if licenseKey == nil {
		return nil, nil
	}

	validator := license.NewLicenseValidator()
	decoded, err := validator.Validate(licenseKey.LicenseKey)
	if err != nil {
		return nil, fmt.Errorf("error decoding license: %w", err)
	}

	return decoded, nil
}
