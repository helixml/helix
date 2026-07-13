package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/interfaces/jsonapi"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// seedTopicWithEvents creates a local-transport topic and appends n
// message events (e-0 oldest … e-(n-1) newest), one minute apart.
func seedTopicWithEvents(t *testing.T, st *store.Store, id string, n int) {
	t.Helper()
	ctx := context.Background()
	topic, err := streaming.NewTopic(
		streaming.TopicID(id), id, "",
		"w-owner", time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
		transport.Transport{Kind: transport.KindLocal}, "org-test",
	)
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	for i := 0; i < n; i++ {
		body := fmt.Sprintf(`{"from":"w-owner","subject":"s%d","body":"body %d"}`, i, i)
		ev, err := streaming.NewEvent(
			streaming.EventID(fmt.Sprintf("e-%d", i)), streaming.TopicID(id),
			"w-owner", body, time.Date(2026, 5, 22, 12, i, 0, 0, time.UTC), "org-test",
		)
		if err != nil {
			t.Fatalf("new event %d: %v", i, err)
		}
		if err := st.Events.Append(ctx, ev); err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}
}

// TestListTopicMessages_FirstPage pins the JSON:API shape: data is a
// `messages` array newest-first, meta carries the full total + page
// state, and the links object has self/first/next/last (no prev on
// page 1).
func TestListTopicMessages_FirstPage(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTopicWithEvents(t, st, "s-msgs", 5)

	rec := do(t, h, "GET", "/topics/s-msgs/messages?page[size]=2&page[number]=1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != jsonapi.ContentType {
		t.Errorf("Content-Type = %q, want %q", ct, jsonapi.ContentType)
	}
	var doc orgapi.MessagesDocument
	decode(t, rec, &doc)
	if len(doc.Data) != 2 {
		t.Fatalf("data len = %d, want 2: %+v", len(doc.Data), doc.Data)
	}
	if doc.Data[0].Type != "messages" {
		t.Errorf("type = %q, want messages", doc.Data[0].Type)
	}
	// Newest first: e-4 then e-3.
	if doc.Data[0].ID != "e-4" || doc.Data[1].ID != "e-3" {
		t.Errorf("ids = [%s %s], want [e-4 e-3]", doc.Data[0].ID, doc.Data[1].ID)
	}
	if doc.Data[0].Attributes.Body != "body 4" || doc.Data[0].Attributes.Subject != "s4" {
		t.Errorf("attrs = %+v, want decoded message", doc.Data[0].Attributes)
	}
	if !doc.Data[0].Attributes.HasMessage {
		t.Error("has_message = false, want true")
	}
	if doc.Meta.Total != 5 {
		t.Errorf("meta.total = %d, want 5", doc.Meta.Total)
	}
	if doc.Meta.TotalPages != 3 {
		t.Errorf("meta.total_pages = %d, want 3", doc.Meta.TotalPages)
	}
	if doc.Links["next"] == "" || doc.Links["self"] == "" || doc.Links["last"] == "" {
		t.Errorf("missing links: %v", doc.Links)
	}
	if _, ok := doc.Links["prev"]; ok {
		t.Errorf("page 1 should have no prev: %v", doc.Links)
	}
}

func TestClearTopicMessages(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTopicWithEvents(t, st, "s-clear", 3)
	keep, _ := streaming.NewTopic("s-keep", "keep", "", "w-owner", time.Now(), transport.Transport{Kind: transport.KindLocal}, "org-test")
	if err := st.Topics.Create(context.Background(), keep); err != nil {
		t.Fatalf("create other topic: %v", err)
	}
	keepEvent, _ := streaming.NewEvent("e-keep", "s-keep", "w-owner", "keep", time.Now(), "org-test")
	if err := st.Events.Append(context.Background(), keepEvent); err != nil {
		t.Fatalf("append other topic event: %v", err)
	}

	rec := do(t, h, "DELETE", "/topics/s-clear/messages", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want 204; body=%s", rec.Code, rec.Body)
	}
	if count, err := st.Events.CountForTopic(context.Background(), "org-test", "s-clear"); err != nil || count != 0 {
		t.Fatalf("cleared count = %d, %v; want 0, nil", count, err)
	}
	if count, _ := st.Events.CountForTopic(context.Background(), "org-test", "s-keep"); count != 1 {
		t.Fatalf("other topic count = %d, want 1", count)
	}

	// The immediately following normal operation must still work.
	next, _ := streaming.NewEvent("e-next", "s-clear", "w-owner", `{"from":"w-owner","body":"next"}`, time.Now(), "org-test")
	if err := st.Events.Append(context.Background(), next); err != nil {
		t.Fatalf("append after clear: %v", err)
	}
	page := do(t, h, "GET", "/topics/s-clear/messages", nil)
	if page.Code != http.StatusOK {
		t.Fatalf("list after clear + append: status=%d body=%s", page.Code, page.Body)
	}
	var doc orgapi.MessagesDocument
	decode(t, page, &doc)
	if len(doc.Data) != 1 || doc.Data[0].ID != "e-next" {
		t.Fatalf("messages after clear + append = %+v, want e-next", doc.Data)
	}
}

