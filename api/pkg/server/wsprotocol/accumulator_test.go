package wsprotocol

import (
	"testing"
)

func TestFirstMessage(t *testing.T) {
	a := &MessageAccumulator{}
	a.AddMessage("msg-1", "Hello world")

	if a.Content != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", a.Content)
	}
	if a.LastMessageID != "msg-1" {
		t.Errorf("expected LastMessageID 'msg-1', got %q", a.LastMessageID)
	}
	if a.Offset != 0 {
		t.Errorf("expected Offset 0, got %d", a.Offset)
	}
}

func TestSameMessageStreaming(t *testing.T) {
	a := &MessageAccumulator{}
	a.AddMessage("msg-1", "He")
	a.AddMessage("msg-1", "Hello")
	a.AddMessage("msg-1", "Hello world")

	if a.Content != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", a.Content)
	}
}

func TestTwoDistinctMessages(t *testing.T) {
	a := &MessageAccumulator{}
	a.AddMessage("msg-1", "Hello world")
	a.AddMessage("msg-2", "Tool call result")

	expected := "Hello world\n\nTool call result"
	if a.Content != expected {
		t.Errorf("expected %q, got %q", expected, a.Content)
	}
	if a.Offset != len("Hello world")+2 {
		t.Errorf("expected offset %d, got %d", len("Hello world")+2, a.Offset)
	}
}

func TestMultiMessageWithStreaming(t *testing.T) {
	// This is the exact bug scenario:
	// 1. message_added(id="1", content="Hello world")     → "Hello world"
	// 2. message_added(id="2", content="[tool]")           → "Hello world\n\n[tool]"
	// 3. message_added(id="2", content="[tool: edit.py]")  → should be "Hello world\n\n[tool: edit.py]"
	a := &MessageAccumulator{}
	a.AddMessage("msg-1", "Hello world")
	a.AddMessage("msg-2", "[tool]")
	a.AddMessage("msg-2", "[tool: edit.py]")

	expected := "Hello world\n\n[tool: edit.py]"
	if a.Content != expected {
		t.Errorf("expected %q, got %q", expected, a.Content)
	}
}

func TestThreeDistinctMessages(t *testing.T) {
	a := &MessageAccumulator{}
	a.AddMessage("msg-1", "Hello world")
	a.AddMessage("msg-2", "Tool call")
	a.AddMessage("msg-3", "Final response")

	expected := "Hello world\n\nTool call\n\nFinal response"
	if a.Content != expected {
		t.Errorf("expected %q, got %q", expected, a.Content)
	}
}

func TestInterleavedStreamingAndNewMessages(t *testing.T) {
	// Simulate a realistic Zed response:
	// msg-1: assistant message (streams)
	// msg-2: tool call (streams)
	// msg-3: assistant follow-up (streams)
	a := &MessageAccumulator{}

	// msg-1 streams
	a.AddMessage("msg-1", "I'll help")
	a.AddMessage("msg-1", "I'll help you with that.")

	// msg-2 (tool call)
	a.AddMessage("msg-2", "```tool\nedit file.py")
	a.AddMessage("msg-2", "```tool\nedit file.py\n```")

	// msg-3 (follow-up)
	a.AddMessage("msg-3", "Done!")

	expected := "I'll help you with that.\n\n```tool\nedit file.py\n```\n\nDone!"
	if a.Content != expected {
		t.Errorf("expected:\n%s\n\ngot:\n%s", expected, a.Content)
	}
}

func TestStreamingAfterAppendPreservesPrefix(t *testing.T) {
	// Key regression test: streaming update of msg-2 must not destroy msg-1's content
	a := &MessageAccumulator{}
	a.AddMessage("msg-1", "First message content")
	a.AddMessage("msg-2", "Second partial")
	a.AddMessage("msg-2", "Second complete message")

	expected := "First message content\n\nSecond complete message"
	if a.Content != expected {
		t.Errorf("expected %q, got %q", expected, a.Content)
	}

	// Verify the prefix is intact
	prefix := a.Content[:len("First message content")]
	if prefix != "First message content" {
		t.Errorf("prefix corrupted: %q", prefix)
	}
}

func TestEmptyContent(t *testing.T) {
	a := &MessageAccumulator{}
	a.AddMessage("msg-1", "")

	if a.Content != "" {
		t.Errorf("expected empty content, got %q", a.Content)
	}
}

