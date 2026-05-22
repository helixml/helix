package chat

import (
	"context"
	"errors"
	"testing"

	runtimehelix "github.com/helixml/helix/api/pkg/org/runtime/helix"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeChatBridgeClient is an in-test satisfier of ChatBridgeClient
// used to assert the port's shape compiles and SendToSession's
// error-surface semantics still hold after the H1-chat extraction.
type fakeChatBridgeClient struct {
	// startReturn drives StartChatWithStatus's three return values.
	startSession types.Session
	startHadErr  bool
	startErr     error
}

func (f *fakeChatBridgeClient) StartChatWithStatus(_ context.Context, _ runtimehelix.StartChatRequest) (types.Session, bool, error) {
	return f.startSession, f.startHadErr, f.startErr
}

func (f *fakeChatBridgeClient) ServerStatus(_ context.Context) (runtimehelix.ServerStatus, error) {
	return runtimehelix.ServerStatus{}, nil
}

func (f *fakeChatBridgeClient) GetOutput(_ context.Context, _ string) (types.SessionOutputResponse, error) {
	return types.SessionOutputResponse{}, nil
}

func (f *fakeChatBridgeClient) StopExternalAgent(_ context.Context, _ string) error { return nil }

func (f *fakeChatBridgeClient) GetSession(_ context.Context, _ string) (types.Session, error) {
	return types.Session{}, nil
}

func (f *fakeChatBridgeClient) GetProject(_ context.Context, id string) (types.Project, error) {
	return types.Project{ID: id, OrganizationID: "org-test"}, nil
}

func (f *fakeChatBridgeClient) SubscribeUpdates(ctx context.Context, _ string) (<-chan types.WebsocketEvent, error) {
	ch := make(chan types.WebsocketEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

// Compile-time assertion: the in-test fake satisfies the port. If a
// new method is added to ChatBridgeClient and not stubbed on the
// fake, this fails the build — surfacing the drift immediately
// instead of letting tests pass against a stale fake.
var _ ChatBridgeClient = (*fakeChatBridgeClient)(nil)

// TestSendToSession_StreamErrorIsSurfaced pins the SendToSession
// stream-error contract: when StartChatWithStatus
// returns (session, true, nil) — i.e. the wire call succeeded but
// the SSE stream emitted an error chunk after the session ID came
// through — SendToSession surfaces a "session no longer running on
// the server" error so the caller knows to treat the persisted
// SessionID as stale and open a fresh one. Without this contract
// every restart would wedge against orphaned in-memory sessions on
// the server.
func TestSendToSession_StreamErrorIsSurfaced(t *testing.T) {
	t.Parallel()
	fc := &fakeChatBridgeClient{
		startSession: types.Session{ID: "ses_old"},
		startHadErr:  true,
		startErr:     nil,
	}
	req := runtimehelix.StartChatRequest{
		SessionID: "ses_old",
		Messages:  []runtimehelix.SessionChatMessage{runtimehelix.NewTextMessage("user", "hi")},
	}
	_, err := SendToSession(context.Background(), fc, req)
	if err == nil {
		t.Fatalf("SendToSession: want error when streamHadErr=true, got nil")
	}
	if err.Error() != "session no longer running on the server" {
		t.Errorf("SendToSession: want stale-session error, got %q", err.Error())
	}
}

// TestSendToSession_HappyPath verifies the no-error case echoes the
// session through unchanged.
func TestSendToSession_HappyPath(t *testing.T) {
	t.Parallel()
	fc := &fakeChatBridgeClient{
		startSession: types.Session{ID: "ses_ok"},
		startHadErr:  false,
		startErr:     nil,
	}
	req := runtimehelix.StartChatRequest{
		SessionID: "ses_ok",
		Messages:  []runtimehelix.SessionChatMessage{runtimehelix.NewTextMessage("user", "hi")},
	}
	got, err := SendToSession(context.Background(), fc, req)
	if err != nil {
		t.Fatalf("SendToSession: unexpected error: %v", err)
	}
	if got.ID != "ses_ok" {
		t.Errorf("SendToSession: got session %q, want ses_ok", got.ID)
	}
}

// TestSendToSession_EmptySessionIDRejected pins the contract that
// SendToSession is a *continuation* helper — it requires a known
// SessionID. Without it the request would silently open a fresh
// session (because the StartChat endpoint accepts an empty
// SessionID), defeating the resume semantics.
func TestSendToSession_EmptySessionIDRejected(t *testing.T) {
	t.Parallel()
	fc := &fakeChatBridgeClient{}
	_, err := SendToSession(context.Background(), fc, runtimehelix.StartChatRequest{})
	if err == nil {
		t.Fatalf("SendToSession: want error for empty SessionID, got nil")
	}
	if !errorsContains(err, "SessionID required") {
		t.Errorf("SendToSession: want \"SessionID required\" error, got %q", err.Error())
	}
}

// errorsContains is a small helper because errors.Is doesn't match
// substrings on plain errors.New — we want to be permissive about
// the surrounding error wording while pinning the key phrase.
func errorsContains(err error, sub string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errors.New(sub)) {
		return true
	}
	return contains(err.Error(), sub)
}

func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
