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
		"api-key":          true, // provider headers spell it with a dash
		"x-api-key":        true,
		"authorization":    true,
		"cookie":           true,
		"credential":       true,
		"private_key":      true,
		"headers":          true, // containers are masked child-by-child
		"env":              true,
		"secrets":          true,
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

// Credentials arrive in several shapes that key-name matching alone misses:
// provider endpoint headers with dashed names, environment lists, and a
// KEY=value string pasted into a free-text field.
func TestRedactJSONCredentialShapes(t *testing.T) {
	t.Run("dashed api key header names", func(t *testing.T) {
		redacted, err := RedactJSON([]byte(`{
			"endpoint": {
				"headers": {"x-api-key": "provider-key-one", "api-key": "provider-key-two", "anthropic-version": "2023-06-01"}
			}
		}`))
		require.NoError(t, err)

		var document map[string]any
		require.NoError(t, json.Unmarshal(redacted, &document))
		headers := document["endpoint"].(map[string]any)["headers"].(map[string]any)
		require.Equal(t, RedactedValue, headers["x-api-key"])
		require.Equal(t, RedactedValue, headers["api-key"])
		require.Equal(t, RedactedValue, headers["anthropic-version"], "every child value of a container is masked")
		require.NotContains(t, string(redacted), "provider-key")
	})

	t.Run("environment list keeps variable names", func(t *testing.T) {
		// types.DesktopAgent.Env is []string of "KEY=value".
		redacted, err := RedactJSON([]byte(`{
			"env": ["GITHUB_TOKEN=ghp_realtokenvalue", "PATH=/usr/local/bin"]
		}`))
		require.NoError(t, err)
		require.NotContains(t, string(redacted), "ghp_realtokenvalue")
		require.NotContains(t, string(redacted), "/usr/local/bin")
		require.Contains(t, string(redacted), "GITHUB_TOKEN=")
		require.Contains(t, string(redacted), "PATH=")
	})

	t.Run("environment map redacts every value", func(t *testing.T) {
		redacted, err := RedactJSON([]byte(`{"environment": {"SENTRY_DSN": "https://abc@example.com/1", "REGION": "eu-west"}}`))
		require.NoError(t, err)
		require.NotContains(t, string(redacted), "abc@example.com")
		require.NotContains(t, string(redacted), "eu-west")
		require.Contains(t, string(redacted), "SENTRY_DSN")
	})

	t.Run("variables container", func(t *testing.T) {
		redacted, err := RedactJSON([]byte(`{"variables": {"anything": "plain-value"}}`))
		require.NoError(t, err)
		require.NotContains(t, string(redacted), "plain-value")
	})

	t.Run("key value string names the credential", func(t *testing.T) {
		redacted, err := RedactJSON([]byte(`{"note": "DB_PASSWORD=s3cr3t", "other": "hello world"}`))
		require.NoError(t, err)

		var document map[string]any
		require.NoError(t, json.Unmarshal(redacted, &document))
		require.Equal(t, "DB_PASSWORD="+RedactedValue, document["note"])
		require.Equal(t, "hello world", document["other"])
	})

	t.Run("credentials embedded in kept urls", func(t *testing.T) {
		redacted, err := RedactJSON([]byte(`{
			"base_url": "https://api.example.com",
			"webhook_url": "https://hooks.example.com/T1/B2?token=live-token",
			"database_url": "postgres://helix:dbpassword@db.internal:5432/helix"
		}`))
		require.NoError(t, err)
		require.NotContains(t, string(redacted), "live-token")
		require.NotContains(t, string(redacted), "dbpassword")
		require.Contains(t, string(redacted), "https://api.example.com", "a credential-free url is kept")
	})

	t.Run("opaque secret under a kept suffix", func(t *testing.T) {
		// `client_secret_id` is a kept suffix, but the value here is a secret.
		redacted, err := RedactJSON([]byte(`{"client_secret_id": "AKIAIOSFODNN7EXAMPLE1234567890ABCDEF"}`))
		require.NoError(t, err)
		require.NotContains(t, string(redacted), "AKIAIOSFODNN7EXAMPLE")
	})

	t.Run("ordinary identifiers under kept suffixes survive", func(t *testing.T) {
		redacted, err := RedactJSON([]byte(`{
			"api_key_file": "/run/secrets/helix-key",
			"token_url": "https://auth.example.com/token",
			"secret_id": "sec_01hxyz",
			"maximum_number_of_retries_allowed": "whatever"
		}`))
		require.NoError(t, err)

		var document map[string]any
		require.NoError(t, json.Unmarshal(redacted, &document))
		require.Equal(t, "/run/secrets/helix-key", document["api_key_file"])
		require.Equal(t, "https://auth.example.com/token", document["token_url"])
		require.Equal(t, "sec_01hxyz", document["secret_id"])
		require.Equal(t, "whatever", document["maximum_number_of_retries_allowed"])
	})
}

