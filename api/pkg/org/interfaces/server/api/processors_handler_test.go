package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// jsonapiDoc wraps attributes in a JSON:API request document.
func jsonapiDoc(typ string, attrs map[string]any) map[string]any {
	return map[string]any{"data": map[string]any{"type": typ, "attributes": attrs}}
}

func seedTrigger(t *testing.T, st *store.Store, id, name string) {
	t.Helper()
	row, err := trigger.New(id, "org-test", name, "", transport.KindLocal, nil, "", time.Now().UTC())
	if err != nil {
		t.Fatalf("new trigger: %v", err)
	}
	if err := st.Triggers.Create(context.Background(), row); err != nil {
		t.Fatalf("create trigger %s: %v", id, err)
	}
}

func TestCreateProcessorReturnsResourceAndBranch(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTrigger(t, st, "s-in", "Inbox")

	rec := do(t, h, "POST", "/processors", jsonapiDoc("processors", map[string]any{
		"name":         "Formatter",
		"input_source": "trigger:s-in",
		"kind":         "template",
		"config":       map[string]string{"template": "From {{ .Message.from }}: {{ .Message.body }}"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Data struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				InputSource string `json:"input_source"`
				Kind        string `json:"kind"`
				Outputs     []struct {
					ID     string `json:"id"`
					Source string `json:"source"`
				} `json:"outputs"`
			} `json:"attributes"`
		} `json:"data"`
	}
	decode(t, rec, &doc)
	if doc.Data.Type != "processors" || doc.Data.ID == "" {
		t.Fatalf("bad resource: %+v", doc.Data)
	}
	if len(doc.Data.Attributes.Outputs) != 1 || doc.Data.Attributes.Outputs[0].ID == "" {
		t.Fatalf("expected one branch with a durable id, got %+v", doc.Data.Attributes.Outputs)
	}
	// The branch advertises the handle an attachment addresses it by.
	if want := "processor_output:" + doc.Data.ID + ":" + doc.Data.Attributes.Outputs[0].ID; doc.Data.Attributes.Outputs[0].Source != want {
		t.Errorf("branch source = %q, want %q", doc.Data.Attributes.Outputs[0].Source, want)
	}
	if doc.Data.Attributes.InputSource != "trigger:s-in" {
		t.Errorf("input_source = %q, want trigger:s-in", doc.Data.Attributes.InputSource)
	}
}

func TestProcessorCRUDLifecycle(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTrigger(t, st, "s-in", "Inbox")

	// Create (server mints the id; capture it).
	rec := do(t, h, "POST", "/processors", jsonapiDoc("processors", map[string]any{
		"name": "Life", "input_source": "trigger:s-in", "kind": "template",
		"config": map[string]string{"template": "{{ .Message.body }}"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	decode(t, rec, &created)
	id := created.Data.ID
	if id == "" {
		t.Fatal("created processor has no id")
	}

	// Get.
	if rec := do(t, h, "GET", "/processors/"+id, nil); rec.Code != http.StatusOK {
		t.Fatalf("get = %d: %s", rec.Code, rec.Body.String())
	}

	// List.
	rec = do(t, h, "GET", "/processors", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d", rec.Code)
	}
	var list struct {
		Data []json.RawMessage `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	decode(t, rec, &list)
	if list.Meta.Total != 1 || len(list.Data) != 1 {
		t.Errorf("list total = %d, len = %d", list.Meta.Total, len(list.Data))
	}

	// Update.
	rec = do(t, h, "PUT", "/processors/"+id+"", jsonapiDoc("processors", map[string]any{
		"name": "Renamed", "kind": "template",
		"config": map[string]string{"template": "X: {{ .Message.subject }}"},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", rec.Code, rec.Body.String())
	}

	// Delete.
	if rec := do(t, h, "DELETE", "/processors/"+id+"", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "GET", "/processors/"+id+"", nil); rec.Code != http.StatusNotFound {
		t.Errorf("get after delete = %d, want 404", rec.Code)
	}
}

func TestCreateProcessorBadInput(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTrigger(t, st, "s-in", "Inbox")

	// Malformed template → 400.
	rec := do(t, h, "POST", "/processors", jsonapiDoc("processors", map[string]any{
		"name": "Bad", "input_source": "trigger:s-in", "kind": "template",
		"config": map[string]string{"template": "{{ .Message.body "},
	}))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed template status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}

	// Unknown kind → 400.
	rec = do(t, h, "POST", "/processors", jsonapiDoc("processors", map[string]any{
		"name": "Bad2", "input_source": "trigger:s-in", "kind": "nope",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown kind status = %d, want 400", rec.Code)
	}
}

// TestUpdateProcessorCycleConflict pins the cycle guard on the path the
// chart actually drives: p1 reads a Trigger, p2 reads p1's branch, then
// re-pointing p1 at p2's branch would close the loop — 409, not a
// silently-accepted graph that would spin at runtime.
func TestUpdateProcessorCycleConflict(t *testing.T) {
	var n int
	deps, st, _ := newDepsClock(t,
		func() time.Time { return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC) },
		func() string { n++; return "id-" + string(rune('a'+n)) },
	)
	h := orgapi.Handler(deps)
	seedTrigger(t, st, "s-a", "A")

	type resource struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Outputs []struct {
					Source string `json:"source"`
				} `json:"outputs"`
			} `json:"attributes"`
		} `json:"data"`
	}
	create := func(name, input string) resource {
		t.Helper()
		attrs := map[string]any{
			"name": name, "kind": "template",
			"config": map[string]string{"template": "{{ .Message.body }}"},
		}
		if input != "" {
			attrs["input_source"] = input
		}
		rec := do(t, h, "POST", "/processors", jsonapiDoc("processors", attrs))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s = %d: %s", name, rec.Code, rec.Body.String())
		}
		var got resource
		decode(t, rec, &got)
		return got
	}

	p1 := create("P1", "trigger:s-a")
	p2 := create("P2", p1.Data.Attributes.Outputs[0].Source)

	// Re-point p1 at p2's branch: s-a → p1 → p2 → p1 is a cycle.
	rec := do(t, h, "PUT", "/processors/"+p1.Data.ID, jsonapiDoc("processors", map[string]any{
		"name": "P1", "kind": "template",
		"input_source": p2.Data.Attributes.Outputs[0].Source,
		"config":       map[string]string{"template": "{{ .Message.body }}"},
	}))
	if rec.Code != http.StatusConflict {
		t.Errorf("cycle status = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
	// Error body is a JSON:API error document: {"errors":[{status,detail}]}.
	var errDoc struct {
		Errors []struct {
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	decode(t, rec, &errDoc)
	if len(errDoc.Errors) != 1 || errDoc.Errors[0].Status != "409" || errDoc.Errors[0].Detail == "" {
		t.Errorf("expected one JSON:API error with status 409, got %+v", errDoc.Errors)
	}
}

func TestCreateProcessorDuplicateNameConflict(t *testing.T) {
	var n int
	deps, st, _ := newDepsClock(t,
		func() time.Time { return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC) },
		func() string { n++; return "id-" + string(rune('a'+n)) }, // unique per call
	)
	h := orgapi.Handler(deps)
	seedTrigger(t, st, "s-in", "Inbox")
	body := jsonapiDoc("processors", map[string]any{
		"name": "Dup", "input_source": "trigger:s-in", "kind": "template",
		"config": map[string]string{"template": "{{ .Message.body }}"},
	})
	if rec := do(t, h, "POST", "/processors", body); rec.Code != http.StatusCreated {
		t.Fatalf("first create = %d: %s", rec.Code, rec.Body.String())
	}
	rec := do(t, h, "POST", "/processors", body)
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate name status = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
	// Clean, non-raw error detail (no SQLSTATE / driver internals).
	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, "SQLSTATE") || strings.Contains(bodyStr, "duplicate key") {
		t.Errorf("conflict error leaked raw driver text: %s", bodyStr)
	}
}

func TestUpdateProcessorRewiresInputSource(t *testing.T) {
	var n int
	deps, st, _ := newDepsClock(t,
		func() time.Time { return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC) },
		func() string { n++; return "id-" + string(rune('a'+n)) },
	)
	h := orgapi.Handler(deps)
	seedTrigger(t, st, "s-in", "Inbox")
	seedTrigger(t, st, "s-in2", "Inbox 2")

	rec := do(t, h, "POST", "/processors", jsonapiDoc("processors", map[string]any{
		"name": "P", "input_source": "trigger:s-in", "kind": "template",
		"config": map[string]string{"template": "{{ .Message.body }}"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	decode(t, rec, &created)
	id := created.Data.ID

	// Re-point the input source (what the chart's drag-to-wire does).
	rec = do(t, h, "PUT", "/processors/"+id, jsonapiDoc("processors", map[string]any{
		"name": "P", "kind": "template", "input_source": "trigger:s-in2",
		"config": map[string]string{"template": "{{ .Message.body }}"},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Data struct {
			Attributes struct {
				InputSource string `json:"input_source"`
			} `json:"attributes"`
		} `json:"data"`
	}
	decode(t, rec, &doc)
	if doc.Data.Attributes.InputSource != "trigger:s-in2" {
		t.Errorf("input after rewire = %q, want trigger:s-in2", doc.Data.Attributes.InputSource)
	}

	// Omitting input_source on update leaves it unchanged.
	rec = do(t, h, "PUT", "/processors/"+id, jsonapiDoc("processors", map[string]any{
		"name": "P2", "kind": "template",
		"config": map[string]string{"template": "{{ .Message.body }}"},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("update2 = %d: %s", rec.Code, rec.Body.String())
	}
	decode(t, rec, &doc)
	if doc.Data.Attributes.InputSource != "trigger:s-in2" {
		t.Errorf("input after name-only update = %q, want trigger:s-in2 (unchanged)", doc.Data.Attributes.InputSource)
	}
}

func TestGetMissingProcessor404(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)
	if rec := do(t, h, "GET", "/processors/p-ghost", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing processor get = %d, want 404", rec.Code)
	}
}
