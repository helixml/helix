package main

import (
	"fmt"

	"github.com/helixml/helix-org/server"
)

// apiError is the surfaced form of a jsonapi error returned by /tail.
type apiError struct {
	Status int
	Title  string
	Detail string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("api error (%d): %s — %s", e.Status, e.Title, e.Detail)
}

// apiErrorsEnvelope matches the jsonapi `{ "errors": [...] }` shape.
type apiErrorsEnvelope struct {
	Errors []server.Error `json:"errors"`
}
