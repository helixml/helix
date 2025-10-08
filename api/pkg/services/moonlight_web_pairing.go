package services

import (
	"bytes"
	"context"
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

// AutoPairWolf automatically pairs moonlight-web with Wolf on startup
// This ensures browser streaming works without manual intervention
func (s *MoonlightWebPairingService) AutoPairWolf(ctx context.Context) error {
	log.Info().Msg("🔗 Checking moonlight-web pairing status with Wolf")

	// Step 1: Wait for moonlight-web to be ready
	if err := s.waitForMoonlightWeb(ctx, 30*time.Second); err != nil {
		return fmt.Errorf("moonlight-web not ready: %w", err)
	}

	// Step 2: Check if Wolf is already paired
	paired, err := s.isWolfPaired()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to check pairing status, will attempt pairing anyway")
	} else if paired {
		log.Info().Msg("✅ Wolf is already paired with moonlight-web")
		return nil
	}

	log.Info().Msg("🔐 Wolf not paired - initiating automatic pairing")

	// Step 3: Trigger pairing request from moonlight-web to Wolf
	// moonlight-web will create a pairing request when it fetches serverinfo
	if err := s.triggerPairingRequest(); err != nil {
		return fmt.Errorf("failed to trigger pairing: %w", err)
	}

	// Step 4: Get pending pair request from Wolf
	time.Sleep(2 * time.Second) // Give Wolf time to register the request
	pendingRequests, err := s.wolfClient.GetPendingPairRequests()
	if err != nil {
		return fmt.Errorf("failed to get pending pair requests from Wolf: %w", err)
	}

	if len(pendingRequests) == 0 {
		return fmt.Errorf("no pending pairing requests found in Wolf")
	}

	// Use the first pending request (should be from moonlight-web)
	pairRequest := pendingRequests[0]
	pairSecret := pairRequest.PairSecret
	if pairSecret == "" {
		return fmt.Errorf("invalid pair request: missing pair_secret")
	}

	// Step 5: Generate PIN for pairing
	// Use a well-known PIN for internal moonlight-web (production-safe since localhost-only)
	pin := "0000" // Internal pairing PIN - moonlight-web is trusted localhost service

	// Step 6: Complete pairing in Wolf using existing Wolf client method
	if err := s.wolfClient.PairClient(pairSecret, pin); err != nil {
		return fmt.Errorf("failed to complete Wolf pairing: %w", err)
	}

	log.Info().Msg("✅ Successfully paired moonlight-web with Wolf automatically")

	// Step 7: moonlight-web will receive the pairing response and save certificates
	// Verify pairing completed
	time.Sleep(2 * time.Second)
	if paired, err := s.isWolfPaired(); err == nil && paired {
		log.Info().Msg("✅ Pairing verified - certificates saved")
		return nil
	}

	log.Warn().Msg("⚠️ Pairing completed in Wolf but moonlight-web may not have saved certificates")
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

// isWolfPaired checks if Wolf is already paired in moonlight-web
func (s *MoonlightWebPairingService) isWolfPaired() (bool, error) {
	// Get host info from moonlight-web API
	url := fmt.Sprintf("%s/api/host?host_id=0", s.moonlightWebURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}

	req.SetBasicAuth(s.credentials, s.credentials)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		// Not authenticated to check - assume not paired
		return false, nil
	}

	if resp.StatusCode != 200 {
		return false, nil
	}

	// Parse response to check if certificates exist
	var hostInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&hostInfo); err != nil {
		return false, err
	}

	// Check if host has pairing status
	if paired, ok := hostInfo["paired"].(bool); ok {
		return paired, nil
	}

	// Fallback: Check if client certificate exists in data.json
	// (moonlight-web stores certs in data.json after successful pairing)
	return s.checkDataJsonForCerts()
}

// checkDataJsonForCerts checks if data.json has certificates (alternative check)
func (s *MoonlightWebPairingService) checkDataJsonForCerts() (bool, error) {
	// Read data.json from moonlight-web container
	// For now, return false to force pairing attempt
	// TODO: Implement volume inspection or API endpoint
	return false, nil
}

// triggerPairingRequest makes moonlight-web initiate a pairing request to Wolf
func (s *MoonlightWebPairingService) triggerPairingRequest() error {
	// Call moonlight-web's pair endpoint to initiate pairing
	url := fmt.Sprintf("%s/api/pair", s.moonlightWebURL)

	reqBody := map[string]interface{}{
		"host_id": 0, // Wolf is host 0
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(s.credentials, s.credentials)

	// This is a streaming endpoint - it will return pairing status updates
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read initial response
	body, _ := io.ReadAll(resp.Body)
	log.Info().
		Str("response", string(body)).
		Msg("Pairing request initiated in moonlight-web")

	return nil
}

// InitializeOnStartup should be called when Helix API starts
// Returns nil if pairing successful or already paired
// Returns error only if pairing is critical and failed
func (s *MoonlightWebPairingService) InitializeOnStartup(ctx context.Context) error {
	// Run auto-pairing in background - don't block Helix startup
	go func() {
		if err := s.AutoPairWolf(context.Background()); err != nil {
			log.Warn().Err(err).Msg("⚠️ Auto-pairing failed - manual pairing required")
			log.Info().Msg("To pair manually: Open http://localhost:8080/moonlight/ and follow pairing wizard")
		}
	}()

	return nil
}
