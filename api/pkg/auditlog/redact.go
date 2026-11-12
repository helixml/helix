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

// BodyLimit is how much of a mutating request body is captured before it is
// handed back to the handler (which still gets the whole body).
const BodyLimit = 64 * 1024

// StoredBodyLimit is how much redacted JSON is persisted per event.
const StoredBodyLimit = 4 * 1024

// StoredQueryLimit caps the recorded query string.
const StoredQueryLimit = 4 * 1024

// sensitiveKeyPattern matches key names whose values must not be recorded.
var sensitiveKeyPattern = regexp.MustCompile(`(?i)password|secret|token|api_?key|authorization|cookie|credential|private`)

// nonSecretKeySuffixes marks keys that merely *name* a credential location.
// `api_key_file` and `base_url` are exactly what the Nov 10 incident needed,
// so they are kept verbatim.
var nonSecretKeySuffixes = []string{"_file", "_path", "_url", "_id", "_type", "_name"}

// IsSensitiveKey reports whether a JSON key's value should be masked.
func IsSensitiveKey(key string) bool {
	if !sensitiveKeyPattern.MatchString(key) {
		return false
	}
	lower := strings.ToLower(key)
	for _, suffix := range nonSecretKeySuffixes {
		if strings.HasSuffix(lower, suffix) {
			return false
		}
	}
	return true
}

// secretQueryKeys are the query parameters the auth layer actually reads. They
// are masked even though `key` on its own does not match the key regex.
var secretQueryKeys = map[string]struct{}{
	"token":        {},
	"access_token": {},
	"api_key":      {},
	"key":          {},
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
			if IsSensitiveKey(key) {
				typed[key] = RedactedValue
				continue
			}
			typed[key] = redactValue(child)
		}
		return typed
	case []any:
		for i, child := range typed {
			typed[i] = redactValue(child)
		}
		return typed
	case string:
		if LooksLikeSecret(typed) {
			return RedactedValue
		}
		return typed
	default:
		return value
	}
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
