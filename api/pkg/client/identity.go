package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

// CreateIdentityAttestation mints an attestation of the caller's spec task,
// session or sandbox. Only a desktop's session or sandbox key may call it.
func (c *HelixClient) CreateIdentityAttestation(ctx context.Context, req *types.IdentityAttestationRequest) (*types.IdentityAttestationResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var resp types.IdentityAttestationResponse
	if err := c.makeRequest(ctx, http.MethodPost, "/identity/attestations", bytes.NewReader(body), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// VerifyIdentityAttestation redeems an attestation once and returns its claims.
func (c *HelixClient) VerifyIdentityAttestation(ctx context.Context, req *types.IdentityAttestationVerifyRequest) (*types.IdentityAttestationClaims, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var claims types.IdentityAttestationClaims
	if err := c.makeRequest(ctx, http.MethodPost, "/identity/attestations/verify", bytes.NewReader(body), &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}
