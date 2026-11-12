// Package auditlog implements the durable request audit trail: redaction of
// request bodies and query strings, and the recorder that fans events out to
// stdout and Postgres without ever blocking the request it describes.
package auditlog

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// RedactedValue replaces anything the redactor considers secret.
const RedactedValue = "[REDACTED]"

// BodyLimit is how much of a mutating request body is captured as the handler
// reads it (the handler still receives every byte).
const BodyLimit = 64 * 1024

// StoredBodyLimit is how much redacted JSON is persisted per event.
const StoredBodyLimit = 4 * 1024

// StoredQueryLimit caps the recorded query string.
const StoredQueryLimit = 4 * 1024

// sensitiveKeyPattern matches key names whose values must not be recorded.
// It deliberately accepts both separator styles: provider endpoints spell the
// header `x-api-key` or `api-key` at least as often as `api_key`.
var sensitiveKeyPattern = regexp.MustCompile(`(?i)password|secret|token|api[-_]?key|authorization|cookie|credential|private`)

// nonSecretKeySuffixes marks keys that merely *name* a credential location or
// identity. `api_key_file` and `base_url` are exactly what the Nov 10 incident
// needed, so they are kept verbatim - unless the value itself turns out to be a
// credential (see redactKeptValue).
var nonSecretKeySuffixes = []string{"_file", "_path", "_url", "_id", "_type", "_name"}

// containerKeys hold credentials as their children rather than as their own
// value, so masking the key's value is not enough: every child value is masked
// while the child name (header name, environment variable name) is kept.
var containerKeys = map[string]struct{}{
	"headers":     {},
	"env":         {},
	"environment": {},
	"variables":   {},
	"secrets":     {},
}

func isContainerKey(key string) bool {
	_, ok := containerKeys[strings.ToLower(key)]
	return ok
}

// IsSensitiveKey reports whether a JSON key's value must be masked outright.
func IsSensitiveKey(key string) bool {
	if isContainerKey(key) {
		return true
	}
	if !sensitiveKeyPattern.MatchString(key) {
		return false
	}
	return !hasNonSecretSuffix(key)
}

// isSensitiveKeptKey reports whether key names a credential but holds a
// location or identifier, e.g. `api_key_file`, `token_url`, `secret_id`.
func isSensitiveKeptKey(key string) bool {
	return sensitiveKeyPattern.MatchString(key) && hasNonSecretSuffix(key)
}

func hasNonSecretSuffix(key string) bool {
	lower := strings.ToLower(key)
	for _, suffix := range nonSecretKeySuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// secretQueryKeys are the query parameters the auth layer actually reads, plus
// the OAuth handshake parameters. `code` and `state` are single-use bearer
// credentials during the handshake and are replayable if recorded.
var secretQueryKeys = map[string]struct{}{
	"token":         {},
	"access_token":  {},
	"refresh_token": {},
	"id_token":      {},
	"api_key":       {},
	"key":           {},
	"code":          {},
	"state":         {},
	"session_state": {},
	"password":      {},
}

func isSensitiveQueryKey(key string) bool {
	if _, ok := secretQueryKeys[strings.ToLower(key)]; ok {
		return true
	}
	return IsSensitiveKey(key)
}

var secretValuePrefixes = []string{
	"sk-",
	"ghp_",
	"github_pat_",
	"hl-",
	"Bearer ",
	"bearer ",
	"-----BEGIN ",
}

// jwtPattern matches a three-segment JWT. Any value carrying one is masked even
// under an innocent key, because customers paste tokens into free-text fields.
var jwtPattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)

// opaqueCredentialPattern matches an unstructured run of credential
// characters: a base64/hex blob or an opaque provider key. Paths and URLs
// contain `/`, `.` or `:` and are therefore never matched.
var opaqueCredentialPattern = regexp.MustCompile(`^[A-Za-z0-9+=_-]{32,}$`)

// lowercaseWordsPattern matches snake_case names, which are structural rather
// than opaque credentials.
var lowercaseWordsPattern = regexp.MustCompile(`^[a-z][a-z_]*$`)

// LooksLikeSecret reports whether a string value is itself a credential.
func LooksLikeSecret(value string) bool {
	trimmed := strings.TrimSpace(value)
	for _, prefix := range secretValuePrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return jwtPattern.MatchString(trimmed)
}

// looksOpaqueCredential reports whether a value is a long, unstructured token.
// It is applied to values under credential-named keys that are otherwise kept
// (e.g. `client_secret_id`) so that an opaque secret pasted under a benign
// suffix cannot slip through.
func looksOpaqueCredential(value string) bool {
	trimmed := strings.TrimSpace(value)
	if !opaqueCredentialPattern.MatchString(trimmed) {
		return false
	}
	// A snake_case name is a word, not a credential.
	return !lowercaseWordsPattern.MatchString(trimmed)
}

