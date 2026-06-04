package yellowdog

import (
	"context"
	"net/url"
)

// Namespace mirrors the JSON shape returned by GET /api/namespaces.
// Field names match the platform's documented model; we capture only
// what Helix uses today (the ID for path-construction and the human
// name for display). Add fields as concrete needs arise rather than
// pre-emptively bloating the type.
type Namespace struct {
	ID        string `json:"id"`
	Namespace string `json:"namespace"`
	Deletable bool   `json:"deletable"`
}

// ListNamespaces returns the first page of namespaces visible to the
// current credentials.
//
// This is also the smoke-test endpoint for the client: it requires
// authentication, returns a small predictable payload, and has no
// side effects. If this call works against a real YellowDog account,
// the auth header construction and JSON decoding plumbing are sound.
func (c *Client) ListNamespaces(ctx context.Context) (*Page[Namespace], error) {
	var page Page[Namespace]
	if err := c.getJSON(ctx, "/namespaces", url.Values{}, &page); err != nil {
		return nil, err
	}
	return &page, nil
}
