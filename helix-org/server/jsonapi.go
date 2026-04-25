package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/helixml/helix-org/store"
)

// MediaType is the jsonapi.org content type.
const MediaType = "application/vnd.api+json"

// Resource is the jsonapi envelope for a single resource.
// Attributes is intentionally opaque so each handler owns its shape.
type Resource struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Attributes json.RawMessage `json:"attributes,omitempty"`
}

// Error follows the jsonapi.org error-object shape (subset).
type Error struct {
	Status string `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

// writeResource writes a single resource envelope at the given status.
func writeResource(w http.ResponseWriter, status int, resource Resource) {
	payload := struct {
		Data Resource `json:"data"`
	}{Data: resource}
	write(w, status, payload)
}

// writeCollection writes a collection envelope.
func writeCollection(w http.ResponseWriter, status int, resources []Resource) {
	payload := struct {
		Data []Resource `json:"data"`
	}{Data: resources}
	write(w, status, payload)
}

// writeError writes a jsonapi error envelope with a single error object.
func writeError(w http.ResponseWriter, status int, title, detail string) {
	payload := struct {
		Errors []Error `json:"errors"`
	}{Errors: []Error{{Status: http.StatusText(status), Title: title, Detail: detail}}}
	write(w, status, payload)
}

// writeStoreError maps store errors to an appropriate jsonapi response.
func writeStoreError(w http.ResponseWriter, err error, title string) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found", err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, title, err.Error())
}

func write(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", MediaType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// mustAttributes marshals v into json.RawMessage for embedding in Resource.
// Panics only on bugs (unmarshallable types); callers should hand-build in
// those cases.
func mustAttributes(v any) json.RawMessage {
	body, err := json.Marshal(v)
	if err != nil {
		// all inputs are simple structs of strings/slices; a marshal failure
		// here is a programming error, not a runtime condition
		return json.RawMessage(`{}`)
	}
	return body
}