// RedactJSON parses a JSON body, masks sensitive keys and secret-looking
// values, and returns the re-encoded document. An unparseable body is an
// error; the caller records only the content type in that case.
func RedactJSON(data []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	return json.Marshal(redactValue(document))
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch {
			case isContainerKey(key):
				typed[key] = redactAllValues(child)
			case IsSensitiveKey(key):
				typed[key] = RedactedValue
			case isSensitiveKeptKey(key):
				typed[key] = redactKeptValue(child)
			default:
				typed[key] = redactValue(child)
			}
		}
		return typed
	case []any:
		for i, child := range typed {
			typed[i] = redactValue(child)
		}
		return typed
	case string:
		return redactString(typed)
	default:
		return value
	}
}

// redactAllValues masks every value below a credential container, keeping the
// names (header names, environment variable names) that say which credential
// was in play.
func redactAllValues(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			typed[key] = redactAllValues(child)
		}
		return typed
	case []any:
		for i, child := range typed {
			typed[i] = redactAllValues(child)
		}
		return typed
	case string:
		return redactAssignment(typed)
	default:
		return RedactedValue
	}
}

// redactKeptValue keeps a location or identifier but still masks an opaque
// credential pasted under a credential-named key with a benign suffix.
func redactKeptValue(value any) any {
	if typed, ok := value.(string); ok && looksOpaqueCredential(typed) {
		return RedactedValue
	}
	return redactValue(value)
}

func redactString(value string) any {
	if assignment, ok := redactAssignmentIfSecret(value); ok {
		return assignment
	}
	if LooksLikeSecret(value) || urlCarriesCredential(value) {
		return RedactedValue
	}
	return value
}

// redactAssignment handles a `KEY=value` string - an environment entry, a
// shell command fragment, a DSN - whose left side names a credential.
func redactAssignmentIfSecret(value string) (string, bool) {
	name, secret, found := strings.Cut(strings.TrimSpace(value), "=")
	if !found || name == "" || secret == "" {
		return "", false
	}
	if strings.ContainsAny(name, " \t\"'") {
		return "", false
	}
	if !IsSensitiveKey(name) {
		return "", false
	}
	return name + "=" + RedactedValue, true
}

// redactAssignment masks a container child value, keeping a `KEY=` prefix so
// the record still names the variable that was set.
func redactAssignment(value string) string {
	if assignment, ok := redactAssignmentIfSecret(value); ok {
		return assignment
	}
	name, secret, found := strings.Cut(strings.TrimSpace(value), "=")
	if found && name != "" && secret != "" && !strings.ContainsAny(name, " \t\"'") {
		return name + "=" + RedactedValue
	}
	return RedactedValue
}

// urlCarriesCredential reports whether a URL embeds a credential in its
// userinfo (`postgres://user:pass@host`) or in a sensitive query parameter.
// Such a value is masked whole; the surrounding key still says what it was.
func urlCarriesCredential(value string) bool {
	trimmed := strings.TrimSpace(value)
	if !strings.Contains(trimmed, "://") {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	if parsed.User != nil {
		if _, set := parsed.User.Password(); set {
			return true
		}
		// `https://ghp_xxx@host` is a token in the username position.
		if LooksLikeSecret(parsed.User.Username()) || looksOpaqueCredential(parsed.User.Username()) {
			return true
		}
	}
	for key := range parsed.Query() {
		if isSensitiveQueryKey(key) {
			return true
		}
	}
	return false
}

// RedactQuery masks credential-bearing query parameters while leaving the rest
// of the string readable. The raw query is kept as sent (no re-encoding).
func RedactQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	parts := strings.Split(rawQuery, "&")
	for i, part := range parts {
		key, _, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		decodedKey, err := url.QueryUnescape(key)
		if err != nil {
			decodedKey = key
		}
		if isSensitiveQueryKey(decodedKey) {
			parts[i] = key + "=" + RedactedValue
		}
	}
	return strings.Join(parts, "&")
}

// domainVerifyPathPrefix is the only route that carries a credential in the
// path: knowing the token lets someone complete a hostname verification they do
// not control.
const domainVerifyPathPrefix = "/.well-known/helix-domain-verify/"

// RedactPath masks credentials that appear as a path segment. Everything else
// in a path is structural and worth keeping.
func RedactPath(path string) string {
	if rest, ok := strings.CutPrefix(path, domainVerifyPathPrefix); ok && rest != "" {
		return domainVerifyPathPrefix + RedactedValue
	}
	return path
}

// Truncate cuts s to at most max bytes without splitting a UTF-8 rune, and
// reports whether it cut anything.
func Truncate(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	cut := max
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// IsJSONContentType reports whether a Content-Type is JSON (including +json).
func IsJSONContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}
