package types

import "time"

// IdentityAttestationRequest asks Helix to attest which spec task, session or
// sandbox the caller's key belongs to, for one audience and one nonce.
type IdentityAttestationRequest struct {
	// Audience names the verifier the token is for, e.g. "ptaas.example.com".
	Audience string `json:"audience"`
	// Nonce is the verifier's challenge (16–128 characters).
	Nonce string `json:"nonce"`
}

// IdentityAttestationResponse carries the signed attestation token.
type IdentityAttestationResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IdentityAttestationVerifyRequest redeems an attestation token. Audience and
// nonce must equal the values the token was minted for.
type IdentityAttestationVerifyRequest struct {
	Token    string `json:"token"`
	Audience string `json:"audience"`
	Nonce    string `json:"nonce"`
}

// IdentityAttestationClaims is what Helix attests about the minting caller.
// Every field is derived server-side from the minting key and the rows it is
// bound to; nothing comes from the request except audience and nonce.
type IdentityAttestationClaims struct {
	SpecTaskID     string    `json:"spec_task_id,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	SandboxID      string    `json:"sandbox_id,omitempty"`
	ProjectID      string    `json:"project_id"`
	OrganizationID string    `json:"organization_id,omitempty"`
	Owner          string    `json:"owner"`
	Audience       string    `json:"aud"`
	Nonce          string    `json:"nonce"`
	IssuedAt       time.Time `json:"iat"`
	ExpiresAt      time.Time `json:"exp"`
	JTI            string    `json:"jti"`
}

// IdentityAttestationRedemption records a verified attestation's jti so the
// token cannot be verified twice. Rows are dropped once the token has expired.
type IdentityAttestationRedemption struct {
	JTI       string    `json:"jti" gorm:"primaryKey;size:64"`
	ExpiresAt time.Time `json:"expires_at" gorm:"index;not null"`
	CreatedAt time.Time `json:"created_at"`
}

// IdentityAttestationError is the body of every refused verification. It is
// the same for every reason so the endpoint is no oracle.
type IdentityAttestationError struct {
	Error string `json:"error"`
}
