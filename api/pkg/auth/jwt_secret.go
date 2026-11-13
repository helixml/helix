package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
)

// jwtSecretBytes is the size of a generated JWT signing secret.
const jwtSecretBytes = 32

// legacyPublicJWTSecrets are values earlier releases used as built-in JWT
// signing secrets. They are public, so a token signed with one proves nothing:
// a configured value equal to one of them is treated as unset.
var legacyPublicJWTSecrets = []string{
	"helix-default-jwt-secret",
	"helix-default-secret-change-in-production",
}

func isLegacyPublicJWTSecret(secret string) bool {
	for _, legacy := range legacyPublicJWTSecrets {
		if secret == legacy {
			return true
		}
	}
	return false
}

// ResolveJWTSecret returns the secret the regular auth provider signs tokens
// with. A configured value (REGULAR_AUTH_JWT_SECRET) wins. Otherwise the secret
// persisted in the database is used, generating and storing a random one on
// first boot so it survives restarts and is shared by every API replica.
func ResolveJWTSecret(ctx context.Context, s store.Store, configured string) (string, error) {
	if configured != "" && !isLegacyPublicJWTSecret(configured) {
		return configured, nil
	}
	if configured != "" {
		log.Warn().Msg("REGULAR_AUTH_JWT_SECRET is set to a publicly known default and is ignored; " +
			"using the generated secret stored in the database. Tokens signed with the old value are no longer accepted.")
	}

	candidate, err := generateJWTSecret()
	if err != nil {
		return "", err
	}
	secret, err := s.EnsureJWTSigningSecret(ctx, candidate)
	if err != nil {
		return "", fmt.Errorf("failed to load or store JWT signing secret: %w", err)
	}
	return secret, nil
}

func generateJWTSecret() (string, error) {
	buf := make([]byte, jwtSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate JWT signing secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
