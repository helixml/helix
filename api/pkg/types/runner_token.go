package types

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// RunnerTokenPrefix starts every per-runner token secret, so the auth layer
// can tell one apart from the shared RUNNER_TOKEN and from user API keys
// (APIKeyPrefix) without a lookup.
const RunnerTokenPrefix = "hrt-"

// runnerTokenVisiblePrefixLen is how much of a secret a listing shows to tell
// tokens apart: the prefix plus four random characters.
const runnerTokenVisiblePrefixLen = len(RunnerTokenPrefix) + 4

// RunnerToken is a credential bound to one runner (sandbox host) id. Only a
// hash of the secret is stored; the secret is shown once, when it is minted.
type RunnerToken struct {
	ID string `json:"id" gorm:"primaryKey"`
	// RunnerID is the host this token may act as. A host registers RevDial
	// tunnels as "hydra-{id}" and "sandbox-{id}" and calls the sandbox and
	// runner routes with the bare id; all of those match (RunnerIDsMatch).
	RunnerID string `json:"runner_id" gorm:"index;not null"`
	Name     string `json:"name"`
	// TokenHash is HashRunnerToken(secret). The secret is 256 random bits, so
	// a plain SHA-256 is enough; there is nothing to brute-force.
	TokenHash   string     `json:"-" gorm:"uniqueIndex;not null"`
	TokenPrefix string     `json:"token_prefix"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty" gorm:"index"`
	// RevokedBy is the user who revoked the token. With CreatedBy and the
	// timestamps, the row is the token's durable audit record; the request
	// audit log has the full requests.
	RevokedBy string `json:"revoked_by,omitempty"`
}

// Revoked reports whether the token has been revoked.
func (t *RunnerToken) Revoked() bool {
	return t.RevokedAt != nil
}

// HashRunnerToken is the stored form of a per-runner token secret.
func HashRunnerToken(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// RunnerTokenVisiblePrefix is the part of a secret a listing may show.
func RunnerTokenVisiblePrefix(secret string) string {
	if len(secret) <= 2*runnerTokenVisiblePrefixLen {
		return ""
	}
	return secret[:runnerTokenVisiblePrefixLen]
}

// runnerHostIDPrefixes are the RevDial id forms a sandbox host registers
// under its own id.
var runnerHostIDPrefixes = []string{"hydra-", "sandbox-"}

// CanonicalRunnerID strips one RevDial host prefix ("hydra-", "sandbox-"),
// so a host's tunnel ids and its bare id compare equal.
func CanonicalRunnerID(id string) string {
	for _, prefix := range runnerHostIDPrefixes {
		if rest, ok := strings.CutPrefix(id, prefix); ok && rest != "" {
			return rest
		}
	}
	return id
}

// RunnerIDsMatch reports whether a token bound to bound may act as presented.
func RunnerIDsMatch(bound, presented string) bool {
	return bound != "" && presented != "" && CanonicalRunnerID(bound) == CanonicalRunnerID(presented)
}

// CreateRunnerTokenRequest mints a per-runner token.
type CreateRunnerTokenRequest struct {
	RunnerID string `json:"runner_id"`
	Name     string `json:"name"`
}

// CreateRunnerTokenResponse carries the token secret. It is returned once,
// from the create call, and never again.
type CreateRunnerTokenResponse struct {
	RunnerToken
	Token string `json:"token"`
}
