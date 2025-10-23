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
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.credentials)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to list hosts, will try adding anyway")
	} else {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var hostsResp struct {
				Hosts []struct {
					Address string `json:"address"`
				} `json:"hosts"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&hostsResp); err == nil {
				// Check if Wolf is already in the list
				for _, host := range hostsResp.Hosts {
					if host.Address == "wolf" {
						log.Info().Msg("Wolf host already exists in moonlight-web")
						return nil
					}
				}
			}
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
		return err
	}

	log.Info().
		Str("url", addURL).
		Str("address", "wolf").
		Int("http_port", 47989).
		Msg("Adding Wolf host to moonlight-web via PUT /api/host")

	req, err = http.NewRequest("PUT", addURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.credentials)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add Wolf host: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// 404 means Wolf is not reachable yet - this is okay, pairing will retry
	if resp.StatusCode == 404 {
		log.Warn().Msg("Wolf not reachable yet (404), will retry during pairing")
		return nil
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to add Wolf host: status %d, body: %s", resp.StatusCode, string(body))
	}

	log.Info().
		Str("response", string(body)).
		Msg("✅ Wolf host added to moonlight-web")
	return nil
}

// AutoPairWolf automatically pairs moonlight-web with Wolf on startup
// This ensures browser streaming works without manual intervention
func (s *MoonlightWebPairingService) AutoPairWolf(ctx context.Context) error {
	log.Info().Msg("🔗 Checking moonlight-web pairing status with Wolf")

	// Step 1: Wait for moonlight-web to be ready
	if err := s.waitForMoonlightWeb(ctx, 30*time.Second); err != nil {
		return fmt.Errorf("moonlight-web not ready: %w", err)
	}

	// Step 2: Add Wolf as a host in moonlight-web if not already present
	if err := s.ensureWolfHostExists(); err != nil {
		log.Warn().Err(err).Msg("Failed to add Wolf host, will attempt pairing anyway")
	}

	// Step 3: Check if Wolf is already paired
	paired, err := s.isWolfPaired()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to check pairing status, will attempt pairing anyway")
	} else if paired {
		log.Info().Msg("✅ Wolf is already paired with moonlight-web")
		return nil
	}

	log.Info().Msg("🔐 Wolf not paired - initiating automatic pairing")

	// Step 3: Trigger pairing request from moonlight-web to Wolf
	// This will make moonlight-web generate a PIN and start the Moonlight protocol
	// Keep the stream open so moonlight-web can receive certificates
	pin, pairStream, err := s.triggerPairingRequest()
	if err != nil {
		return fmt.Errorf("failed to trigger pairing: %w", err)
	}
	defer pairStream.Body.Close()

	log.Info().Str("pin", pin).Msg("moonlight-web generated PIN for pairing")

	// Step 4: Get the pair_secret from Wolf's pending pairing request
	// Wolf creates a pair_secret when moonlight-web calls /pair
	// We need this pair_secret to complete pairing via Wolf's internal API
	time.Sleep(1 * time.Second) // Give Wolf time to create pair request

	pendingRequests, err := s.wolfClient.GetPendingPairRequests()
	if err != nil {
		log.Warn().Err(err).Msg("Could not get pending requests from Wolf internal API")
	}

	var pairSecret string
	if len(pendingRequests) > 0 {
		pairSecret = pendingRequests[0].PairSecret
		log.Info().Str("pair_secret", pairSecret).Msg("Found pair_secret from Wolf internal API")
	}

	// Step 5: Complete pairing by sending PIN to Wolf via Moonlight HTTP protocol
	// This allows moonlight-web to complete the crypto handshake and receive certificates
	if pairSecret != "" {
		log.Info().
			Str("pair_secret", pairSecret).
			Str("pin", pin).
			Msg("Submitting PIN to Wolf via Moonlight protocol HTTP endpoint")

		if err := s.submitPINToWolf(pairSecret, pin); err != nil {
			return fmt.Errorf("failed to submit PIN to Wolf: %w", err)
		}

		log.Info().Msg("✅ PIN submitted to Wolf - waiting for moonlight-web to receive certificates")

		// Step 6: Read the final pairing result from the stream
		// This should be "Paired" if successful
		finalResult, _ := io.ReadAll(pairStream.Body)
		log.Info().
			Str("final_response", string(finalResult)).
			Msg("moonlight-web pairing stream completed")
	} else {
		return fmt.Errorf("could not find pair_secret from Wolf - pairing cannot be completed")
	}

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

	req.Header.Set("Authorization", "Bearer "+s.credentials)

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

// submitPINToWolf submits the PIN to Wolf's Moonlight HTTP protocol endpoint
// This completes the pairing handshake and allows moonlight-web to receive certificates
func (s *MoonlightWebPairingService) submitPINToWolf(pairSecret, pin string) error {
	// CRITICAL: Only use Wolf's HTTP /pin/ endpoint (Moonlight protocol)
	// DO NOT use Wolf internal API - it breaks the Moonlight protocol stream!
	// The internal API completes the promise, preventing moonlight-web from receiving certificates

	url := "http://wolf:47989/pin/"

	// Send PIN and secret as JSON (Wolf's Moonlight HTTP server expects this)
	pinData := map[string]string{
		"pin":    pin,
		"secret": pairSecret,
	}

	jsonData, err := json.Marshal(pinData)
	if err != nil {
		return fmt.Errorf("failed to marshal PIN data: %w", err)
	}

	log.Info().
		Str("url", url).
		Str("pin", pin).
		Str("secret", pairSecret).
		Msg("Submitting PIN to Wolf Moonlight protocol HTTP /pin/ endpoint")

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create PIN submission request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to submit PIN to Wolf HTTP endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Info().
		Int("status", resp.StatusCode).
		Str("response", string(body)).
		Msg("Submitted PIN to Wolf /pin/ endpoint")

	if resp.StatusCode != 200 {
		return fmt.Errorf("Wolf /pin/ rejected PIN: status %d, response: %s", resp.StatusCode, string(body))
	}

	log.Info().Msg("✅ Wolf accepted PIN via Moonlight protocol")
	return nil
}

// triggerPairingRequest makes moonlight-web initiate a pairing request to Wolf
// Returns the PIN generated by moonlight-web and the open HTTP response stream
// Caller MUST close the response body after reading the final pairing result
func (s *MoonlightWebPairingService) triggerPairingRequest() (string, *http.Response, error) {
	// Call moonlight-web's pair endpoint to initiate pairing
	url := fmt.Sprintf("%s/api/pair", s.moonlightWebURL)

	reqBody := map[string]interface{}{
		"host_id": 0, // Wolf is host 0
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, err
	}

	log.Info().
		Str("url", url).
		Str("body", string(jsonData)).
		Msg("Sending pairing request to moonlight-web")

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.credentials)

	// This is a streaming endpoint - it will return pairing status updates
	// DO NOT close the response body yet - we need to keep stream open!
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to call moonlight-web /api/pair")
		return "", nil, err
	}

	log.Info().Int("status", resp.StatusCode).Msg("moonlight-web /api/pair streaming response started")

	// Read first JSON object from NDJSON stream to get PIN
	// Response format: {"Pin":"0681"}\n"PairError" OR {"Pin":"0681"}\n"Paired"
	var pinResponse struct {
		Pin string `json:"Pin"`
	}

	// Use JSON decoder to read first object from stream
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&pinResponse); err != nil {
		resp.Body.Close()
		return "", nil, fmt.Errorf("could not parse PIN from stream: %w", err)
	}

	if pinResponse.Pin == "" {
		resp.Body.Close()
		return "", nil, fmt.Errorf("PIN is empty in response")
	}

	log.Info().Str("pin", pinResponse.Pin).Msg("✅ Extracted PIN from moonlight-web stream")

	// Return PIN and the open response body so caller can complete pairing and read final result
	return pinResponse.Pin, resp, nil
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

// generateSecurePIN generates a cryptographically secure 4-digit PIN
func generateSecurePIN() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based if crypto rand fails
		return fmt.Sprintf("%04d", time.Now().UnixNano()%10000)
	}

	// Convert to 4-digit PIN (0000-9999)
	pin := ""
	for _, byte := range b {
		pin += fmt.Sprintf("%d", byte%10)
	}
	return pin
}
