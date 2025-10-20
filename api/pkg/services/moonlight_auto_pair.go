package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog/log"
)

// MoonlightAutoPairService handles automatic pairing of Wolf with moonlight-web
type MoonlightAutoPairService struct {
	moonlightWebURL string
	wolfURL         string
	credentials     string
}

// NewMoonlightAutoPairService creates a new auto-pairing service
func NewMoonlightAutoPairService(moonlightWebURL, wolfURL, credentials string) *MoonlightAutoPairService {
	return &MoonlightAutoPairService{
		moonlightWebURL: moonlightWebURL,
		wolfURL:         wolfURL,
		credentials:     credentials,
	}
}

// PairRequest for moonlight-web pairing API
type PairRequest struct {
	HostID int    `json:"host_id"`
	Pin    string `json:"pin"`
}

// PairResponse from moonlight-web
type PairResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// HostStatus from moonlight-web
type HostStatus struct {
	HostID  int    `json:"host_id"`
	Address string `json:"address"`
	Paired  bool   `json:"paired"`
}

// InitializeAndPair ensures Wolf is added and paired with moonlight-web
func (s *MoonlightAutoPairService) InitializeAndPair(ctx context.Context) error {
	log.Info().
		Str("moonlight_web", s.moonlightWebURL).
		Str("wolf", s.wolfURL).
		Msg("Starting automatic Wolf pairing with moonlight-web")

	// Wait for moonlight-web to be ready
	if err := s.waitForMoonlightWeb(ctx); err != nil {
		return fmt.Errorf("moonlight-web not ready: %w", err)
	}

	// Check if Wolf is already paired
	paired, err := s.checkHostPaired(0) // Host ID 0 is Wolf
	if err != nil {
		log.Warn().Err(err).Msg("Failed to check pairing status")
	} else if paired {
		log.Info().Msg("✅ Wolf is already paired with moonlight-web")
		return nil
	}

	// Get pairing PIN from Wolf
	// For auto-pairing, we'll use a well-known PIN or fetch from Wolf's API
	// Since Wolf doesn't expose pairing PIN via API, we'll use serverinfo approach
	pin, err := s.getPairingPIN()
	if err != nil {
		return fmt.Errorf("failed to get pairing PIN: %w", err)
	}

	// Initiate pairing with moonlight-web
	if err := s.pairHost(0, pin); err != nil {
		return fmt.Errorf("failed to pair with Wolf: %w", err)
	}

	log.Info().Msg("✅ Successfully paired Wolf with moonlight-web")
	return nil
}

// waitForMoonlightWeb waits for moonlight-web to be ready
func (s *MoonlightAutoPairService) waitForMoonlightWeb(ctx context.Context) error {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for moonlight-web")
		case <-ticker.C:
			resp, err := http.Get(s.moonlightWebURL + "/")
			if err == nil && resp.StatusCode == 200 {
				resp.Body.Close()
				log.Info().Msg("moonlight-web is ready")
				return nil
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
}

// checkHostPaired checks if a host is already paired
func (s *MoonlightAutoPairService) checkHostPaired(hostID int) (bool, error) {
	url := fmt.Sprintf("%s/api/host?host_id=%d", s.moonlightWebURL, hostID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}

	// Add basic auth
	req.SetBasicAuth(s.credentials, s.credentials)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, nil
	}

	var hostStatus HostStatus
	if err := json.NewDecoder(resp.Body).Decode(&hostStatus); err != nil {
		return false, err
	}

	return hostStatus.Paired, nil
}

// getPairingPIN gets the pairing PIN for Wolf
// Wolf auto-accepts pairing when MOONLIGHT_INTERNAL_PAIRING_PIN is set
// We use the same PIN that Wolf expects for automatic pairing
func (s *MoonlightAutoPairService) getPairingPIN() (string, error) {
	// Get the auto-pairing PIN from environment
	// This must match the MOONLIGHT_INTERNAL_PAIRING_PIN set in Wolf's environment
	pin := os.Getenv("MOONLIGHT_INTERNAL_PAIRING_PIN")
	if pin == "" {
		return "", fmt.Errorf("MOONLIGHT_INTERNAL_PAIRING_PIN not set - cannot auto-pair")
	}

	log.Info().
		Str("pin", pin).
		Msg("Using auto-pairing PIN from environment")

	return pin, nil
}

// pairHost initiates pairing with a host
func (s *MoonlightAutoPairService) pairHost(hostID int, pin string) error {
	url := fmt.Sprintf("%s/api/pair", s.moonlightWebURL)

	reqBody := PairRequest{
		HostID: hostID,
		Pin:    pin,
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("pairing failed: %s (status: %d)", string(body), resp.StatusCode)
	}

	log.Info().
		Int("host_id", hostID).
		Str("response", string(body)).
		Msg("Pairing request sent successfully")

	return nil
}
