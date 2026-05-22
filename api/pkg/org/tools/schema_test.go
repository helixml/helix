package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/domain"
)

// TestHireWorkerSchemaSurfacesKindEnum pins the contract that the
// `kind` field appears as a JSON-Schema enum on the hire_worker input
// schema. MCP clients (Claude Code, etc.) consume this directly, so the
// LLM sees the valid values up front rather than discovering them via
// a server-side validation error round-trip.
func TestHireWorkerSchemaSurfacesKindEnum(t *testing.T) {
	t.Parallel()
	schema := (&HireWorker{}).InputSchema()
	props, ok := schema.Properties["kind"]
	if !ok {
		t.Fatalf("kind not in schema properties: %+v", schema.Properties)
	}
	if props.Type != "string" {
		t.Errorf("kind type = %q, want string", props.Type)
	}
	got := make(map[string]bool, len(props.Enum))
	for _, v := range props.Enum {
		s, _ := v.(string)
		got[s] = true
	}
	for _, want := range []string{"human", "ai"} {
		if !got[want] {
			t.Errorf("kind enum missing %q (got %v)", want, props.Enum)
		}
	}
	// And nothing extra: "claude" famously does not belong here.
	if got["claude"] {
		t.Errorf("kind enum unexpectedly contains \"claude\"")
	}
}

// TestHireWorkerInvokeRejectsUnknownKindWithValidList exercises the
// runtime safety net for clients that ignore the schema and post a bad
// `kind` anyway. The error must list the valid values verbatim — that
// is the contract that lets a self-correcting agent retry without
// reading source.
func TestHireWorkerInvokeRejectsUnknownKindWithValidList(t *testing.T) {
	t.Parallel()
	tool := &HireWorker{deps: Deps{EnvsDir: t.TempDir()}}
	args, _ := json.Marshal(map[string]any{
		"id":              "w-bad",
		"positionId":      "p-x",
		"kind":            "claude",
		"identityContent": "hi",
	})
	_, err := tool.Invoke(context.Background(), domain.Invocation{Args: args})
	if err == nil {
		t.Fatal("Invoke = nil, want error")
	}
	for _, want := range []string{`"human"`, `"ai"`, "claude"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to mention %q", err, want)
		}
	}
}

// TestQuotedListRendersDomainEnums covers the helper used by every
// "valid: …" error. Pin both the WorkerKind and TransportKind cases so
// a future enum addition that breaks the generic constraint is caught
// here, not in a Slack-channel report.
func TestQuotedListRendersDomainEnums(t *testing.T) {
	t.Parallel()
	if got := domain.QuotedList(worker.KindValues()); got != `"human", "ai"` {
		t.Errorf("WorkerKind QuotedList = %q", got)
	}
	if got := domain.QuotedList(transport.KindValues()); !strings.Contains(got, `"local"`) || !strings.Contains(got, `"github"`) {
		t.Errorf("TransportKind QuotedList = %q (missing local or github)", got)
	}
}
