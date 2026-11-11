package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	maxProviderEndpointNameLength    = 128
	maxProviderEndpointBaseURLLength = 2048
)

// validateProviderEndpointShape enforces the minimum configuration a provider
// endpoint needs to be usable: a name and, unless it routes through Vertex AI
// (which derives its URL from project/region), a well-formed http(s) base URL.
// Name and BaseURL must already be trimmed. This checks shape only — it does
// not resolve or contact the destination.
func validateProviderEndpointShape(endpoint *types.ProviderEndpoint) error {
	if endpoint.Name == "" {
		return errors.New("name is required")
	}
	if len(endpoint.Name) > maxProviderEndpointNameLength {
		return fmt.Errorf("name exceeds %d characters", maxProviderEndpointNameLength)
	}
	if strings.IndexFunc(endpoint.Name, unicode.IsControl) >= 0 {
		return errors.New("name contains control characters")
	}
	if endpoint.VertexRegion != "" && !vertexRegionPattern.MatchString(endpoint.VertexRegion) {
		return errors.New("vertex_region must be a Google Cloud region name such as us-east5 or global")
	}
	if endpoint.BaseURL == "" {
		if endpoint.VertexProjectID != "" {
			return nil
		}
		return errors.New("base_url is required")
	}
	return validateProviderBaseURL(endpoint.BaseURL)
}

// vertexRegionPattern matches a Google Cloud region ("us-east5") or "global".
// The region is interpolated into the Vertex hostname, so anything else could
// point the request — and the bearer token on it — at another host.
var vertexRegionPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// errVertexCredentialsFileAdminOnly and errVertexCredentialsRequired are the
// two ways a non-admin Vertex endpoint could otherwise authenticate with the
// server's own Google credentials: a path to a key file on the server, or no
// credentials at all, which falls back to the server's Application Default
// Credentials.
var (
	errVertexCredentialsFileAdminOnly = errors.New("only admins can add endpoints that read Vertex credentials from a server file")
	errVertexCredentialsRequired      = errors.New("vertex_credentials_json is required for a Vertex AI endpoint")
)

// validateVertexCredentialsSource rejects Vertex configurations that would
// borrow the server's own Google credentials. Admins configure the server, so
// they may use a credentials file or the server's default credentials.
func validateVertexCredentialsSource(endpoint *types.ProviderEndpoint, isAdmin bool) error {
	if isAdmin {
		return nil
	}
	if endpoint.VertexCredentialsFile != "" {
		return errVertexCredentialsFileAdminOnly
	}
	if endpoint.VertexProjectID != "" && endpoint.VertexCredentialsJSON == "" {
		return errVertexCredentialsRequired
	}
	return nil
}

// validateProviderBaseURL checks the base URL's form. Scheme and host rules
// come from netguard.ValidateHTTPURL, the shared outbound-URL check;
// destination (SSRF) policy is enforced separately by
// validateProviderEndpointBaseURL and again at dial time.
func validateProviderBaseURL(raw string) error {
	if len(raw) > maxProviderEndpointBaseURLLength {
		return fmt.Errorf("base_url exceeds %d characters", maxProviderEndpointBaseURLLength)
	}
	u, err := netguard.ValidateHTTPURL(raw)
	if err != nil {
		return fmt.Errorf("invalid base_url: %w", err)
	}
	if u.User != nil {
		return errors.New("base_url must not embed credentials; use api_key or headers instead")
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return errors.New("base_url must not contain a fragment")
	}
	return nil
}

// jsonDecodeErrorMessage converts an encoding/json decode error into a
// client-facing message that names the offending JSON field without exposing
// Go type names (e.g. "pq.StringArray", "Go struct field ProviderEndpoint").
func jsonDecodeErrorMessage(err error) string {
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &typeErr) && typeErr.Field != "":
		return fmt.Sprintf("Invalid request body: field %q has the wrong type", typeErr.Field)
	case errors.As(err, &syntaxErr), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "Invalid request body: must be a valid JSON object"
	default:
		return "Invalid request body"
	}
}