func TestOutOfOrderFlushUpdates(t *testing.T) {
	// THE BUG: Zed's Stopped flush sends corrected content for earlier message_ids
	// after later ones have already been processed. The old accumulator treated
	// these as new appends instead of in-place replacements.
	//
	// Real scenario from Zed logs:
	// During streaming (throttled, content truncated mid-word):
	//   id="2"  "I'll start...understand the c"
	//   id="3"  "**Tool Call: List the `clea` directory's contents**"
	//   id="4"  "**Tool Call: List the `helix-specs/d`..."
	//   ...
	//   id="18" "The design docs have been pushed..."
	//
	// During Stopped flush (complete content, OUT OF ORDER):
	//   id="2"  "I'll start...understand the codebase structure..."  ← MUST REPLACE, not append
	//   id="3"  "**Tool Call: List the `clean-truncation-test`..."   ← MUST REPLACE, not append
	a := &MessageAccumulator{}

	// Streaming phase: entries arrive in order but with truncated content
	a.AddMessage("2", "I'll start...understand the c")
	a.AddMessage("3", "**Tool Call: List the `clea`**\nStatus: Pending")
	a.AddMessage("4", "**Tool Call: List the `helix-specs/d`**\nStatus: Pending")
	a.AddMessage("6", "The repo is very")
	a.AddMessage("18", "The design docs")

	// Verify truncated state
	if a.Content != "I'll start...understand the c\n\n**Tool Call: List the `clea`**\nStatus: Pending\n\n**Tool Call: List the `helix-specs/d`**\nStatus: Pending\n\nThe repo is very\n\nThe design docs" {
		t.Fatalf("unexpected truncated state:\n%s", a.Content)
	}

	// Flush phase: corrected content arrives for earlier message_ids
	a.AddMessage("2", "I'll start...understand the codebase structure")
	a.AddMessage("3", "**Tool Call: List the `clean-truncation-test`**\nStatus: Completed")
	a.AddMessage("6", "The repo is very minimal — just a README.")
	a.AddMessage("18", "The design docs have been pushed and are ready for review.")

	expected := "I'll start...understand the codebase structure\n\n**Tool Call: List the `clean-truncation-test`**\nStatus: Completed\n\n**Tool Call: List the `helix-specs/d`**\nStatus: Pending\n\nThe repo is very minimal — just a README.\n\nThe design docs have been pushed and are ready for review."
	if a.Content != expected {
		t.Errorf("out-of-order flush failed.\nexpected:\n%s\n\ngot:\n%s", expected, a.Content)
	}
}

func TestFlushDoesNotDuplicateContent(t *testing.T) {
	// Verify that updating an earlier message_id does NOT append a duplicate —
	// it must replace the content at the original position.
	a := &MessageAccumulator{}
	a.AddMessage("1", "first")
	a.AddMessage("2", "second")
	a.AddMessage("3", "third")

	// "Update" message 1 with new content
	a.AddMessage("1", "FIRST (corrected)")

	expected := "FIRST (corrected)\n\nsecond\n\nthird"
	if a.Content != expected {
		t.Errorf("expected %q, got %q", expected, a.Content)
	}

	// Verify no duplication: count occurrences of "FIRST"
	count := 0
	for i := 0; i < len(a.Content)-4; i++ {
		if a.Content[i:i+5] == "FIRST" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'FIRST' to appear once, appeared %d times in: %q", count, a.Content)
	}
}

func TestResumeFromPersistedState(t *testing.T) {
	// Simulate restoring state from DB after API restart
	a := &MessageAccumulator{
		Content:       "Previous message\n\nStreaming...",
		LastMessageID: "msg-2",
		Offset:        len("Previous message") + 2,
	}

	// Continue streaming msg-2
	a.AddMessage("msg-2", "Streaming complete")

	expected := "Previous message\n\nStreaming complete"
	if a.Content != expected {
		t.Errorf("expected %q, got %q", expected, a.Content)
	}

	// New msg-3
	a.AddMessage("msg-3", "Final")

	expected = "Previous message\n\nStreaming complete\n\nFinal"
	if a.Content != expected {
		t.Errorf("expected %q, got %q", expected, a.Content)
	}
}
