package store

import (
	"context"
	"os"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// GetSystemSettings retrieves the global system settings
func (s *PostgresStore) GetSystemSettings(ctx context.Context) (*types.SystemSettings, error) {
	var settings types.SystemSettings
	err := s.gdb.WithContext(ctx).Where("id = ?", types.SystemSettingsID).First(&settings).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create default settings if they don't exist
			settings = types.SystemSettings{
				ID:      types.SystemSettingsID,
				Created: time.Now(),
				Updated: time.Now(),
			}
			err = s.gdb.WithContext(ctx).Create(&settings).Error
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return &settings, nil
}

// GetEffectiveSystemSettings retrieves system settings with environment variable fallback
func (s *PostgresStore) GetEffectiveSystemSettings(ctx context.Context) (*types.SystemSettings, error) {
	settings, err := s.GetSystemSettings(ctx)
	if err != nil {
		return nil, err
	}

	// Create a copy for effective settings
	effectiveSettings := *settings

	// If no HF token is set in database, check environment variable
	if effectiveSettings.HuggingFaceToken == "" {
		if envToken := os.Getenv("HF_TOKEN"); envToken != "" {
			effectiveSettings.HuggingFaceToken = envToken
		}
	}

	return &effectiveSettings, nil
}

// UpdateSystemSettings updates the global system settings
func (s *PostgresStore) UpdateSystemSettings(ctx context.Context, req *types.SystemSettingsRequest) (*types.SystemSettings, error) {
	// Get existing settings
	settings, err := s.GetSystemSettings(ctx)
	if err != nil {
		return nil, err
	}

	// Update only provided fields
	if req.HuggingFaceToken != nil {
		settings.HuggingFaceToken = *req.HuggingFaceToken
	}
	if req.KoditEnrichmentProvider != nil {
		settings.KoditEnrichmentProvider = *req.KoditEnrichmentProvider
	}
	if req.KoditEnrichmentModel != nil {
		settings.KoditEnrichmentModel = *req.KoditEnrichmentModel
	}

	settings.Updated = time.Now()

	// Save changes
	err = s.gdb.WithContext(ctx).Save(settings).Error
	if err != nil {
		return nil, err
	}

	return settings, nil
}
