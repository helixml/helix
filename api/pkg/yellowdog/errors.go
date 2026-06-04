package yellowdog

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError is the shape YellowDog returns on 4xx / 5xx, mirroring
// RFC 7807 (problem+json). Fields match the live API responses observed
// 2026-06-04 (e.g. an unauthenticated GET against /api/compute/workerPools
// returns {type, title, status, detail, instance}).
type APIError struct {
	Type     string `json:"type,omitempty"`
	Title    string `json:"title,omitempty"`
	Status   int    `json:"status,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

func (e *APIError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("yellowdog: %d %s: %s", e.Status, e.Title, e.Detail)
	}
	if e.Title != "" {
		return fmt.Sprintf("yellowdog: %d %s", e.Status, e.Title)
	}
	return fmt.Sprintf("yellowdog: HTTP %d", e.Status)
}

// IsNotFound is a convenience for callers that need to distinguish a
// missing resource from other failure modes (e.g. cancel a WR that's
// already terminal).
func IsNotFound(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Status == http.StatusNotFound
}

// IsUnauthorized indicates that the credentials were rejected by the
// platform - either malformed, revoked, or scoped to a namespace
// where the requested resource doesn't live.
func IsUnauthorized(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Status == http.StatusUnauthorized || apiErr.Status == http.StatusForbidden
}
