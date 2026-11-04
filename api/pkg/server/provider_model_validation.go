package server

import (
	"fmt"
	"regexp"
	"strings"
)

const maxProviderModelIDLength = 256

// providerModelIDCharset covers the identifiers real providers use:
// "gpt-4o-mini", "llama3:8b", "meta-llama/Llama-3.1-8B-Instruct",
// "claude-3-5-sonnet@20240620", "us.anthropic.claude-3-5-sonnet-20241022-v2:0",
// and OpenRouter aliases such as "~anthropic/claude-opus-latest".
var providerModelIDCharset = regexp.MustCompile(`^[A-Za-z0-9._:@+~/-]+$`)

// validateProviderModelID rejects model identifiers that are not registry-safe:
// whitespace, characters outside the charset, and path-shaped values (leading,
// trailing or doubled "/", or "." / ".." segments) that could be reinterpreted
// as filesystem or URL paths downstream.
func validateProviderModelID(id string) error {
	if len(id) > maxProviderModelIDLength {
		return fmt.Errorf("model %q exceeds %d characters", id, maxProviderModelIDLength)
	}
	if !providerModelIDCharset.MatchString(id) {
		return fmt.Errorf("model %q contains invalid characters (allowed: letters, digits and . _ : @ + ~ / -)", id)
	}
	for _, segment := range strings.Split(id, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("model %q is not a valid model identifier: empty, '.' or '..' path segments are not allowed", id)
		}
	}
	return nil
}

// normalizeProviderModels trims, drops empty entries, de-duplicates and
// validates an endpoint's enabled-models list.
func normalizeProviderModels(in []string) ([]string, error) {
	models := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, m := range in {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		if err := validateProviderModelID(m); err != nil {
			return nil, err
		}
		seen[m] = true
		models = append(models, m)
	}
	return models, nil
}