func TestRedactPath(t *testing.T) {
	require.Equal(t, "/.well-known/helix-domain-verify/"+RedactedValue,
		RedactPath("/.well-known/helix-domain-verify/abc123"))
	require.Equal(t, "/api/v1/projects/prj_1/secrets", RedactPath("/api/v1/projects/prj_1/secrets"))
	require.Equal(t, "/.well-known/helix-domain-verify/", RedactPath("/.well-known/helix-domain-verify/"))
}

func TestIsSensitiveBodyRoute(t *testing.T) {
	sensitive := []string{
		"/api/v1/secrets",
		"/api/v1/secrets/sec_1",
		"/api/v1/projects/prj_1/secrets",
		"/api/v1/auth/login",
		"/api/v1/auth/password-reset-complete",
		"/api/v1/oauth/providers",
		"/api/v1/oauth/connections/oauth_1",
		"/api/v1/git-provider-connections",
		"/api/v1/git-provider-connections/gpc_1",
		"/api/v1/service-connections/sc_1",
		"/api/v1/provider-endpoints/pe_1",
		"/api/v1/agents",
		"/api/v1/apps/app_1",
		"/api/v1/organizations/org_1/webhook-endpoints/wh_1/rotate-secret",
		"/api/v1/organizations/org_1/sandboxes",
		"/api/v1/projects/prj_1/secret-intakes/sci_1/submissions",
		"/connect/intake/redeem",
		"/connect/intake/submit",
		"/api/v1/identity/attestations",
		"/api/v1/identity/attestations/verify",
	}
	for _, path := range sensitive {
		require.True(t, IsSensitiveBodyRoute(path), "path %q", path)
	}

	notSensitive := []string{
		"/api/v1/sessions",
		"/api/v1/sessions/ses_1",
		"/api/v1/organizations/org_1/sandboxes/sbx_1/commands",
		"/api/v1/secretsmanager",
		"/api/v1/projects/prj_1",
		"/api/v1/agentsmd",
	}
	for _, path := range notSensitive {
		require.False(t, IsSensitiveBodyRoute(path), "path %q", path)
	}
}

func TestRedactQuery(t *testing.T) {
	cases := map[string]string{
		"":                            "",
		"access_token=plain-token":    "access_token=" + RedactedValue,
		"token=plain-token&page=2":    "token=" + RedactedValue + "&page=2",
		"api_key=plain-key&limit=10":  "api_key=" + RedactedValue + "&limit=10",
		"key=plain-key":               "key=" + RedactedValue,
		"password=plain-password":     "password=" + RedactedValue,
		"query=hello&flag":            "query=hello&flag",
		"token=one&token=two":         "token=" + RedactedValue + "&token=" + RedactedValue,
		"code=oauth-code":             "code=" + RedactedValue,
		"state=opaque-state":          "state=" + RedactedValue,
		"session_state=session-state": "session_state=" + RedactedValue,
		"id_token=a&refresh_token=b":  "id_token=" + RedactedValue + "&refresh_token=" + RedactedValue,
		"client_secret=c":             "client_secret=" + RedactedValue,
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
