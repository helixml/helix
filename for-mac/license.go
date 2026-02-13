package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// Embedded Launchpad ECDSA P-256 public key (base64-encoded PEM)
const launchpadPubKeyEncoded = `LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0KTUZrd0V3WUhLb1pJemowQ0FRWUlLb1pJemowREFRY0RRZ0FFMjdOMmRjOXQ1UFZ4Ym1Nc0R6YlF1UkM4WjQzSwpUSkw5MnQ1UDRlYzBhWFc2TXJYL1FaYW4xUkhqSEhWQTMvSmJTdmswWjhHM1oxOTBVcWZWY0lpM2FnPT0KLS0tLS1FTkQgUFVCTElDIEtFWS0tLS0tCg==`

// LicenseStatus represents the current license/trial state for the frontend
type LicenseStatus struct {
	State       string `json:"state"`                  // "no_trial", "trial_active", "trial_expired", "licensed"
	TrialEndsAt string `json:"trial_ends_at,omitempty"` // ISO 8601
	LicensedTo  string `json:"licensed_to,omitempty"`   // Organization from license
	ExpiresAt   string `json:"expires_at,omitempty"`    // License expiration
}

// LicenseEnvelope is the signed envelope format from Launchpad
type LicenseEnvelope struct {
	Data      string `json:"data"`
	Signature string `json:"signature"`
}

// LicenseData is the payload inside the envelope
type LicenseData struct {
	ID           string          `json:"id"`
	Valid        bool            `json:"valid"`
	Organization string          `json:"organization"`
	Email        string          `json:"email,omitempty"` // Licensee email (if included by Launchpad)
	Issued       time.Time       `json:"issued"`
	ValidUntil   time.Time       `json:"valid_until"`
	Features     LicenseFeatures `json:"features"`
	Limits       LicenseLimits   `json:"limits"`
}

// LicenseFeatures describes what the license enables
type LicenseFeatures struct {
	Users      bool `json:"users"`
	SuperAdmin bool `json:"superAdmin"`
}

// LicenseLimits describes license limits
type LicenseLimits struct {
	Users    int64 `json:"users"`
	Machines int64 `json:"machines"`
}

const (
	trialDuration   = 24 * time.Hour
	expirationGrace = 24 * time.Hour * 10 // 10-day grace period, matching Launchpad
)

// LicenseValidator handles trial tracking and license key validation
type LicenseValidator struct {
	pubKey *ecdsa.PublicKey
}

// NewLicenseValidator creates a validator with the embedded Launchpad public key
func NewLicenseValidator() (*LicenseValidator, error) {
	pubKey, err := loadEmbeddedPubKey()
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded public key: %w", err)
	}
	return &LicenseValidator{pubKey: pubKey}, nil
}

// GetLicenseStatus determines the current license/trial state
func (v *LicenseValidator) GetLicenseStatus(settings AppSettings) LicenseStatus {
	// Check for valid license first
	if settings.LicenseKey != "" {
		license, err := v.ValidateLicenseKey(settings.LicenseKey)
		if err == nil && license.Valid {
			if !license.ValidUntil.IsZero() && time.Since(license.ValidUntil) > expirationGrace {
				return LicenseStatus{
					State:      "trial_expired", // License expired, treat as expired
					ExpiresAt:  license.ValidUntil.Format(time.RFC3339),
					LicensedTo: license.Organization,
				}
			}
			return LicenseStatus{
				State:      "licensed",
				LicensedTo: license.Organization,
				ExpiresAt:  license.ValidUntil.Format(time.RFC3339),
			}
		}
	}

	// Check trial state
	if settings.TrialStartedAt == nil {
		return LicenseStatus{State: "no_trial"}
	}

	trialEnd := settings.TrialStartedAt.Add(trialDuration)
	if time.Now().After(trialEnd) {
		return LicenseStatus{
			State:       "trial_expired",
			TrialEndsAt: trialEnd.Format(time.RFC3339),
		}
	}

	return LicenseStatus{
		State:       "trial_active",
		TrialEndsAt: trialEnd.Format(time.RFC3339),
	}
}

