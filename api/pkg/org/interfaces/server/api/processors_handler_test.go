package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
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
}

func TestGetMissingProcessor404(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)
	if rec := do(t, h, "GET", "/processors/p-ghost", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing processor get = %d, want 404", rec.Code)
	}
}

func TestPreviewReturnsBeforeAfter(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/processors/preview", jsonapiDoc("processor-previews", map[string]any{
		"kind":   "template",
		"config": map[string]string{"template": "From {{ .Message.from }}: {{ .Message.body }}"},
		"samples": []map[string]any{
			{"from": "alice", "body": "hi"},
			{"from": "bob", "body": "yo"},
		},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d: %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Data []struct {
			Attributes struct {
				Before struct {
					Body string `json:"body"`
				} `json:"before"`
				After []struct {
					Body string `json:"body"`
				} `json:"after"`
			} `json:"attributes"`
		} `json:"data"`
	}
	decode(t, rec, &doc)
	if len(doc.Data) != 2 {
		t.Fatalf("want 2 preview pairs, got %d", len(doc.Data))
	}
	if doc.Data[0].Attributes.After[0].Body != "From alice: hi" {
		t.Errorf("pair0 after = %q", doc.Data[0].Attributes.After[0].Body)
	}
	if doc.Data[1].Attributes.After[0].Body != "From bob: yo" {
		t.Errorf("pair1 after = %q", doc.Data[1].Attributes.After[0].Body)
	}
}
