package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/oauth2/google"
)

// vertexRegionPattern matches Google Cloud location names ("global",
// "us-east5", "europe-west1"). The region becomes part of the Vertex hostname,
// so anything else could redirect the request, and its bearer token, to a
// different host.
var vertexRegionPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidateVertexRegion rejects a region that is not a plain location name.
func ValidateVertexRegion(region string) error {
	if !vertexRegionPattern.MatchString(region) {
		return fmt.Errorf("invalid vertex_region %q", region)
	}
	return nil
}

// vertexServiceAccountTokenURLs are the token endpoints a service account key
// may name. Google issues keys with the first; the second is the legacy form.
var vertexServiceAccountTokenURLs = map[string]bool{
	"":                 true, // library default (google.JWTTokenURL)
	google.JWTTokenURL: true,
	"https://accounts.google.com/o/oauth2/token": true,
}

// ValidateTenantVertexCredentials checks Vertex credentials supplied by a user
// or organization (not by the Helix operator). Only an inline service account
// key is accepted:
//   - credential files and Application Default Credentials would make the
//     request authenticate as the Helix server's own Google identity;
//   - other credential types ("external_account", "impersonated_service_account",
//     "authorized_user", ...) can name arbitrary token, metadata and file
//     sources that the API server would read or contact;
//   - a service account key with a non-Google token_uri would make the server
//     post a signed assertion to an arbitrary URL.
func ValidateTenantVertexCredentials(credentialsJSON string) error {
	if strings.TrimSpace(credentialsJSON) == "" {
		return errors.New("vertex_credentials_json is required: provide a service account key")
	}
	var key struct {
		Type     string `json:"type"`
		TokenURI string `json:"token_uri"`
	}
	if err := json.Unmarshal([]byte(credentialsJSON), &key); err != nil {
		return errors.New("vertex_credentials_json must be a service account key in JSON format")
	}
	if key.Type != "service_account" {
		return fmt.Errorf("vertex_credentials_json type %q is not supported: provide a service account key", key.Type)
	}
	if !vertexServiceAccountTokenURLs[key.TokenURI] {
		return fmt.Errorf("vertex_credentials_json token_uri %q is not a Google token endpoint", key.TokenURI)
	}
	return nil
}
