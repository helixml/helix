package transport

import (
	"encoding/json"
	"errors"
	"fmt"
)

// KindEmail is a bidirectional email transport. Provider credentials
// live at server level (see config.transport.postmark); per-stream
// config carries only the routing identity (`alias`).
//
// Inbound: an external service (Postmark today) POSTs parsed inbound
// mail to /email/<provider>; the transport reads the recipient
// address, extracts the alias, and routes to the matching Stream. The
// body becomes a Message envelope on that Stream — From=sender,
// To=[recipient], Subject, Body, MessageID, InReplyTo, ThreadID
// populated from the email's headers.
//
// Outbound: every Event appended to an email Stream is rendered to a
// provider API call (Postmark /email today). The Message envelope's
// From/To/Subject/InReplyTo/ThreadID drive the outbound headers; the
// global `from` from server config is the envelope sender unless the
// Stream's Message specifies otherwise.
const KindEmail Kind = "email"

// EmailConfig is the parsed shape of Transport.Config when
// Kind == KindEmail. Provider credentials live in server-level config;
// the only per-stream knob is the alias used for routing.
type EmailConfig struct {
	// Alias is the routing identifier for this Stream. Inbound mail
	// addressed to <hash>+<alias>@inbound.postmarkapp.com (no-domain
	// path) or <alias>@yourdomain.com (with-domain path) lands on
	// this Stream. Required and unique within the installation.
	Alias string `json:"alias,omitempty"`
}

// Validate enforces that Alias is present and uses the conservative
// alias shape — see isValidEmailAlias.
func (e EmailConfig) Validate() error {
	if e.Alias == "" {
		return errors.New("email transport: alias is required")
	}
	if !isValidEmailAlias(e.Alias) {
		return fmt.Errorf("email transport: alias %q must be lowercase alphanumeric / dash / underscore (no @, +, dots)", e.Alias)
	}
	return nil
}

// EmailConfig parses t.Config as an EmailConfig. Same shape and
// semantics as Transport.WebhookConfig() — see webhook.go.
func (t Transport) EmailConfig() (EmailConfig, error) {
	if t.Kind != KindEmail {
		return EmailConfig{}, fmt.Errorf("transport kind is %q, not email", t.Kind)
	}
	return parseEmailConfig(t.Config)
}

// email is the Strategy for KindEmail.
type email struct{}

// ParseConfig satisfies Strategy.
func (email) ParseConfig(raw json.RawMessage) (Config, error) {
	c, err := parseEmailConfig(raw)
	return c, err
}

// parseEmailConfig is the typed parser. Returns the zero value with
// no error when Config is empty.
func parseEmailConfig(raw json.RawMessage) (EmailConfig, error) {
	var c EmailConfig
	if len(raw) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return EmailConfig{}, fmt.Errorf("parse email config: %w", err)
	}
	return c, nil
}

// isValidEmailAlias enforces a conservative alias shape so it can be
// safely composed into addresses (`<hash>+<alias>@…` or
// `<alias>@yourdomain.com`) without ambiguity. ASCII letters, digits,
// dash, underscore. No `+` (we use it as the separator), no `@`, no
// `.` (avoids subaddress-of-subaddress confusion), no whitespace.
func isValidEmailAlias(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}
