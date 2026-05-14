package notion

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
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

func TestParseAutomationEvent(t *testing.T) {
	body := []byte(`{
		"source": "automation",
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
