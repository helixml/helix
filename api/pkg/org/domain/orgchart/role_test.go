// Package role_test characterises Role's public behaviour: the New
// constructor's validation rules, the immutability of timestamps after
// construction, and (new in B7) the typed Tools and Streams fields.
//
// Coverage versus the legacy helix-org/domain/role_test.go: every case
// the legacy file pinned is preserved (valid, empty id, empty content,
// zero time), and the new Tools / Streams fields get their own
// validation cases (nil, empty, populated, mixed). The legacy file is
// deleted in this same commit.
package orgchart_test

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// --- Legacy characterisation cases (preserved verbatim in spirit) -------

func TestNew_AcceptsValidInputs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	r, err := orgchart.NewRole("r-ceo", "# CEO\nMakes calls.", nil, nil, now, "org-test")
	if err != nil {
		t.Fatalf("New() = %v, want nil", err)
	}
	if r.ID != "r-ceo" {
		t.Fatalf("ID = %q, want %q", r.ID, "r-ceo")
	}
	if r.Content != "# CEO\nMakes calls." {
		t.Fatalf("Content = %q", r.Content)
	}
	if !r.CreatedAt.Equal(now) || !r.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps not set: created=%v updated=%v", r.CreatedAt, r.UpdatedAt)
	}
}

func TestNew_RejectsEmptyID(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	_, err := orgchart.NewRole("", "# CEO", nil, nil, now, "org-test")
	if err == nil {
		t.Fatal("New() with empty id: want error, got nil")
	}
}

func TestNew_RejectsEmptyContent(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	_, err := orgchart.NewRole("r-ceo", "", nil, nil, now, "org-test")
	if err == nil {
		t.Fatal("New() with empty content: want error, got nil")
	}
}

func TestNew_RejectsZeroTime(t *testing.T) {
	t.Parallel()
	_, err := orgchart.NewRole("r-ceo", "# CEO", nil, nil, time.Time{}, "org-test")
	if err == nil {
		t.Fatal("New() with zero time: want error, got nil")
	}
}

// --- Tools and Streams (new in B7) --------------------------------------

func TestNew_NilToolsAndStreamsAreValid(t *testing.T) {
	t.Parallel()
	// A Role with no declared tools or streams is valid — the hiring
	// caller's prompt is responsible for figuring out what to grant
	// and subscribe from Content alone.
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	r, err := orgchart.NewRole("r-minimal", "# Minimal", nil, nil, now, "org-test")
	if err != nil {
		t.Fatalf("New() = %v, want nil", err)
	}
	if r.Tools != nil {
		t.Fatalf("Tools = %v, want nil", r.Tools)
	}
	if r.Streams != nil {
		t.Fatalf("Streams = %v, want nil", r.Streams)
	}
}

func TestNew_EmptyToolsAndStreamsAreValid(t *testing.T) {
	t.Parallel()
	// Empty slices are equivalent to nil — neither is invalid.
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	r, err := orgchart.NewRole("r-minimal", "# Minimal", []tool.Name{}, []streaming.StreamID{}, now, "org-test")
	if err != nil {
		t.Fatalf("New() = %v, want nil", err)
	}
	if len(r.Tools) != 0 {
		t.Fatalf("Tools = %v, want empty", r.Tools)
	}
	if len(r.Streams) != 0 {
		t.Fatalf("Streams = %v, want empty", r.Streams)
	}
}

func TestNew_PopulatedToolsAndStreamsRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	tools := []tool.Name{"read_events", "publish", "subscribe"}
	streams := []streaming.StreamID{"s-general", "s-inbox"}
	r, err := orgchart.NewRole("r-secretary", "# Secretary", tools, streams, now, "org-test")
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	if !sliceEqual(r.Tools, tools) {
		t.Fatalf("Tools = %v, want %v", r.Tools, tools)
	}
	if !sliceEqualStream(r.Streams, streams) {
		t.Fatalf("Streams = %v, want %v", r.Streams, streams)
	}
}

func TestNew_OnlyToolsDeclared(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	tools := []tool.Name{"hire_worker"}
	r, err := orgchart.NewRole("r-recruiter", "# Recruiter", tools, nil, now, "org-test")
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	if len(r.Tools) != 1 || r.Tools[0] != "hire_worker" {
		t.Fatalf("Tools = %v, want [hire_worker]", r.Tools)
	}
	if r.Streams != nil {
		t.Fatalf("Streams = %v, want nil", r.Streams)
	}
}

func TestNew_OnlyStreamsDeclared(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	streams := []streaming.StreamID{"s-broadcast"}
	r, err := orgchart.NewRole("r-watcher", "# Watcher", nil, streams, now, "org-test")
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	if r.Tools != nil {
		t.Fatalf("Tools = %v, want nil", r.Tools)
	}
	if len(r.Streams) != 1 || r.Streams[0] != "s-broadcast" {
		t.Fatalf("Streams = %v, want [s-broadcast]", r.Streams)
	}
}

// --- helpers ------------------------------------------------------------

func sliceEqual(a, b []tool.Name) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sliceEqualStream(a, b []streaming.StreamID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
