package notion

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeNotionAPI captures calls and returns canned responses.
type fakeNotionAPI struct {
	mu                sync.Mutex
	appendedURLs      []string
	appendedPageIDs   []string
	deletedBlocks     []string
	patchedProps      []patchCall
	appendBlockID     string
	appendErr         error
	deleteErr         error
	patchErr          error
}

type patchCall struct{ pageID, name, text string }

func (f *fakeNotionAPI) GetPage(_ context.Context, pageID string) (*Page, error) {
	return &Page{ID: pageID}, nil
}
func (f *fakeNotionAPI) PatchRichTextProperty(_ context.Context, pageID, name, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.patchedProps = append(f.patchedProps, patchCall{pageID, name, text})
	return f.patchErr
}
func (f *fakeNotionAPI) AppendEmbedBlock(_ context.Context, pageID, embedURL string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.appendedURLs = append(f.appendedURLs, embedURL)
	f.appendedPageIDs = append(f.appendedPageIDs, pageID)
	if f.appendErr != nil {
		return "", f.appendErr
	}
	id := f.appendBlockID
	if id == "" {
		id = "blk-" + pageID
	}
	return id, nil
}
func (f *fakeNotionAPI) DeleteBlock(_ context.Context, blockID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletedBlocks = append(f.deletedBlocks, blockID)
	return f.deleteErr
}

// fakeCreator records creation calls and returns a canned spectask.
type fakeCreator struct {
	calls    []*types.CreateTaskRequest
	returned *types.SpecTask
	err      error
}

func (f *fakeCreator) CreateTaskFromPrompt(_ context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return nil, f.err
	}
	if f.returned != nil {
		return f.returned, nil
	}
	return &types.SpecTask{ID: "spt-test", ProjectID: req.ProjectID, UserID: req.UserID}, nil
}

// fakeLookup returns a configured spectask for any ref (or nil if disabled).
type fakeLookup struct {
	task *types.SpecTask
	err  error
}

func (f *fakeLookup) GetSpecTaskByExternalRef(_ context.Context, _ *types.ExternalTriggerRef) (*types.SpecTask, error) {
	return f.task, f.err
}

// fakeCanceller records cancel calls.
type fakeCanceller struct {
	calls []*types.ExternalTriggerRef
	err   error
}

func (f *fakeCanceller) CancelTaskByExternalRef(_ context.Context, ref *types.ExternalTriggerRef) (*types.SpecTask, error) {
	f.calls = append(f.calls, ref)
	if f.err != nil {
		return nil, f.err
	}
	return &types.SpecTask{ID: "spt-cancelled"}, nil
}

type fakeEmbedURLs struct{}

func (fakeEmbedURLs) BuildEmbedURL(spectask *types.SpecTask, accessToken string) string {
	return "https://test.helix.example/embed/task/" + spectask.ID + "?access_token=" + accessToken
}

func newTestNotion(t *testing.T, opts ...func(*Notion)) (*Notion, *fakeNotionAPI, *store.MockStore, func()) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	api := &fakeNotionAPI{}

	cfg := &config.ServerConfig{}
	n := New(cfg, mockStore, &fakeCreator{}, nil, nil, fakeEmbedURLs{})
	n.newClient = func(string) NotionAPI { return api }
	for _, o := range opts {
		o(n)
	}
	return n, api, mockStore, ctrl.Finish
}

