package license

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

type License struct {
	ID           string    `json:"id"`
	Organization string    `json:"organization"`
	Valid        bool      `json:"valid"`
	Issued       time.Time `json:"issued"`
	ValidUntil   time.Time `json:"valid_until"`
	Features     Features  `json:"features"`
	Limits       Limits    `json:"limits"`
}

type Features struct {
	Users bool `json:"users"`
}

type Limits struct {
	Users    int64 `json:"users"`
	Machines int64 `json:"machines"`
}

func (l *License) Expired() bool {
	return time.Now().After(l.ValidUntil)
}

type LicenseValidator interface {
	Validate(license string) (*License, error)
}

type DefaultValidator struct {
	publicKey *ecdsa.PublicKey
}

func NewLicenseValidator() *DefaultValidator {
	// Embedded public key for validating licenses
	publicKeyPEM := `LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0KTUZrd0V3WUhLb1pJemowQ0FRWUlLb1pJemowREFRY0RRZ0FFMjdOMmRjOXQ1UFZ4Ym1Nc0R6YlF1UkM4WjQzSwpUSkw5MnQ1UDRlYzBhWFc2TXJYL1FaYW4xUkhqSEhWQTMvSmJTdmswWjhHM1oxOTBVcWZWY0lpM2FnPT0KLS0tLS1FTkQgUFVCTElDIEtFWS0tLS0tCg==`

	publicKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyPEM)
	if err != nil {
		panic(fmt.Sprintf("failed to decode public key: %v", err))
	}

	publicKey, err := x509.ParsePKIXPublicKey(publicKeyBytes)
	if err != nil {
		panic(fmt.Sprintf("failed to parse public key: %v", err))
	}

	return &DefaultValidator{
		publicKey: publicKey.(*ecdsa.PublicKey),
	}
}

func (v *DefaultValidator) Validate(licenseStr string) (*License, error) {
	// For now, just decode and return the license
	// TODO: Add actual signature validation
	var license License
	err := json.Unmarshal([]byte(licenseStr), &license)
	if err != nil {
		return nil, fmt.Errorf("invalid license format: %w", err)
	}

	return &license, nil
}
