// Package attestation mints and verifies identity attestations: short-lived,
// single-audience tokens in which Helix states which spec task, session or
// sandbox a caller's key belongs to, so an external verifier can learn the
// caller's identity without ever holding its Helix key.
//
// Tokens are HS256 JWTs signed with a key derived (HKDF-SHA256) from the
// server's JWT signing secret under a dedicated label. Session JWTs are signed
// with the secret itself, so neither kind of token verifies as the other; the
// kid, typ, issuer and purpose claim make the distinction explicit as well.
package attestation

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/helixml/helix/api/pkg/types"
)

const (
	// TTL is how long an attestation stays valid after it is minted.
	TTL = 300 * time.Second
	// ClockSkew is the leeway allowed on iat and exp when verifying.
	ClockSkew = 30 * time.Second

	keyID   = "helix-identity-attestation-v1"
	tokType = "helix-attestation+jwt"
	issuer  = "helix-identity-attestation"
	purpose = "identity_attestation"
	// hkdfInfo separates the attestation key from every other use of the
	// JWT signing secret.
	hkdfInfo = "helix/identity-attestation/hs256/v1"

	minNonceLen = 16
	maxNonceLen = 128
	// MaxTokenLen bounds the token a verifier may submit.
	MaxTokenLen = 4096
)

var (
	audiencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)
	noncePattern    = regexp.MustCompile(`^[A-Za-z0-9._~+/=-]+$`)

	// ErrInvalid is returned for every verification failure. Callers must
	// not distinguish reasons in their responses.
	ErrInvalid = errors.New("attestation not valid")
)

// ValidateAudience checks an audience: 1–128 characters of letters, digits
// and . _ : / -, starting with a letter or digit.
func ValidateAudience(audience string) error {
	if !audiencePattern.MatchString(audience) {
		return errors.New("audience must be 1-128 characters of [A-Za-z0-9._:/-], starting with a letter or digit")
	}
	return nil
}

// ValidateNonce checks a nonce: 16–128 URL/base64-safe characters.
func ValidateNonce(nonce string) error {
	if len(nonce) < minNonceLen || len(nonce) > maxNonceLen || !noncePattern.MatchString(nonce) {
		return fmt.Errorf("nonce must be %d-%d characters of [A-Za-z0-9._~+/=-]", minNonceLen, maxNonceLen)
	}
	return nil
}

// Subject is the identity an attestation states. The server fills it from the
// minting key and the rows the key is bound to.
type Subject struct {
	SpecTaskID     string
	SessionID      string
	SandboxID      string
	ProjectID      string
	OrganizationID string
	Owner          string
}

type tokenClaims struct {
	jwt.RegisteredClaims
	Purpose        string `json:"purpose"`
	Nonce          string `json:"nonce"`
	SpecTaskID     string `json:"spec_task_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	SandboxID      string `json:"sandbox_id,omitempty"`
	ProjectID      string `json:"project_id"`
	OrganizationID string `json:"organization_id,omitempty"`
}

// Signer mints and verifies attestations with one derived key.
type Signer struct {
	key []byte
	now func() time.Time
}

// NewSigner derives the attestation key from the server's JWT signing secret.
func NewSigner(secret string) (*Signer, error) {
	if secret == "" {
		return nil, errors.New("attestation signer needs a non-empty secret")
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, hkdfInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("derive attestation key: %w", err)
	}
	return &Signer{key: key, now: time.Now}, nil
}

// WithClock returns a copy of the signer that reads the time from now.
func (s *Signer) WithClock(now func() time.Time) *Signer {
	return &Signer{key: s.key, now: now}
}

// Mint signs an attestation of subject for audience and nonce.
func (s *Signer) Mint(subject Subject, audience, nonce string) (string, *types.IdentityAttestationClaims, error) {
	if err := ValidateAudience(audience); err != nil {
		return "", nil, err
	}
	if err := ValidateNonce(nonce); err != nil {
		return "", nil, err
	}
	if subject.ProjectID == "" || subject.Owner == "" {
		return "", nil, errors.New("attestation subject needs a project and an owner")
	}
	jti, err := newJTI()
	if err != nil {
		return "", nil, err
	}
	issuedAt := s.now().UTC().Truncate(time.Second)
	claims := tokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   subject.Owner,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			NotBefore: jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(TTL)),
			ID:        jti,
		},
		Purpose:        purpose,
		Nonce:          nonce,
		SpecTaskID:     subject.SpecTaskID,
		SessionID:      subject.SessionID,
		SandboxID:      subject.SandboxID,
		ProjectID:      subject.ProjectID,
		OrganizationID: subject.OrganizationID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = keyID
	token.Header["typ"] = tokType
	signed, err := token.SignedString(s.key)
	if err != nil {
		return "", nil, fmt.Errorf("sign attestation: %w", err)
	}
	return signed, claims.public(), nil
}

// Verify checks the token's signature, type, expiry, audience and nonce. Every
// failure returns ErrInvalid.
func (s *Signer) Verify(token, audience, nonce string) (*types.IdentityAttestationClaims, error) {
	if token == "" || len(token) > MaxTokenLen || audience == "" || nonce == "" {
		return nil, ErrInvalid
	}
	claims := &tokenClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Header["kid"] != keyID || t.Header["typ"] != tokType {
			return nil, ErrInvalid
		}
		return s.key, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(ClockSkew),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil || !parsed.Valid {
		return nil, ErrInvalid
	}
	if claims.Purpose != purpose || claims.ID == "" || claims.ProjectID == "" || claims.Subject == "" ||
		claims.IssuedAt == nil || len(claims.Audience) != 1 ||
		claims.ExpiresAt.Sub(claims.IssuedAt.Time) > TTL ||
		subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1 {
		return nil, ErrInvalid
	}
	return claims.public(), nil
}

func (c *tokenClaims) public() *types.IdentityAttestationClaims {
	return &types.IdentityAttestationClaims{
		SpecTaskID:     c.SpecTaskID,
		SessionID:      c.SessionID,
		SandboxID:      c.SandboxID,
		ProjectID:      c.ProjectID,
		OrganizationID: c.OrganizationID,
		Owner:          c.Subject,
		Audience:       c.Audience[0],
		Nonce:          c.Nonce,
		IssuedAt:       c.IssuedAt.UTC(),
		ExpiresAt:      c.ExpiresAt.UTC(),
		JTI:            c.ID,
	}
}

func newJTI() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate attestation id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
