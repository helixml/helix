package audit

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactArguments(t *testing.T) {
	raw := json.RawMessage(`{"name":"production","password":"hunter2","nested":{"api_token":"token-value","public_key":"ssh-ed25519 AAAA"},"items":[{"private_key":"key-value","safe":true}]}`)

	redacted := RedactArguments(raw)

	require.JSONEq(t, `{"name":"production","password":"[REDACTED]","nested":{"api_token":"[REDACTED]","public_key":"ssh-ed25519 AAAA"},"items":[{"private_key":"[REDACTED]","safe":true}]}`, string(redacted))
}

func TestRedactArgumentsInvalidJSON(t *testing.T) {
	require.JSONEq(t, `{}`, string(RedactArguments(json.RawMessage(`not-json`))))
}
