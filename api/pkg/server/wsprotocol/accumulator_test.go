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
