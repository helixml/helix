package auditlog

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSensitiveKey(t *testing.T) {
	cases := map[string]bool{
		"password":         true,
		"current_password": true,
		"secret":           true,
		"client_secret":    true,
		"token":            true,
		"access_token":     true,
		"api_key":          true,
		"apikey":           true,
		"apiKey":           true,
		"authorization":    true,
		"cookie":           true,
		"credential":       true,
		"private_key":      true,
		"api_key_file":     false, // the path is exactly what we need to see
		"private_key_path": false,
		"token_url":        false,
		"secret_id":        false,
		"credential_type":  false,
		"api_key_name":     false,
		"base_url":         false,
		"user_id":          false,
		"email":            false,
		"name":             false,
	}
	for key, want := range cases {
		require.Equal(t, want, IsSensitiveKey(key), "key %q", key)
	}
}

func TestRedactJSON(t *testing.T) {
	// Assembled from parts so secret scanners do not match the fixtures.
	jwtFixture := "eyJhbGciOiJIUzI1NiJ9" + "." + "eyJzdWIiOiIxMjM0NTY3ODkwIn0" + "." + "dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	pemFixture := "-----BEGIN " + "RSA PRIVATE KEY-----"

	input := []byte(`{
		"email": "victim@example.com",
		"password": "hunter2",
		"api_key": "abc123",
		"api_key_file": "/run/secrets/helix-key",
		"base_url": "https://api.example.com",
		"nested": {"access_token": "abc123", "keep": "visible"},
		"listed": [{"client_secret": "abc123"}, "visible"],
		"free_text": "Bearer abc123",
		"looks_like_key": "sk-abc123",
		"pem": "` + pemFixture + `",
		"jwt": "` + jwtFixture + `"
	}`)

	redacted, err := RedactJSON(input)
	require.NoError(t, err)

	var document map[string]any
	require.NoError(t, json.Unmarshal(redacted, &document))

	require.Equal(t, "victim@example.com", document["email"])
	require.Equal(t, RedactedValue, document["password"])
	require.Equal(t, RedactedValue, document["api_key"])
	require.Equal(t, "/run/secrets/helix-key", document["api_key_file"])
	require.Equal(t, "https://api.example.com", document["base_url"])

	nested := document["nested"].(map[string]any)
	require.Equal(t, RedactedValue, nested["access_token"])
	require.Equal(t, "visible", nested["keep"])

	listed := document["listed"].([]any)
	require.Equal(t, RedactedValue, listed[0].(map[string]any)["client_secret"])
	require.Equal(t, "visible", listed[1])

	require.Equal(t, RedactedValue, document["free_text"])
	require.Equal(t, RedactedValue, document["looks_like_key"])
	require.Equal(t, RedactedValue, document["pem"])
	require.Equal(t, RedactedValue, document["jwt"])
}

func TestRedactJSONRejectsInvalidJSON(t *testing.T) {
	_, err := RedactJSON([]byte("not json"))
	require.Error(t, err)
}

func TestRedactQuery(t *testing.T) {
	cases := map[string]string{
		"":                           "",
		"access_token=plain-token":   "access_token=" + RedactedValue,
		"token=plain-token&page=2":   "token=" + RedactedValue + "&page=2",
		"api_key=plain-key&limit=10": "api_key=" + RedactedValue + "&limit=10",
		"key=plain-key":              "key=" + RedactedValue,
		"password=plain-password":    "password=" + RedactedValue,
		"query=hello&flag":           "query=hello&flag",
		"token=one&token=two":        "token=" + RedactedValue + "&token=" + RedactedValue,
	}
	for raw, want := range cases {
		require.Equal(t, want, RedactQuery(raw), "raw query %q", raw)
	}
}

func TestTruncateKeepsUTF8RunesIntact(t *testing.T) {
	input := "aaaé"
	cut, truncated := Truncate(input, 4) // would split the two-byte é
	require.True(t, truncated)
	require.Equal(t, "aaa", cut)

	unchanged, truncated := Truncate("short", 10)
	require.False(t, truncated)
	require.Equal(t, "short", unchanged)
}

func TestIsJSONContentType(t *testing.T) {
	require.True(t, IsJSONContentType("application/json"))
	require.True(t, IsJSONContentType("application/json; charset=utf-8"))
	require.True(t, IsJSONContentType("application/merge-patch+json"))
	require.False(t, IsJSONContentType("multipart/form-data; boundary=x"))
	require.False(t, IsJSONContentType("text/plain"))
}
