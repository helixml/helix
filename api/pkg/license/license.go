package license

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

type Envelope struct {
	Data      string `json:"data"`
	Signature string `json:"signature"`
}

func (e *Envelope) UnmarshalJSON(data []byte) error {
	type Alias Envelope
	aux := &struct {
		Signature string `json:"signature"`
		*Alias
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded, err := base64.StdEncoding.DecodeString(aux.Signature)
	if err != nil {
		return fmt.Errorf("signature decode failed: %s", err)
	}
	e.Signature = string(decoded)
	return nil
}

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

type Validator interface {
	Validate(license string) (*License, error)
}

type DefaultValidator struct {
	publicKey *ecdsa.PublicKey
}

func NewLicenseValidator() *DefaultValidator {
	// Embedded public key for validating licenses
	publicKeyPEM := `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE27N2dc9t5PVxbmMsDzbQuRC8Z43K
TJL92t5P4ec0aXW6MrX/QZan1RHjHHVA3/JbSvk0Z8G3Z190UqfVcIi3ag==
-----END PUBLIC KEY-----`

	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		panic("failed to parse PEM block containing the public key")
	}

	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		panic(fmt.Sprintf("failed to parse public key: %v", err))
	}

	ecdsaKey, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		panic("public key is not an ECDSA key")
	}

	return &DefaultValidator{
		publicKey: ecdsaKey,
	}
}

func (v *DefaultValidator) Validate(licenseStr string) (*License, error) {
	// Decode base64 encoded license string
	decodedStr, err := base64.StdEncoding.DecodeString(licenseStr)
	if err != nil {
		return nil, fmt.Errorf("invalid license encoding: %w", err)
	}

	// Parse the envelope containing the license and signature
	var envelope Envelope
	if err := json.Unmarshal(decodedStr, &envelope); err != nil {
		return nil, fmt.Errorf("invalid license format: %w", err)
	}

	// Verify signature
	if !Verify([]byte(envelope.Data), []byte(envelope.Signature), v.publicKey) {
		return nil, fmt.Errorf("invalid license signature")
	}

	// Parse the actual license data
	var license License
	if err := json.Unmarshal([]byte(envelope.Data), &license); err != nil {
		return nil, fmt.Errorf("invalid license data: %w", err)
	}

	return &license, nil
}

// Verify checks a raw ECDSA signature.
// Returns true if it's valid and false if not.
func Verify(data, signature []byte, pubkey *ecdsa.PublicKey) bool {
	// hash message
	digest := sha256.Sum256(data)

	curveOrderByteSize := pubkey.Curve.Params().P.BitLen() / 8

	r, s := new(big.Int), new(big.Int)
	r.SetBytes(signature[:curveOrderByteSize])
	s.SetBytes(signature[curveOrderByteSize:])

	return ecdsa.Verify(pubkey, digest[:], r, s)
}
