package version

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/license"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/rs/zerolog/log"
)

func hashLicenseKey(licenseKey string) string {
	if licenseKey == "" {
		return "unknown"
	}
	hasher := sha256.New()
	hasher.Write([]byte(licenseKey))
	return hex.EncodeToString(hasher.Sum(nil))
}

type PingResponse struct {
	LatestVersion string `json:"latest_version"`
}

type PingService struct {
	db            *store.PostgresStore
	launchpadURL  string
	ticker        *time.Ticker
	done          chan bool
	envLicenseKey string // renamed from licenseKey to be more explicit
	latestVersion string
	edition       string // e.g. "mac-desktop", "server", ""
}

func NewPingService(db *store.PostgresStore, envLicenseKey string, launchpadURL string, edition string) *PingService {
	return &PingService{
		db:            db,
		launchpadURL:  launchpadURL,
		ticker:        time.NewTicker(1 * time.Hour),
		done:          make(chan bool),
		envLicenseKey: envLicenseKey,
		latestVersion: "",
		edition:       edition,
	}
}

func (s *PingService) Start(ctx context.Context) {
	// Don't start the ping service if HELIX_DISABLE_VERSION_CHECK is set
	if os.Getenv("HELIX_DISABLE_VERSION_CHECK") != "" {
		log.Info().Msg("Version check service disabled via HELIX_DISABLE_VERSION_CHECK")
		return
	}

	go func() {
		// Send initial ping
		s.SendPing(ctx)

		for {
			select {
			case <-s.done:
				return
			case <-ctx.Done():
				return
			case <-s.ticker.C:
				s.SendPing(ctx)
			}
		}
	}()
}

func (s *PingService) Stop() {
	s.ticker.Stop()
	s.done <- true
}

func (s *PingService) SendPing(ctx context.Context) {
	// Get app count from database
	appCount, err := s.db.GetAppCount()
	if err != nil {
		log.Error().Err(err).Msg("Error getting app count")
		return
	}

	// Get user count
	userCount, err := s.getUserCount(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Error getting user count")
		return
	}

	// Prepare ping data
	pingData := map[string]interface{}{
		"version":       data.GetHelixVersion(),
		"apps_count":    appCount,
		"users_count":   userCount,
		"deployment_id": s.GetDeploymentID(),
	}
	if s.edition != "" {
		pingData["edition"] = s.edition
	}

	// Send ping to launchpad
	jsonData, err := json.Marshal(pingData)
	if err != nil {
		log.Error().Err(err).Msg("Error marshaling ping data")
		return
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/api/ping", s.launchpadURL),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		log.Error().Err(err).Msg("Error sending ping")
		return
	}
	defer resp.Body.Close()

	// Parse the response to get latest version
	var pingResp PingResponse
	if err := json.NewDecoder(resp.Body).Decode(&pingResp); err != nil {
		log.Error().Err(err).Msg("Error decoding ping response")
		return
	}

	// Store the latest version
	s.latestVersion = pingResp.LatestVersion
}

func (s *PingService) getUserCount(ctx context.Context) (int64, error) {
	// Get the number of users from the database
	userCount, err := s.db.CountUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get user count: %w", err)
	}

	return userCount, nil
}

func (s *PingService) GetLatestVersion() string {
	return s.latestVersion
}

func (s *PingService) GetDeploymentID() string {
	// Check for license key in database first
	if dbLicense, err := s.db.GetLicenseKey(context.Background()); err != nil {
		log.Error().Err(err).Msg("failed to get license key from database")
	} else if dbLicense != nil && dbLicense.LicenseKey != "" {
		return hashLicenseKey(dbLicense.LicenseKey)
	}

	// Fall back to environment license key if no valid database license
	return hashLicenseKey(s.envLicenseKey)
}

// GetLicenseInfo returns the decoded license information from either the database or environment
func (s *PingService) GetLicenseInfo(ctx context.Context) (*license.License, error) {
	// First try to get from database
	dbLicense, err := s.db.GetDecodedLicense(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to get decoded license from database")
		// Continue to try environment license as fallback
	} else if dbLicense != nil {
		return dbLicense, nil
	}

	// If no valid database license, try to decode from environment
	if s.envLicenseKey != "" {
		validator := license.NewLicenseValidator()
		decodedLicense, err := validator.Validate(s.envLicenseKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decode environment license key: %w", err)
		}
		return decodedLicense, nil
	}

	return nil, nil
}
