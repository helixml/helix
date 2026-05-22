package helix

import "testing"

// TestEntryStreamTextSettlesOnReplace verifies the core invariant: a
// text entry is held open until its slot is replaced by a different
// MessageID, then the full accumulated content is emitted.
func TestEntryStreamTextSettlesOnReplace(t *testing.T) {
	t.Parallel()
	var got []Event
	s := NewEntryStream(func(e Event) { got = append(got, e) })

	// Three append-only patches against the same MessageID.
	s.Apply(SessionUpdate{EntryPatches: []EntryPatch{
		{Index: 0, MessageID: "m1", Type: "text", Patch: "Hello", PatchOffset: 0},
	}})
	s.Apply(SessionUpdate{EntryPatches: []EntryPatch{
		{Index: 0, MessageID: "m1", Type: "text", Patch: " world", PatchOffset: 5},
	}})
	if len(got) != 0 {
		t.Fatalf("expected no emits while text is still streaming, got %v", got)
	}

	// New MessageID at same index: previous text settles.
	s.Apply(SessionUpdate{EntryPatches: []EntryPatch{
		{Index: 0, MessageID: "m2", Type: "tool_call", Patch: `{"x":1}`, PatchOffset: 0, ToolName: "publish", ToolStatus: "In Progress"},
	}})
	if len(got) != 2 {
		t.Fatalf("expected 2 events (assistant text, tool_use), got %v", got)
	}
	if got[0].Kind != EventAssistant || got[0].Text != "Hello world" {
		t.Errorf("text: %+v", got[0])
	}
	if got[1].Kind != EventToolUse || got[1].ToolName != "publish" {
		t.Errorf("tool_use: %+v", got[1])
	}
}

// TestEntryStreamToolCompletes verifies that a tool_call entry seals
// when ToolStatus reaches Completed.
func TestEntryStreamToolCompletes(t *testing.T) {
	t.Parallel()
	var got []Event
	s := NewEntryStream(func(e Event) { got = append(got, e) })

	s.Apply(SessionUpdate{EntryPatches: []EntryPatch{
		{Index: 0, MessageID: "t1", Type: "tool_call", Patch: `{"x":1}`, PatchOffset: 0, ToolName: "fetch", ToolStatus: "In Progress"},
	}})
	s.Apply(SessionUpdate{EntryPatches: []EntryPatch{
		{Index: 0, MessageID: "t1", Type: "tool_call", Patch: ` ok`, PatchOffset: 7, ToolStatus: "Completed"},
	}})
	if len(got) != 2 {
		t.Fatalf("expected tool_use + tool_result, got %v", got)
	}
	if got[1].Kind != EventToolResult || got[1].Text != `{"x":1} ok` {
		t.Errorf("tool_result: %+v", got[1])
	}
}

// TestEntryStreamToolFailedEmitsError verifies that ToolStatus=Failed
// produces a tool_result-error rather than tool_result.
func TestEntryStreamToolFailedEmitsError(t *testing.T) {
	t.Parallel()
	var got []Event
	s := NewEntryStream(func(e Event) { got = append(got, e) })

	s.Apply(SessionUpdate{EntryPatches: []EntryPatch{
		{Index: 0, MessageID: "t1", Type: "tool_call", Patch: "boom", ToolName: "x", ToolStatus: "In Progress"},
	}})
	s.Apply(SessionUpdate{EntryPatches: []EntryPatch{
		{Index: 0, MessageID: "t1", Type: "tool_call", ToolStatus: "Failed"},
	}})
	if got[len(got)-1].Kind != EventToolResultError {
		t.Errorf("expected tool_result-error, got %+v", got[len(got)-1])
	}
}

// TestEntryStreamFlushSealsOpenText verifies Flush emits any open
// text entries, e.g. on session terminal state.
func TestEntryStreamFlushSealsOpenText(t *testing.T) {
	t.Parallel()
	var got []Event
	s := NewEntryStream(func(e Event) { got = append(got, e) })
	s.Apply(SessionUpdate{EntryPatches: []EntryPatch{
		{Index: 0, MessageID: "m1", Type: "text", Patch: "answer", PatchOffset: 0},
	}})
	s.Flush()
	if len(got) != 1 || got[0].Kind != EventAssistant || got[0].Text != "answer" {
		t.Errorf("unexpected events: %v", got)
	}
}

// TestEntryStreamSnapshotReplayDoesNotDoubleEmit verifies that
// re-applying the same patches (e.g. a late-joiner snapshot) doesn't
// duplicate events. The MessageID matches and PatchOffset starts
// from 0 with full content; the splice clamps so content reaches a
// stable end-state without producing extra emits.
func TestEntryStreamSnapshotReplayDoesNotDoubleEmit(t *testing.T) {
	t.Parallel()
	var got []Event
	s := NewEntryStream(func(e Event) { got = append(got, e) })
	patches := []EntryPatch{
		{Index: 0, MessageID: "t1", Type: "tool_call", Patch: `{"x":1}`, PatchOffset: 0, ToolName: "fetch", ToolStatus: "In Progress"},
		{Index: 0, MessageID: "t1", Type: "tool_call", Patch: ` done`, PatchOffset: 7, ToolStatus: "Completed"},
	}
	s.Apply(SessionUpdate{EntryPatches: patches})
	first := append([]Event(nil), got...)
	// Replay same patches.
	s.Apply(SessionUpdate{EntryPatches: patches})
	if len(got) != len(first) {
		t.Errorf("snapshot replay double-emitted: %d → %d events", len(first), len(got))
	}
}

// TestEntryStreamInteractionErrorEmitsErrorEvent verifies that an
// `interaction_update` with State="error" surfaces an error event.
func TestEntryStreamInteractionErrorEmitsErrorEvent(t *testing.T) {
	t.Parallel()
	var got []Event
	s := NewEntryStream(func(e Event) { got = append(got, e) })
	s.Apply(SessionUpdate{Interaction: &Interaction{State: "error", Error: "boom"}})
	if len(got) != 1 || got[0].Kind != EventError || got[0].Text != "boom" {
		t.Errorf("expected one error event, got %v", got)
	}
}
