// Package notion implements a Helix trigger backed by Notion Database
// Automations and Button properties (primary path) and the Notion API webhook
// subscription (secondary path).
//
// See design/tasks/002021_investigate-notion/ for the full design.
package notion

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// Headers the integration relies on. All three are set by the user when they
// configure the Notion Database Automation / Button "Send webhook" action.
const (
	HeaderSharedSecret = "X-Helix-Webhook-Secret" // primary-path auth
	HeaderSource       = "X-Helix-Source"         // "notion-automation" | "notion-button"
	HeaderAction       = "X-Helix-Action"         // "create" | "cancel"

	HeaderNotionSignature = "X-Notion-Signature" // secondary-path auth
)

const (
	SourceAutomation = "notion-automation"
	SourceButton     = "notion-button"

	ActionCreate = "create"
	ActionCancel = "cancel"
)

// ErrInvalidSignature is returned when an inbound webhook fails authentication.
var ErrInvalidSignature = errors.New("notion: invalid webhook signature")

// VerifySharedSecret performs a constant-time compare of the configured shared
// secret against the X-Helix-Webhook-Secret header. Returns nil on match,
// ErrInvalidSignature otherwise.
func VerifySharedSecret(headers http.Header, configuredSecret string) error {
	if configuredSecret == "" {
		return ErrInvalidSignature
	}
	got := headers.Get(HeaderSharedSecret)
	if subtle.ConstantTimeCompare([]byte(got), []byte(configuredSecret)) != 1 {
		return ErrInvalidSignature
	}
	return nil
}

// VerifyNotionSignature checks the X-Notion-Signature header against an
// HMAC-SHA256 of the raw body keyed by the webhook subscription's
// verification_token (the secret returned during the API webhook subscription
// handshake — secondary path only).
func VerifyNotionSignature(headers http.Header, rawBody []byte, verificationToken string) error {
	if verificationToken == "" {
		return ErrInvalidSignature
	}
	got := headers.Get(HeaderNotionSignature)
	got = strings.TrimPrefix(got, "sha256=")

	mac := hmac.New(sha256.New, []byte(verificationToken))
	mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
		return ErrInvalidSignature
	}
	return nil
}

// AutomationEvent is the shape of the JSON body Notion sends from a Database
// Automation / Button property "Send webhook" action. The user picks which
// fields land in the body when they configure the automation; we deliberately
// only depend on `data.id` (the page ID — Notion always includes it) plus
// whatever properties the user chose.
//
// Properties is the raw map of field-name → property value as Notion serialises
// it; consumers pull values out via Property* helpers.
type AutomationEvent struct {
	Source string         `json:"source"`         // "automation" | "button" | "..."
	Data   AutomationData `json:"data,omitempty"` // Notion's wrapper around the page payload
}

// AutomationData wraps the page identity and the property snapshot Notion
// includes in the webhook body. Notion's exact field names here are
// conservative best-effort and may need adjusting after live discovery — see
// the prerequisite manual webhook.site capture in tasks.md.
type AutomationData struct {
	ID         string                     `json:"id"`           // page ID
	Object     string                     `json:"object"`       // "page"
	Parent     map[string]json.RawMessage `json:"parent"`       // contains database_id
	Properties map[string]json.RawMessage `json:"properties"`   // user-selected fields
}

// ParseAutomationEvent unmarshals a primary-path webhook body. Returns the
// parsed event and the page ID for convenience; the page ID is the only field
// dispatch keys off (alongside the X-Helix-Action header, which is read
// separately).
func ParseAutomationEvent(body []byte) (*AutomationEvent, string, error) {
	var ev AutomationEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, "", err
	}
	return &ev, ev.Data.ID, nil
}

// DatabaseIDFromParent extracts the database_id from the page's parent block.
// Returns empty string if the page is not a database row.
func DatabaseIDFromParent(parent map[string]json.RawMessage) string {
	raw, ok := parent["database_id"]
	if !ok {
		return ""
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return ""
	}
	return id
}