func TestClearTopicMessages_NotFound(t *testing.T) {
	deps, _, _ := newDeps(t)
	rec := do(t, orgapi.Handler(deps), "DELETE", "/topics/missing/messages", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404; body=%s", rec.Code, rec.Body)
	}
}

// TestListTopicMessages_LastPartialPage pins the tail: the final page
// holds the remainder, has prev, and no next. meta.total is unchanged.
func TestListTopicMessages_LastPartialPage(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTopicWithEvents(t, st, "s-msgs", 5)

	rec := do(t, h, "GET", "/topics/s-msgs/messages?page[size]=2&page[number]=3", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var doc orgapi.MessagesDocument
	decode(t, rec, &doc)
	if len(doc.Data) != 1 || doc.Data[0].ID != "e-0" {
		t.Fatalf("data = %+v, want single e-0", doc.Data)
	}
	if doc.Meta.Total != 5 {
		t.Errorf("meta.total = %d, want 5", doc.Meta.Total)
	}
	if doc.Links["prev"] == "" {
		t.Errorf("last page should have prev: %v", doc.Links)
	}
	if _, ok := doc.Links["next"]; ok {
		t.Errorf("last page should have no next: %v", doc.Links)
	}
}

// TestListTopicMessages_BeyondLastPage pins that requesting a page past
// the end returns an empty data array (not an error), with total intact.
func TestListTopicMessages_BeyondLastPage(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTopicWithEvents(t, st, "s-msgs", 3)

	rec := do(t, h, "GET", "/topics/s-msgs/messages?page[size]=2&page[number]=9", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var doc orgapi.MessagesDocument
	decode(t, rec, &doc)
	if len(doc.Data) != 0 {
		t.Fatalf("data = %+v, want empty", doc.Data)
	}
	if doc.Meta.Total != 3 {
		t.Errorf("meta.total = %d, want 3", doc.Meta.Total)
	}
}

// TestListTopicMessages_EmptyTopic pins total:0 / data:[] for a topic
// with no messages.
func TestListTopicMessages_EmptyTopic(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTopicWithEvents(t, st, "s-empty", 0)

	rec := do(t, h, "GET", "/topics/s-empty/messages", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var doc orgapi.MessagesDocument
	decode(t, rec, &doc)
	if len(doc.Data) != 0 {
		t.Errorf("data = %+v, want empty", doc.Data)
	}
	if doc.Meta.Total != 0 {
		t.Errorf("meta.total = %d, want 0", doc.Meta.Total)
	}
}

// TestListTopicMessages_TotalConsistentAcrossPages pins that meta.total
// is the full count regardless of which page is fetched.
func TestListTopicMessages_TotalConsistentAcrossPages(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTopicWithEvents(t, st, "s-msgs", 5)

	for _, page := range []string{"1", "2", "3"} {
		rec := do(t, h, "GET", "/topics/s-msgs/messages?page[size]=2&page[number]="+page, nil)
		var doc orgapi.MessagesDocument
		decode(t, rec, &doc)
		if doc.Meta.Total != 5 {
			t.Errorf("page %s: meta.total = %d, want 5", page, doc.Meta.Total)
		}
	}
}

// TestListTopicMessages_UnknownTopic pins 404 for a topic that
// doesn't exist (rather than an empty 200).
func TestListTopicMessages_UnknownTopic(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, "GET", "/topics/s-ghost/messages", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404; body=%s", rec.Code, rec.Body)
	}
}

// TestListTopicMessages_BadPagingParams pins 400 for non-numeric /
// out-of-range paging params.
func TestListTopicMessages_BadPagingParams(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedTopicWithEvents(t, st, "s-msgs", 2)

	for _, q := range []string{"page[number]=0", "page[number]=abc", "page[size]=-1"} {
		rec := do(t, h, "GET", "/topics/s-msgs/messages?"+q, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status got %d, want 400; body=%s", q, rec.Code, rec.Body)
		}
	}
}