func makeAutomationBody(t *testing.T, pageID, prompt, dbID string) []byte {
	t.Helper()
	body := map[string]any{
		"source": "automation",
		"data": map[string]any{
			"id":     pageID,
			"object": "page",
			"parent": map[string]any{"database_id": dbID},
			"properties": map[string]any{
				"Name": map[string]any{
					"type":  "title",
					"title": []map[string]any{{"plain_text": "Notion test row"}},
				},
				"Prompt": map[string]any{
					"type":      "rich_text",
					"rich_text": []map[string]any{{"plain_text": prompt}},
				},
			},
		},
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return b
}

func makeTriggerConfig(secret string) *types.TriggerConfiguration {
	return &types.TriggerConfiguration{
		ID:    "trig-1",
		AppID: "app-1",
		Owner: "user-1",
		Trigger: types.Trigger{
			Notion: &types.NotionTrigger{
				Enabled:           true,
				SharedSecret:      secret,
				OAuthConnectionID: "oauth-conn-1",
				TargetProjectID:   "prj-1",
				EmbedAccessToken:  "embed-tok-1",
				ColumnMapping: types.NotionColumnMap{
					ActionColumn:       "Go/NoGo",
					ActionColumnType:   "select",
					ActionOptionCreate: "Go",
					ActionOptionCancel: "NoGo",
					PromptColumn:       "Prompt",
					ResultColumn:       "Result",
				},
			},
		},
	}
}

func TestProcessWebhook_RejectsBadSecret(t *testing.T) {
	n, _, _, done := newTestNotion(t)
	defer done()

	headers := http.Header{}
	headers.Set(HeaderSharedSecret, "wrong")
	headers.Set(HeaderSource, SourceAutomation)
	headers.Set(HeaderAction, ActionCreate)

	err := n.ProcessWebhook(context.Background(), makeTriggerConfig("right"), headers, makeAutomationBody(t, "p1", "do x", "db1"))
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestProcessWebhook_RejectsUnknownSource(t *testing.T) {
	n, _, _, done := newTestNotion(t)
	defer done()

	headers := http.Header{}
	headers.Set(HeaderSharedSecret, "right")
	// Missing X-Helix-Source and no Notion signature header.

	err := n.ProcessWebhook(context.Background(), makeTriggerConfig("right"), headers, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for unrecognised webhook")
	}
}

func TestOnExternalCreate_CreatesSpecTaskAndAppendsEmbed(t *testing.T) {
	creator := &fakeCreator{returned: &types.SpecTask{ID: "spt-100", ProjectID: "prj-1"}}
	lookup := &fakeLookup{task: nil} // no existing
	n, api, mockStore, done := newTestNotion(t, func(n *Notion) {
		n.specTaskCreator = creator
		n.lookup = lookup
	})
	defer done()

	// We expect two UpdateSpecTask calls: one to persist the ref, one to
	// persist the embed block id after appending.
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).Times(2).Return(nil)
	mockStore.EXPECT().GetOAuthConnection(gomock.Any(), "oauth-conn-1").Return(&types.OAuthConnection{AccessToken: "tok-xyz"}, nil)

	headers := http.Header{}
	headers.Set(HeaderSharedSecret, "secret")
	headers.Set(HeaderSource, SourceAutomation)
	headers.Set(HeaderAction, ActionCreate)

	body := makeAutomationBody(t, "page-100", "investigate notion", "db-1")
	if err := n.ProcessWebhook(context.Background(), makeTriggerConfig("secret"), headers, body); err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}

	if got := len(creator.calls); got != 1 {
		t.Fatalf("creator called %d times, want 1", got)
	}
	if creator.calls[0].Prompt != "investigate notion" {
		t.Errorf("creator prompt = %q", creator.calls[0].Prompt)
	}
	if creator.calls[0].ProjectID != "prj-1" {
		t.Errorf("creator project = %q", creator.calls[0].ProjectID)
	}

	if len(api.appendedURLs) != 1 {
		t.Fatalf("expected 1 append-embed call, got %d", len(api.appendedURLs))
	}
	wantURL := "https://test.helix.example/embed/task/spt-100?access_token=embed-tok-1"
	if api.appendedURLs[0] != wantURL {
		t.Errorf("embed URL = %q, want %q", api.appendedURLs[0], wantURL)
	}
}

func TestOnExternalCreate_IdempotentOnReplay(t *testing.T) {
	existing := &types.SpecTask{
		ID: "spt-existing",
		ExternalTriggerRef: &types.ExternalTriggerRef{
			Type:            types.ExternalTriggerSourceNotion,
			TriggerConfigID: "trig-1",
			Payload:         marshalNotionPayload(&types.NotionTriggerPayload{PageID: "page-100"}),
		},
	}
	creator := &fakeCreator{}
	lookup := &fakeLookup{task: existing}
	n, api, _, done := newTestNotion(t, func(n *Notion) {
		n.specTaskCreator = creator
		n.lookup = lookup
	})
	defer done()

	headers := http.Header{}
	headers.Set(HeaderSharedSecret, "secret")
	headers.Set(HeaderSource, SourceAutomation)
	headers.Set(HeaderAction, ActionCreate)

	body := makeAutomationBody(t, "page-100", "p", "db-1")
	if err := n.ProcessWebhook(context.Background(), makeTriggerConfig("secret"), headers, body); err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}

	if len(creator.calls) != 0 {
		t.Errorf("creator should not be called on replay, got %d calls", len(creator.calls))
	}
	if len(api.appendedURLs) != 0 {
		t.Errorf("embed should not be appended on replay")
	}
}

func TestOnExternalCancel_RemovesEmbedAndCancels(t *testing.T) {
	existing := &types.SpecTask{
		ID: "spt-cancel-me",
		ExternalTriggerRef: &types.ExternalTriggerRef{
			Type:            types.ExternalTriggerSourceNotion,
			TriggerConfigID: "trig-1",
			Payload: marshalNotionPayload(&types.NotionTriggerPayload{
				PageID:       "page-200",
				EmbedBlockID: "blk-200",
			}),
		},
	}
	canceller := &fakeCanceller{}
	lookup := &fakeLookup{task: existing}
	n, api, mockStore, done := newTestNotion(t, func(n *Notion) {
		n.lookup = lookup
		n.canceller = canceller
	})
	defer done()

	mockStore.EXPECT().GetOAuthConnection(gomock.Any(), "oauth-conn-1").Return(&types.OAuthConnection{AccessToken: "tok"}, nil)

	headers := http.Header{}
	headers.Set(HeaderSharedSecret, "secret")
	headers.Set(HeaderSource, SourceAutomation)
	headers.Set(HeaderAction, ActionCancel)

	body := makeAutomationBody(t, "page-200", "", "db-1")
	if err := n.ProcessWebhook(context.Background(), makeTriggerConfig("secret"), headers, body); err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}

	if len(canceller.calls) != 1 {
		t.Fatalf("canceller calls = %d, want 1", len(canceller.calls))
	}
	if len(api.deletedBlocks) != 1 || api.deletedBlocks[0] != "blk-200" {
		t.Errorf("expected delete of blk-200, got %v", api.deletedBlocks)
	}
}

