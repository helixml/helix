package yellowdog

import "fmt"

// Credentials holds the YellowDog API key pair. They live in the
// platform's keyring; ours are read from .env or a Helix secret store
// at runtime.
//
// Both fields are tagged `json:"-"` so accidentally JSON-marshalling
// a struct that embeds this type cannot leak the secret. The String
// and GoString methods redact both fields, so `fmt.Printf("%v", c)`
// or stack traces that print the credentials value will not surface
// the raw secret either.
type Credentials struct {
	KeyID  string `json:"-"`
	Secret string `json:"-"`
}

// String redacts both fields. Used by `fmt` verbs that pick up
// Stringer (%s, %v, %+v).
func (c Credentials) String() string {
	return fmt.Sprintf("yellowdog.Credentials{KeyID: %q, Secret: %q}", maskKey(c.KeyID), redacted(c.Secret))
}

// GoString redacts both fields. Used by `fmt.Printf("%#v", c)`.
func (c Credentials) GoString() string {
	return c.String()
}

func maskKey(s string) string {
	if len(s) == 0 {
		return ""
	}
	if len(s) <= 6 {
		return "<redacted>"
	}
	return s[:6] + "..."
}

func redacted(s string) string {
	if s == "" {
		return ""
	}
	return "<redacted>"
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
//
// Naive concatenation matches the upstream Python implementation; it
// assumes YellowDog secrets contain no `:` (which is consistent with
// observed key formats), and the platform tolerates extra colons by
// treating everything after the first one as the secret.
//
// Do NOT log the return value of this method, embed it in error
// strings, or call httputil.DumpRequest on a request that carries it.
func (c Credentials) authHeader() string {
	return fmt.Sprintf("yd-key %s:%s", c.KeyID, c.Secret)
}
