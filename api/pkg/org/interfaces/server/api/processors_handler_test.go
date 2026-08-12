package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// jsonapiDoc wraps attributes in a JSON:API request document.
func jsonapiDoc(typ string, attrs map[string]any) map[string]any {
	return map[string]any{"data": map[string]any{"type": typ, "attributes": attrs}}
}

func seedTopic(t *testing.T, st *store.Store, id, name string) {
	t.Helper()
	top, err := streaming.NewTopic(streaming.TopicID(id), name, "", "", time.Now().UTC(), transport.LocalTransport(), "org-test")
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(context.Background(), top); err != nil {
		t.Fatalf("create topic %s: %v", id, err)
	}
}

func TestCreateProcessorReturnsResourceAndOutputTopic(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTopic(t, st, "s-in", "Inbox")

	rec := do(t, h, "POST", "/processors", jsonapiDoc("processors", map[string]any{
		"name":           "Formatter",
		"input_topic_id": "s-in",
		"kind":           "template",
		"config":         map[string]string{"template": "From {{ .Message.from }}: {{ .Message.body }}"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Data struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				InputTopicID string `json:"input_topic_id"`
				Kind         string `json:"kind"`
				Outputs      []struct {
					TopicID string `json:"topic_id"`
					Owned   bool   `json:"owned"`
				} `json:"outputs"`
			} `json:"attributes"`
		} `json:"data"`
	}
	decode(t, rec, &doc)
	if doc.Data.Type != "processors" || doc.Data.ID == "" {
		t.Fatalf("bad resource: %+v", doc.Data)
	}
	if len(doc.Data.Attributes.Outputs) != 1 || doc.Data.Attributes.Outputs[0].TopicID == "" || !doc.Data.Attributes.Outputs[0].Owned {
		t.Fatalf("expected one owned auto-provisioned output, got %+v", doc.Data.Attributes.Outputs)
	}
	// The auto-created output topic exists.
	if _, err := st.Topics.Get(context.Background(), "org-test", streaming.TopicID(doc.Data.Attributes.Outputs[0].TopicID)); err != nil {
		t.Errorf("output topic not created: %v", err)
	}
}

func TestProcessorCRUDLifecycle(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTopic(t, st, "s-in", "Inbox")

	// Create (server mints the id; capture it).
	rec := do(t, h, "POST", "/processors", jsonapiDoc("processors", map[string]any{
		"name": "Life", "input_topic_id": "s-in", "kind": "template",
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
	seedTopic(t, st, "s-in", "Inbox")

	// Malformed template → 400.
	rec := do(t, h, "POST", "/processors", jsonapiDoc("processors", map[string]any{
		"name": "Bad", "input_topic_id": "s-in", "kind": "template",
		"config": map[string]string{"template": "{{ .Message.body "},
	}))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed template status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}

	// Unknown kind → 400.
	rec = do(t, h, "POST", "/processors", jsonapiDoc("processors", map[string]any{
		"name": "Bad2", "input_topic_id": "s-in", "kind": "nope",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown kind status = %d, want 400", rec.Code)
	}
}

func TestCreateProcessorCycleConflict(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTopic(t, st, "s-a", "A")

	// Explicit output == input → cycle → 409.
	rec := do(t, h, "POST", "/processors", jsonapiDoc("processors", map[string]any{
		"name": "Self", "input_topic_id": "s-a", "kind": "template",
		"config":  map[string]string{"template": "{{ .Message.body }}"},
		"outputs": []map[string]any{{"topic_id": "s-a"}},
	}))
	if rec.Code != http.StatusConflict {
		t.Errorf("self-cycle status = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
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
	seedTopic(t, st, "s-in", "Inbox")
	body := jsonapiDoc("processors", map[string]any{
		"name": "Dup", "input_topic_id": "s-in", "kind": "template",
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

func TestDeleteProcessorOutputTopicBlocked(t *testing.T) {
	var n int
	deps, st, _ := newDepsClock(t,
		func() time.Time { return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC) },
		func() string { n++; return "id-" + string(rune('a'+n)) },
	)
	h := orgapi.Handler(deps)
	seedTopic(t, st, "s-in", "Inbox")

	rec := do(t, h, "POST", "/processors", jsonapiDoc("processors", map[string]any{
		"name": "Fmt", "input_topic_id": "s-in", "kind": "template",
		"config": map[string]string{"template": "{{ .Message.body }}"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Data struct {
			Attributes struct {
				Outputs []struct {
					TopicID string `json:"topic_id"`
				} `json:"outputs"`
			} `json:"attributes"`
		} `json:"data"`
	}
	decode(t, rec, &doc)
	outID := doc.Data.Attributes.Outputs[0].TopicID

	// Deleting the processor-owned output topic directly must be blocked.
	del := do(t, h, "DELETE", "/topics/"+outID, nil)
	if del.Code != http.StatusConflict {
		t.Errorf("delete owned output topic = %d, want 409 (body=%s)", del.Code, del.Body.String())
	}
	// The topic must still exist.
	if _, err := st.Topics.Get(context.Background(), "org-test", streaming.TopicID(outID)); err != nil {
		t.Errorf("owned output topic was deleted despite the guard: %v", err)
	}
}

func TestUpdateProcessorRewiresInputTopic(t *testing.T) {
	var n int
	deps, st, _ := newDepsClock(t,
		func() time.Time { return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC) },
		func() string { n++; return "id-" + string(rune('a'+n)) },
	)
	h := orgapi.Handler(deps)
	seedTopic(t, st, "s-in", "Inbox")
	seedTopic(t, st, "s-in2", "Inbox 2")

	rec := do(t, h, "POST", "/processors", jsonapiDoc("processors", map[string]any{
		"name": "P", "input_topic_id": "s-in", "kind": "template",
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

	// Re-point the input topic (what the chart's drag-to-wire does).
	rec = do(t, h, "PUT", "/processors/"+id, jsonapiDoc("processors", map[string]any{
		"name": "P", "kind": "template", "input_topic_id": "s-in2",
		"config": map[string]string{"template": "{{ .Message.body }}"},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Data struct {
			Attributes struct {
				InputTopicID string `json:"input_topic_id"`
			} `json:"attributes"`
		} `json:"data"`
	}
	decode(t, rec, &doc)
	if doc.Data.Attributes.InputTopicID != "s-in2" {
		t.Errorf("input after rewire = %q, want s-in2", doc.Data.Attributes.InputTopicID)
	}

	// Omitting input_topic_id on update leaves it unchanged.
	rec = do(t, h, "PUT", "/processors/"+id, jsonapiDoc("processors", map[string]any{
		"name": "P2", "kind": "template",
		"config": map[string]string{"template": "{{ .Message.body }}"},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("update2 = %d: %s", rec.Code, rec.Body.String())
	}
	decode(t, rec, &doc)
	if doc.Data.Attributes.InputTopicID != "s-in2" {
		t.Errorf("input after name-only update = %q, want s-in2 (unchanged)", doc.Data.Attributes.InputTopicID)
	}
}

func TestGetMissingProcessor404(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)
	if rec := do(t, h, "GET", "/processors/p-ghost", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing processor get = %d, want 404", rec.Code)
	}
}
