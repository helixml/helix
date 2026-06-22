// Package jsonapi is a small, composable toolkit for emitting
// jsonapi.org-formatted responses from the helix-org HTTP interfaces.
//
// The codebase had no JSON:API helpers, so this package provides the
// minimum the topic-messages endpoint needs while staying generic and
// reusable for any future org/interfaces endpoint.
//
// The design is composition-first: a Document is assembled by applying
// independent Components, each of which contributes its own slice
// (meta keys, links, …). Meta and pagination are therefore separate,
// reusable units rather than fields baked into one bespoke response
// struct — compose whichever ones a given endpoint needs:
//
//	doc := jsonapi.NewDocument(resources,
//	    jsonapi.TotalMeta{Total: total},          // meta via composition
//	    jsonapi.Pagination{...},                  // pagination via composition
//	)
//	jsonapi.Write(w, http.StatusOK, doc)
package jsonapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// ContentType is the media type the JSON:API spec mandates.
const ContentType = "application/vnd.api+json"

// Bind decodes a JSON:API request document — `{ "data": { "type": …,
// "attributes": { … } } }` — into attrs (a pointer to the endpoint's
// attribute struct). The wrapper's type/id are ignored here: handlers
// take the resource id from the URL and validate the type by routing.
// Returns an error on malformed JSON or a missing data.attributes
// object, so handlers can map it to 400.
func Bind(r *http.Request, attrs any) error {
	var doc struct {
		Data struct {
			Type       string          `json:"type"`
			ID         string          `json:"id"`
			Attributes json.RawMessage `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty request body; expected a JSON:API document")
		}
		return fmt.Errorf("decode JSON:API document: %w", err)
	}
	if len(doc.Data.Attributes) == 0 {
		return errors.New("missing data.attributes in JSON:API document")
	}
	if err := json.Unmarshal(doc.Data.Attributes, attrs); err != nil {
		return fmt.Errorf("decode data.attributes: %w", err)
	}
	return nil
}

// Meta is the top-level (or resource-level) free-form metadata object.
type Meta map[string]any

// Links is a JSON:API links object (self/first/prev/next/last/…).
type Links map[string]string

// Document is a top-level JSON:API document. Data is the primary data
// (a single Resource, a slice of Resource, or nil); Meta and Links are
// contributed by Components and omitted from the wire when empty.
type Document struct {
	Data  any   `json:"data"`
	Meta  Meta  `json:"meta,omitempty"`
	Links Links `json:"links,omitempty"`
}

// Component contributes its part to a Document. Composition is "apply
// many": NewDocument folds each Component onto the document in turn.
type Component interface {
	Apply(*Document)
}

// NewDocument builds a Document around data and applies each Component.
// Components mutate the document's Meta/Links via the ensure* helpers,
// so the maps only materialise when something actually populates them.
func NewDocument(data any, components ...Component) *Document {
	d := &Document{Data: data}
	for _, c := range components {
		if c != nil {
			c.Apply(d)
		}
	}
	return d
}

// ensureMeta lazily creates the Meta map so Components can write into
// it without each one nil-checking.
func (d *Document) ensureMeta() Meta {
	if d.Meta == nil {
		d.Meta = Meta{}
	}
	return d.Meta
}

// ensureLinks lazily creates the Links map.
func (d *Document) ensureLinks() Links {
	if d.Links == nil {
		d.Links = Links{}
	}
	return d.Links
}

// Resource is a single JSON:API resource object.
type Resource struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes any    `json:"attributes,omitempty"`
}

// TotalMeta is a Component that contributes the total item count to
// meta.total. Kept separate from Pagination so an endpoint can surface
// a total without paging (and vice versa).
type TotalMeta struct {
	Total int
}

// Apply writes total into the document's meta.
func (m TotalMeta) Apply(d *Document) {
	d.ensureMeta()["total"] = m.Total
}

// Write encodes doc as JSON:API with the correct content type and
// status. Encoding errors are swallowed after the header is written —
// the same contract as the sibling api package's writeJSON.
func Write(w http.ResponseWriter, status int, doc *Document) {
	w.Header().Set("Content-Type", ContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(doc)
}

// ErrorObject is a single JSON:API error object. Only the fields the
// helix-org endpoints populate are modelled (status as a string per the
// spec, plus a human-readable detail); title/code/source can be added
// when a consumer needs them.
type ErrorObject struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// ErrorDocument is a top-level JSON:API document carrying errors instead
// of data (the two are mutually exclusive per the spec).
type ErrorDocument struct {
	Errors []ErrorObject `json:"errors"`
}

// WriteError emits a spec-compliant JSON:API error document
// (`{"errors":[{"status","detail"}]}`) with the vnd.api+json content
// type. status is rendered both as the HTTP status and, stringified, in
// the error object's status member.
func WriteError(w http.ResponseWriter, status int, err error) {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	w.Header().Set("Content-Type", ContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorDocument{
		Errors: []ErrorObject{{Status: strconv.Itoa(status), Detail: detail}},
	})
}
