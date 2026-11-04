package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode"

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
	if endpoint.BaseURL == "" {
		if endpoint.VertexProjectID != "" {
			return nil
		}
		return errors.New("base_url is required")
	}
	return validateProviderBaseURL(endpoint.BaseURL)
}

func validateProviderBaseURL(raw string) error {
	if len(raw) > maxProviderEndpointBaseURLLength {
		return fmt.Errorf("base_url exceeds %d characters", maxProviderEndpointBaseURLLength)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("base_url must be a valid absolute http(s) URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("base_url must use the http or https scheme")
	}
	if u.Opaque != "" || u.Hostname() == "" {
		return errors.New("base_url must include a host")
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