// GetLicenseeEmail extracts the email from a valid license key, or returns "".
func (v *LicenseValidator) GetLicenseeEmail(settings AppSettings) string {
	if settings.LicenseKey == "" {
		return ""
	}
	license, err := v.ValidateLicenseKey(settings.LicenseKey)
	if err != nil || license == nil {
		return ""
	}
	return license.Email
}

// ValidateLicenseKey validates a base64-encoded license key string
func (v *LicenseValidator) ValidateLicenseKey(key string) (*LicenseData, error) {
	// Step 1: Base64 decode the envelope
	envelopeJSON, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("invalid license key format: %w", err)
	}

	// Step 2: JSON unmarshal the envelope
	var envelope LicenseEnvelope
	if err := json.Unmarshal(envelopeJSON, &envelope); err != nil {
		return nil, fmt.Errorf("invalid license envelope: %w", err)
	}

	// Step 3: Verify ECDSA signature
	data := []byte(envelope.Data)
	sig, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding: %w", err)
	}

	if !verifySignature(data, sig, v.pubKey) {
		return nil, fmt.Errorf("license signature verification failed")
	}

	// Step 4: Parse the license data
	var license LicenseData
	if err := json.Unmarshal(data, &license); err != nil {
		return nil, fmt.Errorf("invalid license data: %w", err)
	}

	if !license.Valid {
		return nil, fmt.Errorf("license is marked as invalid")
	}

	// Step 5: Check expiration (with grace period)
	if !license.ValidUntil.IsZero() && time.Since(license.ValidUntil) > expirationGrace {
		return &license, fmt.Errorf("license expired on %s", license.ValidUntil.Format("2006-01-02"))
	}

	return &license, nil
}

// StartTrial records the trial start time if not already started
func (v *LicenseValidator) StartTrial(sm *SettingsManager) error {
	settings := sm.Get()
	if settings.TrialStartedAt != nil {
		return nil // Already started
	}

	now := time.Now()
	settings.TrialStartedAt = &now
	return sm.Save(settings)
}

// CanStartVM checks if the license/trial state allows starting a VM
func (v *LicenseValidator) CanStartVM(settings AppSettings) error {
	status := v.GetLicenseStatus(settings)
	switch status.State {
	case "licensed", "trial_active":
		return nil
	case "no_trial":
		return fmt.Errorf("no trial started. Start a free 24-hour trial or enter a license key")
	case "trial_expired":
		return fmt.Errorf("trial expired. Enter a license key to continue using Helix Desktop")
	default:
		return fmt.Errorf("unknown license state: %s", status.State)
	}
}

// loadEmbeddedPubKey decodes the embedded base64 PEM public key
func loadEmbeddedPubKey() (*ecdsa.PublicKey, error) {
	// Base64 decode → PEM text
	pemBytes, err := base64.StdEncoding.DecodeString(launchpadPubKeyEncoded)
	if err != nil {
		return nil, fmt.Errorf("failed to base64 decode public key: %w", err)
	}

	// PEM decode → DER block
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to PEM decode public key")
	}

	// Parse PKIX public key
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	ecdsaPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not ECDSA")
	}

	if ecdsaPub.Curve != elliptic.P256() {
		return nil, fmt.Errorf("public key is not P-256")
	}

	return ecdsaPub, nil
}

// verifySignature verifies an ECDSA P-256 signature over data
// Signature format: 64 bytes = 32-byte R + 32-byte S (big-endian, zero-padded)
func verifySignature(data, signature []byte, pubKey *ecdsa.PublicKey) bool {
	if len(signature) != 64 {
		return false
	}

	digest := sha256.Sum256(data)

	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])

	return ecdsa.Verify(pubKey, digest[:], r, s)
}
