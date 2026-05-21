// Package message owns the canonical Stream payload — the Message
// envelope every Event in the org carries in its Body, and the
// Attachment value type those Messages reference. Transports translate
// provider-native shapes (an email Body, a Slack message, a webhook
// POST body) to and from Message at the transport boundary, so a
// Worker reading any Stream sees the same envelope regardless of
// which transport delivered the Event.
//
// Canonical home, lifted from helix-org/domain/message.go in B3b.
// The NewMessageEvent constructor and the Event.Message() accessor
// stay in helix-org/domain because they depend on the Event struct,
// which has not been lifted yet — those will move when the Event
// concept lifts in a later migration.
//
// Naming: message.Decode drops to message.Decode here (the
// package qualifier already supplies the type, per the B1
// stutter-removal precedent). The Message and Attachment type names
// stay — they match the package's noun.
package message

import (
	"encoding/json"
	"fmt"
)

// Message is the canonical Stream payload. It is always carried as
// JSON in Event.Body — the system stores no other shape.
//
// Identity convention:
//   - From / To carry transport-native identifiers verbatim. worker.ID
//     ("w-alice") when the originator is a known internal Worker;
//     transport-native otherwise ("alice@example.com", "U0123ABCD",
//     "+15551234567", "thermo-3"). No prefixes — Stream context plus
//     value shape is enough to disambiguate.
//   - Empty From means "no human or named originator" — typical for
//     data feeds (RSS, alerts, cron, IoT).
//
// All fields except Body are optional in practice; an event with only
// Body set is a valid plain text message; an empty Message (encoding
// to "{}") is a valid pure-trigger pulse.
type Message struct {
	From            string          `json:"from,omitempty"`
	To              []string        `json:"to,omitempty"`
	Subject         string          `json:"subject,omitempty"`
	Body            string          `json:"body,omitempty"`
	BodyContentType string          `json:"body_content_type,omitempty"`
	ThreadID        string          `json:"thread_id,omitempty"`
	InReplyTo       string          `json:"in_reply_to,omitempty"`
	MessageID       string          `json:"message_id,omitempty"`
	Attachments     []Attachment    `json:"attachments,omitempty"`
	Extra           json.RawMessage `json:"extra,omitempty"`
}

// Attachment is a pointer to bytes the Message references — never
// the bytes themselves. Inbound transports record the provider's URL
// (CDN, signed URL); an object-store integration to take ownership of
// the bytes is a future concern.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	URL         string `json:"url,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
}

// Encode marshals the Message to its canonical JSON form for storage
// in Event.Body. Returns an error only on JSON encoding failure,
// which for this struct is a programming bug.
func (m Message) Encode() (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("encode message: %w", err)
	}
	return string(b), nil
}

// MustEncode is Encode that panics on error. Convenient at call sites
// where the inputs are known-safe (literal strings, validated IDs).
func (m Message) MustEncode() string {
	s, err := m.Encode()
	if err != nil {
		panic(err)
	}
	return s
}

// Decode parses the canonical JSON form back into a Message. Returns
// an error on malformed JSON; missing fields are zero-valued (no
// required-field validation here — Workers may emit Messages with
// only Body set, and that's valid). Renamed from DecodeMessage on the
// way through (B1 stutter-removal precedent).
func Decode(encoded string) (Message, error) {
	var m Message
	if err := json.Unmarshal([]byte(encoded), &m); err != nil {
		return Message{}, fmt.Errorf("decode message: %w", err)
	}
	return m, nil
}
