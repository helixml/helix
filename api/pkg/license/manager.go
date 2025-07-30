package license

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

type Manager struct {
	license *License
}

func NewLicenseManager(license *License) *Manager {
	return &Manager{
		license: license,
	}
}

func (m *Manager) Run(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Do initial check
	if err := m.validate(); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := m.validate(); err != nil {
				return err
			}
		}
	}
}

func (m *Manager) validate() error {
	if m.license == nil {
		log.Warn().Msg("No license provided - running in development mode")
		return nil
	}

	if !m.license.Valid {
		return fmt.Errorf("license is not valid")
	}

	if m.license.Expired() {
		return fmt.Errorf("license has expired, please contact info@helix.ml")
	}

	return nil
}