func TestOnExternalCancel_NoLiveSpecTaskIsNoOp(t *testing.T) {
	canceller := &fakeCanceller{}
	lookup := &fakeLookup{task: nil}
	n, api, _, done := newTestNotion(t, func(n *Notion) {
		n.lookup = lookup
		n.canceller = canceller
	})
	defer done()

	headers := http.Header{}
	headers.Set(HeaderSharedSecret, "secret")
	headers.Set(HeaderSource, SourceAutomation)
	headers.Set(HeaderAction, ActionCancel)

	body := makeAutomationBody(t, "page-orphan", "", "db-1")
	if err := n.ProcessWebhook(context.Background(), makeTriggerConfig("secret"), headers, body); err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}
	if len(canceller.calls) != 0 {
		t.Errorf("expected no cancel call, got %d", len(canceller.calls))
	}
	if len(api.deletedBlocks) != 0 {
		t.Errorf("expected no embed delete, got %v", api.deletedBlocks)
	}
}

func TestOnSpecTaskCompleted_PatchesResultColumnOnly(t *testing.T) {
	n, api, mockStore, done := newTestNotion(t)
	defer done()

	mockStore.EXPECT().GetOAuthConnection(gomock.Any(), "oauth-conn-1").Return(&types.OAuthConnection{AccessToken: "tok"}, nil)

	spectask := &types.SpecTask{
		ID: "spt-done",
		ExternalTriggerRef: &types.ExternalTriggerRef{
			Type:            types.ExternalTriggerSourceNotion,
			TriggerConfigID: "trig-1",
			Payload:         marshalNotionPayload(&types.NotionTriggerPayload{PageID: "page-300"}),
		},
	}
	if err := n.OnSpecTaskCompleted(context.Background(), makeTriggerConfig("s"), spectask, "all done"); err != nil {
		t.Fatalf("OnSpecTaskCompleted: %v", err)
	}
	if len(api.patchedProps) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(api.patchedProps))
	}
	got := api.patchedProps[0]
	if got.pageID != "page-300" || got.name != "Result" || got.text != "all done" {
		t.Errorf("patch = %+v, want pageID=page-300 name=Result text='all done'", got)
	}
}

func TestOnSpecTaskCompleted_NoOpWhenNoResultColumn(t *testing.T) {
	n, api, _, done := newTestNotion(t)
	defer done()

	cfg := makeTriggerConfig("s")
	cfg.Trigger.Notion.ColumnMapping.ResultColumn = ""

	spectask := &types.SpecTask{
		ID: "spt-x",
		ExternalTriggerRef: &types.ExternalTriggerRef{
			Type:            types.ExternalTriggerSourceNotion,
			TriggerConfigID: "trig-1",
			Payload:         marshalNotionPayload(&types.NotionTriggerPayload{PageID: "page-x"}),
		},
	}
	if err := n.OnSpecTaskCompleted(context.Background(), cfg, spectask, "any"); err != nil {
		t.Fatalf("OnSpecTaskCompleted: %v", err)
	}
	if len(api.patchedProps) != 0 {
		t.Errorf("expected no patch when ResultColumn empty, got %v", api.patchedProps)
	}
}

func TestOnExternalCreate_AppendEmbedFailureDoesNotBlockSpecTask(t *testing.T) {
	creator := &fakeCreator{returned: &types.SpecTask{ID: "spt-200", ProjectID: "prj-1"}}
	lookup := &fakeLookup{task: nil}
	n, api, mockStore, done := newTestNotion(t, func(n *Notion) {
		n.specTaskCreator = creator
		n.lookup = lookup
	})
	defer done()
	api.appendErr = errors.New("notion 503")

	// First UpdateSpecTask (persist ref) succeeds; the second-phase update
	// for the embed-block id never happens because append failed.
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).Return(nil)
	mockStore.EXPECT().GetOAuthConnection(gomock.Any(), "oauth-conn-1").Return(&types.OAuthConnection{AccessToken: "tok"}, nil)

	headers := http.Header{}
	headers.Set(HeaderSharedSecret, "secret")
	headers.Set(HeaderSource, SourceAutomation)
	headers.Set(HeaderAction, ActionCreate)

	body := makeAutomationBody(t, "page-200", "investigate", "db-1")
	if err := n.ProcessWebhook(context.Background(), makeTriggerConfig("secret"), headers, body); err != nil {
		t.Fatalf("expected spectask creation to succeed despite embed failure, got %v", err)
	}
	if len(creator.calls) != 1 {
		t.Errorf("expected 1 spectask, got %d", len(creator.calls))
	}
}
