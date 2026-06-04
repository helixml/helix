package yellowdog

import "fmt"

// Credentials holds the YellowDog API key pair. They live in the
// platform's keyring; ours are read from .env or a Helix secret store
// at runtime. Never log either field.
type Credentials struct {
	KeyID  string
	Secret string
}

// Valid returns true if both fields are non-empty. Callers should reject
// an attempt to construct a Client with invalid credentials early.
func (c Credentials) Valid() bool {
	return c.KeyID != "" && c.Secret != ""
}

// authHeader returns the value of the Authorization header expected by
// the YellowDog API. Format mirrors the official Python SDK (see
// yellowdog_client/common/credentials/api_key_authentication_headers_provider.py):
//
//	Authorization: yd-key <keyID>:<secret>
func (c Credentials) authHeader() string {
	return fmt.Sprintf("yd-key %s:%s", c.KeyID, c.Secret)
}
