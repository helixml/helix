package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/wolf"
	"github.com/rs/zerolog/log"
)

// MoonlightWebPairingService handles automatic pairing of moonlight-web with Wolf
// Uses the existing Wolf pairing infrastructure
type MoonlightWebPairingService struct {
	wolfClient      *wolf.Client
	moonlightWebURL string
	credentials     string
}

// NewMoonlightWebPairingService creates a new pairing service
func NewMoonlightWebPairingService(wolfClient *wolf.Client, moonlightWebURL, credentials string) *MoonlightWebPairingService {
	return &MoonlightWebPairingService{
		wolfClient:      wolfClient,
		moonlightWebURL: moonlightWebURL,
		credentials:     credentials,
	}
}

// MoonlightWebHost represents a host in moonlight-web's data.json
type MoonlightWebHost struct {
	Address             string  `json:"address"`
	HTTPPort            int     `json:"http_port"`
	Name                string  `json:"name,omitempty"`
	ClientPrivateKey    *string `json:"client_private_key"`
	ClientCertificate   *string `json:"client_certificate"`
	ServerCertificate   *string `json:"server_certificate"`
}

// MoonlightWebData represents moonlight-web's data.json structure
type MoonlightWebData struct {
	Hosts []MoonlightWebHost `json:"hosts"`
}

// ensureWolfHostExists adds Wolf to moonlight-web's host list if not already present
func (s *MoonlightWebPairingService) ensureWolfHostExists() error {
	// Step 1: Check if Wolf is already in the hosts list
	listURL := fmt.Sprintf("%s/api/hosts", s.moonlightWebURL)

	req, err := http.NewRequest("GET", listURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create list hosts request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.credentials)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to list hosts from moonlight-web: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to list hosts: status %d, body: %s", resp.StatusCode, string(body))
	}

	var hostsResp struct {
		Hosts []struct {
			Address string `json:"address"`
		} `json:"hosts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&hostsResp); err != nil {
		return fmt.Errorf("failed to decode hosts list: %w", err)
	}

	// Check if Wolf is already in the list
	for _, host := range hostsResp.Hosts {
		if host.Address == "wolf" {
			log.Info().Msg("Wolf host already exists in moonlight-web")
			return nil
		}
	}

	// Step 2: Add Wolf via PUT /api/host
	addURL := fmt.Sprintf("%s/api/host", s.moonlightWebURL)

	hostData := map[string]interface{}{
		"address":   "wolf",
		"http_port": 47989,
	}

	jsonData, err := json.Marshal(hostData)
	if err != nil {
		return fmt.Errorf("failed to marshal host data: %w", err)
	}

	log.Info().
		Str("url", addURL).
		Str("address", "wolf").
		Int("http_port", 47989).
		Msg("Adding Wolf host to moonlight-web via PUT /api/host")

	req, err = http.NewRequest("PUT", addURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create add host request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.credentials)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add Wolf host: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// 404 means Wolf is not reachable yet - this is a real error
	if resp.StatusCode == 404 {
		return fmt.Errorf("Wolf not reachable at wolf:47989 - cannot add host")
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to add Wolf host: status %d, body: %s", resp.StatusCode, string(body))
	}

	log.Info().
		Str("response", string(body)).
		Msg("✅ Wolf host added to moonlight-web")
	return nil
}

// waitForWolf waits for Wolf to be ready
func (s *MoonlightWebPairingService) waitForWolf(ctx context.Context, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for Wolf")
		case <-ticker.C:
			// Try to list apps from Wolf internal API
			_, err := s.wolfClient.ListApps()
			if err == nil {
				log.Info().Msg("✅ Wolf is ready")
				return nil
			}
		}
	}
}

// AutoPairWolf ensures Wolf is registered as a host in moonlight-web
// Note: Actual per-client pairing happens in moonlight-web's streaming code,
// not here. Each client session pairs itself with unique credentials using
// MOONLIGHT_INTERNAL_PAIRING_PIN for auto-acceptance by Wolf.
func (s *MoonlightWebPairingService) AutoPairWolf(ctx context.Context) error {
	log.Info().Msg("🔗 Ensuring Wolf is registered in moonlight-web")

	// Step 1: Wait for moonlight-web to be ready
	if err := s.waitForMoonlightWeb(ctx, 30*time.Second); err != nil {
		return fmt.Errorf("moonlight-web not ready: %w", err)
	}

	// Step 2: Wait for Wolf to be ready
	if err := s.waitForWolf(ctx, 30*time.Second); err != nil {
		return fmt.Errorf("Wolf not ready: %w", err)
	}

	// Step 3: Add Wolf as a host in moonlight-web if not already present
	// This is all we need - per-client pairing happens automatically in stream.rs
	if err := s.ensureWolfHostExists(); err != nil {
		return fmt.Errorf("failed to add Wolf host: %w", err)
	}

	log.Info().Msg("✅ Wolf registered in moonlight-web - per-client pairing will happen automatically")
	return nil
}

// waitForMoonlightWeb waits for moonlight-web to be ready
func (s *MoonlightWebPairingService) waitForMoonlightWeb(ctx context.Context, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for moonlight-web")
		case <-ticker.C:
			resp, err := http.Get(s.moonlightWebURL + "/")
			if err == nil && resp.StatusCode == 200 {
				resp.Body.Close()
				log.Info().Msg("✅ moonlight-web is ready")
				return nil
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
}

// InitializeOnStartup should be called when Helix API starts
// Ensures Wolf is registered as a host in moonlight-web
func (s *MoonlightWebPairingService) InitializeOnStartup(ctx context.Context) error {
	// Run host registration in background - don't block Helix startup
	go func() {
		if err := s.AutoPairWolf(context.Background()); err != nil {
			log.Warn().Err(err).Msg("⚠️ Failed to register Wolf host in moonlight-web")
		}
	}()

	return nil
}
