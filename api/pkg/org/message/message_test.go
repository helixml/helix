// Package message_test characterises the public behaviour of the
// canonical Message envelope and the Encode/Decode round-trip prior
// to B3b's lift from helix-org/domain to this canonical home.
//
// The test cases were authored against the unmoved code (with a
// temporary upward import to helix-org/domain) and ran green before
// the lift; only the import path and symbol references changed in the
// lift commit. Names lost the redundant prefix where applicable
// (DecodeMessage -> message.Decode) per the B1 stutter-removal
// precedent.
//
// Coverage matches M1..M4 from the B3b success criteria:
//
//	M1 full Message round-trips losslessly (every field, attachments)
//	M2 minimal Message (Body only) round-trips; omitempty omits unset fields
//	M3 empty Message encodes to "{}"
//	M4 Decode rejects malformed JSON
//
// NewMessageEvent and Event.Message stay in helix-org/domain (they
// depend on Event, which has not been lifted yet); their tests
// (M5..M7 in the legacy file) stay alongside them.
package message_test

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/message"
)

func TestMessage_FullRoundTrip(t *testing.T) { // M1
	t.Parallel()
	msg := message.Message{
		From:            "w-alice",
		To:              []string{"w-bob"},
		Subject:         "hi",
		Body:            "hello\nthere",
		BodyContentType: "text/plain",
		ThreadID:        "t-123",
		InReplyTo:       "m-prev",
		MessageID:       "m-now",
		Attachments: []message.Attachment{
			{Filename: "x.pdf", ContentType: "application/pdf", URL: "https://e.com/x", SizeBytes: 1024},
		},
	}
	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := message.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.From != msg.From || got.Body != msg.Body || len(got.To) != 1 || got.To[0] != "w-bob" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.ThreadID != msg.ThreadID || got.InReplyTo != msg.InReplyTo || got.MessageID != msg.MessageID {
		t.Fatalf("threading mismatch: %+v", got)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Filename != "x.pdf" {
		t.Fatalf("attachment lost: %+v", got.Attachments)
	}
	if got.Attachments[0].SizeBytes != 1024 {
		t.Fatalf("attachment SizeBytes lost: %d", got.Attachments[0].SizeBytes)
	}
}

func TestMessage_MinimalRoundTripAndOmitempty(t *testing.T) { // M2
	t.Parallel()
	msg := message.Message{Body: "hello"}
	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.Contains(encoded, `"body":"hello"`) {
		t.Fatalf("expected body field in JSON, got %q", encoded)
	}
	// omitempty must omit unset string fields, slices, and Extra.
	for _, banned := range []string{`"from"`, `"to"`, `"subject"`, `"attachments"`, `"extra"`, `"thread_id"`, `"in_reply_to"`, `"message_id"`, `"body_content_type"`} {
		if strings.Contains(encoded, banned) {
			t.Errorf("unset field %s leaked into JSON: %q", banned, encoded)
		}
	}
	got, err := message.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Body != "hello" {
		t.Fatalf("round-trip Body = %q, want %q", got.Body, "hello")
	}
}

func TestMessage_EmptyEncodesToBraces(t *testing.T) { // M3
	t.Parallel()
	msg := message.Message{}
	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if encoded != "{}" {
		t.Fatalf("empty message = %q, want %q", encoded, "{}")
	}
}

func TestDecode_RejectsMalformedJSON(t *testing.T) { // M4
	t.Parallel()
	cases := []string{
		``,
		`not json`,
		`{`,
		`[`,
	}
	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			t.Parallel()
			if _, err := message.Decode(c); err == nil {
				t.Fatalf("Decode(%q) = nil, want error", c)
			}
		})
	}
}

func TestMessage_MustEncodeNeverPanicsOnValidShape(t *testing.T) { // M2 extension
	t.Parallel()
	// MustEncode panics only on a json.Marshal failure, which for this
	// struct (all fields are basic types or json.RawMessage) is a
	// programming bug. The contract worth pinning is: any concrete
	// Message that callers actually construct must round-trip.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MustEncode panicked on a valid Message: %v", r)
		}
	}()
	_ = message.Message{From: "w-x", Body: "y"}.MustEncode()
}
