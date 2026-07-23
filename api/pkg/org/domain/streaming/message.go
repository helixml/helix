package streaming

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Message is the canonical Topic payload. It is always carried as
// JSON in Event.Body — the system stores no other shape.
//
// Identity convention:
//   - From / To carry transport-native identifiers verbatim.
//     orgchart.BotID ("w-alice") when the originator is a known
//     internal Worker; transport-native otherwise
//     ("alice@example.com", "U0123ABCD", "+15551234567", "thermo-3").
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

	// ReplyHint is transport-authored guidance, rendered into the
	// recipient Worker's activation prompt, on how to respond to this
	// message through its origin transport — e.g. for Slack, publish basic
	// text through a configured Topic or mint a token for rich actions.
	// The inbound transport sets it (with the concrete
	// coordinates baked in); it rides through routing like the rest of the
	// envelope, so a Worker reached via a processor still knows how to
	// reply. Empty for in-process Topics with no external egress.
	ReplyHint string `json:"reply_hint,omitempty"`
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

// DecodeMessage parses the canonical JSON form back into a Message.
// Returns an error on malformed JSON; missing fields are zero-valued.
func DecodeMessage(encoded string) (Message, error) {
	var m Message
	if err := json.Unmarshal([]byte(encoded), &m); err != nil {
		return Message{}, fmt.Errorf("decode message: %w", err)
	}
	return m, nil
}

// workerIDPrefix is the canonical prefix every internal Worker ID
// carries (per ADR-0001 / orgchart.NewHumanWorker / NewAIWorker
// conventions). FromPrincipal uses it to disambiguate Worker IDs
// from transport-native sender strings.
const workerIDPrefix = "w-"

// FromPrincipal returns the Message's From field as a typed
// Principal. Inference rules:
//
//   - empty From → zero-Principal (no sender, system-emitted)
//   - From starts with "w-" → KindWorker
//   - anything else → KindTransport
func (m Message) FromPrincipal() Principal {
	if m.From == "" {
		return Principal{}
	}
	if strings.HasPrefix(m.From, workerIDPrefix) {
		return NewPrincipalWorker(m.From)
	}
	return NewPrincipalTransport(m.From)
}
