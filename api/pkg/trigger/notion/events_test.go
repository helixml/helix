package notion

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

func TestVerifySharedSecret(t *testing.T) {
	cases := []struct {
		name     string
		header   string
		expected string
		wantErr  bool
	}{
		{name: "match", header: "abc123", expected: "abc123", wantErr: false},
		{name: "mismatch", header: "abc123", expected: "different", wantErr: true},
		{name: "empty header", header: "", expected: "abc123", wantErr: true},
		{name: "empty configured secret", header: "abc123", expected: "", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := http.Header{}
			h.Set(HeaderSharedSecret, c.header)
			err := VerifySharedSecret(h, c.expected)
			if (err != nil) != c.wantErr {
				t.Fatalf("VerifySharedSecret err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}

func TestVerifyNotionSignature(t *testing.T) {
	body := []byte(`{"hello":"notion"}`)
	token := "verification-token-xyz"

	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(body)
	good := hex.EncodeToString(mac.Sum(nil))

	t.Run("good with sha256= prefix", func(t *testing.T) {
		h := http.Header{}
		h.Set(HeaderNotionSignature, "sha256="+good)
		if err := VerifyNotionSignature(h, body, token); err != nil {
			t.Fatalf("expected ok, got %v", err)
		}
	})

	t.Run("good without prefix", func(t *testing.T) {
		h := http.Header{}
		h.Set(HeaderNotionSignature, good)
		if err := VerifyNotionSignature(h, body, token); err != nil {
			t.Fatalf("expected ok, got %v", err)
		}
	})

	t.Run("mismatched body", func(t *testing.T) {
		h := http.Header{}
		h.Set(HeaderNotionSignature, "sha256="+good)
		if err := VerifyNotionSignature(h, []byte("tampered"), token); err == nil {
			t.Fatal("expected error for tampered body")
		}
	})

	t.Run("mismatched token", func(t *testing.T) {
		h := http.Header{}
		h.Set(HeaderNotionSignature, "sha256="+good)
		if err := VerifyNotionSignature(h, body, "wrong-token"); err == nil {
			t.Fatal("expected error for wrong token")
		}
	})

	t.Run("empty token rejected", func(t *testing.T) {
		h := http.Header{}
		h.Set(HeaderNotionSignature, "sha256="+good)
		if err := VerifyNotionSignature(h, body, ""); err == nil {
			t.Fatal("expected error for empty configured token")
		}
	})
}

// TestParseAutomationEvent_LiveFixture parses the actual webhook body Notion
// sent during live verification on 2026-05-15. Captured at
// testdata/automation_webhook_create.json. If Notion changes their wire
// format this test will catch it before users hit it in production.
func TestParseAutomationEvent_LiveFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/automation_webhook_create.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture struct {
		Headers map[string][]string `json:"headers"`
		Body    json.RawMessage     `json:"body"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode fixture wrapper: %v", err)
	}

	ev, pageID, err := ParseAutomationEvent(fixture.Body)
	if err != nil {
		t.Fatalf("ParseAutomationEvent on live fixture: %v", err)
	}
	if pageID != "36100975-e919-81bb-a013-e800f1c31aa7" {
		t.Errorf("pageID = %q", pageID)
	}
	if ev.Source.Type != "automation" {
		t.Errorf("source.type = %q, want automation", ev.Source.Type)
	}
	if ev.Source.AutomationID == "" || ev.Source.ActionID == "" || ev.Source.EventID == "" {
		t.Errorf("source IDs missing: %+v", ev.Source)
	}
	if got := DatabaseIDFromParent(ev.Data.Parent); got != "36100975-e919-81c8-9d72-d7ee85ed47c2" {
		t.Errorf("DatabaseIDFromParent = %q", got)
	}
	if got := extractTitle(ev); got != "Investigate Notion integration" {
		t.Errorf("extractTitle = %q", got)
	}
	if got := extractPrompt(ev, "Prompt"); got != "Test row for verifying the Helix → Notion webhook flow" {
		t.Errorf("extractPrompt = %q", got)
	}
	// Sanity-check the Helix headers Notion forwarded.
	wantHeaders := map[string]string{
		"x-helix-action":         "create",
		"x-helix-source":         "notion-automation",
		"x-helix-webhook-secret": "helix-test-secret-12345",
	}
	for k, want := range wantHeaders {
		got := fixture.Headers[k]
		if len(got) != 1 || got[0] != want {
			t.Errorf("header %s = %v, want [%s]", k, got, want)
		}
	}
}

func TestParseAutomationEvent(t *testing.T) {
	body := []byte(`{
		"source": {"type": "automation", "automation_id": "a1", "action_id": "x1", "event_id": "e1", "attempt": 1},
		"data": {
			"id": "page-abc-123",
			"object": "page",
			"parent": {"database_id": "db-xyz-789"},
			"properties": {
				"Name": {"type": "title", "title": [{"plain_text": "Hello"}]},
				"Prompt": {"type": "rich_text", "rich_text": [{"plain_text": "Do the thing"}]}
			}
		}
	}`)

	ev, pageID, err := ParseAutomationEvent(body)
	if err != nil {
		t.Fatalf("ParseAutomationEvent error: %v", err)
	}
	if pageID != "page-abc-123" {
		t.Errorf("pageID = %q, want page-abc-123", pageID)
	}
	if got := DatabaseIDFromParent(ev.Data.Parent); got != "db-xyz-789" {
		t.Errorf("DatabaseIDFromParent = %q, want db-xyz-789", got)
	}
	if got := extractTitle(ev); got != "Hello" {
		t.Errorf("extractTitle = %q, want Hello", got)
	}
	if got := extractPrompt(ev, "Prompt"); got != "Do the thing" {
		t.Errorf("extractPrompt = %q, want 'Do the thing'", got)
	}
	if got := extractPrompt(ev, "Missing"); got != "" {
		t.Errorf("extractPrompt(missing column) = %q, want empty", got)
	}
}
