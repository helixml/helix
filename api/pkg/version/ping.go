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

	"github.com/Nerzal/gocloak/v13"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/rs/zerolog/log"
)

type PingResponse struct {
	LatestVersion string `json:"latest_version"`
}

type PingService struct {
	db             *store.PostgresStore
	launchpadURL   string
	ticker         *time.Ticker
	done           chan bool
	licenseKey     string
	latestVersion  string
	keycloakConfig *config.Keycloak
	gocloak        *gocloak.GoCloak
}

func NewPingService(db *store.PostgresStore, licenseKey string, launchpadURL string, keycloakConfig *config.Keycloak) *PingService {
	return &PingService{
		db:             db,
		launchpadURL:   launchpadURL,
		ticker:         time.NewTicker(1 * time.Hour),
		done:           make(chan bool),
		licenseKey:     licenseKey,
		latestVersion:  "",
		keycloakConfig: keycloakConfig,
		gocloak:        gocloak.NewClient(keycloakConfig.KeycloakURL),
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
		s.sendPing()

		for {
			select {
			case <-s.done:
				return
			case <-ctx.Done():
				return
			case <-s.ticker.C:
				s.sendPing()
			}
		}
	}()
}

func (s *PingService) Stop() {
	s.ticker.Stop()
	s.done <- true
}

func (s *PingService) sendPing() {
	// Get app count from database
	appCount, err := s.db.GetAppCount()
	if err != nil {
		log.Error().Err(err).Msg("Error getting app count")
		return
	}

	// Get user count from Keycloak
	userCount, err := s.getUserCount()
	if err != nil {
		log.Error().Err(err).Msg("Error getting user count")
		return
	}

	deploymentID := "unknown"
	if s.licenseKey != "" {
		// Generate deployment ID from license key
		hasher := sha256.New()
		hasher.Write([]byte(s.licenseKey)) // Use license key hash for deployment ID
		deploymentID = hex.EncodeToString(hasher.Sum(nil))
	}

	// Prepare ping data
	pingData := map[string]interface{}{
		"version":       data.GetHelixVersion(),
		"apps_count":    appCount,
		"users_count":   userCount,
		"deployment_id": deploymentID,
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

func (s *PingService) getUserCount() (int, error) {
	// Get admin token
	token, err := s.gocloak.LoginAdmin(
		context.Background(),
		s.keycloakConfig.Username,
		s.keycloakConfig.Password,
		s.keycloakConfig.AdminRealm,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get admin token: %w", err)
	}

	// Get users count
	count, err := s.gocloak.GetUserCount(
		context.Background(),
		token.AccessToken,
		s.keycloakConfig.Realm,
		gocloak.GetUsersParams{},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get user count: %w", err)
	}

	return int(count), nil
}

func (s *PingService) GetLatestVersion() string {
	return s.latestVersion
}
