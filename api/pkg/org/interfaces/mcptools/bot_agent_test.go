package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

type lifecycleCaller struct{}

func (lifecycleCaller) ID() string             { return "b-owner" }
func (lifecycleCaller) OrganizationID() string { return "org-1" }

type botLifecycleFakes struct {
	restarts, cancels, resets, retires int
	audits                             []audit.Entry
}

func (f *botLifecycleFakes) RestartRuntime(context.Context, string, orgchart.NodeID) error {
	f.restarts++
	return nil
}
func (f *botLifecycleFakes) CancelOutstanding(context.Context, string, orgchart.NodeID) error {
	f.cancels++
	return nil
}
func (f *botLifecycleFakes) ResetSession(context.Context, string, orgchart.NodeID, string) error {
	f.resets++
	return nil
}
func (f *botLifecycleFakes) HomeEstablished(context.Context, string, orgchart.NodeID) (bool, error) {
	return true, nil
}
func (f *botLifecycleFakes) RetireHome(context.Context, string, orgchart.NodeID) (activations.HomeErasure, error) {
	f.retires++
	return activations.HomeErasure{Existed: true, Generation: "bh_1", RetiredPath: "/retired"}, nil
}
func (f *botLifecycleFakes) Record(_ context.Context, e audit.Entry) error {
	f.audits = append(f.audits, e)
	return nil
}
func (f *botLifecycleFakes) SessionID(context.Context, string, orgchart.NodeID) (string, error) {
	return "ses_1", nil
}
func (f *botLifecycleFakes) Ensure(context.Context, string, orgchart.NodeID) (string, string, string, error) {
	return "prj", "app", "repo", nil
}
func (f *botLifecycleFakes) DispatchHire(context.Context, string, orgchart.NodeID, activation.ID)   {}
func (f *botLifecycleFakes) DispatchManual(context.Context, string, orgchart.NodeID, activation.ID) {}

func newLifecycleDeps(f *botLifecycleFakes) Deps {
	return Deps{Activations: activations.New(activations.Deps{
		Ensurer: f, Dispatcher: f, Sessions: f, Resetter: f, Canceller: f,
		Restarter: f, Homes: f, Audit: f,
	})}
}

// restart_bot is the non-destructive restart: no purge, no session delete.
func TestRestartBotToolIsNonDestructive(t *testing.T) {
	f := &botLifecycleFakes{}
	raw, err := NewRestartBot(newLifecycleDeps(f)).Invoke(context.Background(), tool.Invocation{
		Caller: lifecycleCaller{}, Args: json.RawMessage(`{"bot_id":"b-bird"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var got botRestartView
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != "runtime" || len(got.Preserves) == 0 {
		t.Fatalf("result = %s", raw)
	}
	if f.restarts != 1 || f.cancels != 0 || f.resets != 0 || f.retires != 0 {
		t.Fatalf("restart_bot was destructive: %+v", f)
	}
}

func TestResetBotToolRequiresConfirmationAndAuditsTheBotCaller(t *testing.T) {
	f := &botLifecycleFakes{}
	reset := NewResetBot(newLifecycleDeps(f))
	_, err := reset.Invoke(context.Background(), tool.Invocation{
		Caller: lifecycleCaller{}, Args: json.RawMessage(`{"bot_id":"b-bird","scope":"session_and_home"}`),
	})
	if !errors.Is(err, activations.ErrInvalidReset) || f.cancels+f.resets+f.retires != 0 {
		t.Fatalf("unconfirmed reset: err=%v fakes=%+v", err, f)
	}

	raw, err := reset.Invoke(context.Background(), tool.Invocation{
		Caller: lifecycleCaller{}, Args: json.RawMessage(`{"bot_id":"b-bird","scope":"session_and_home","confirm":"b-bird"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var got botResetView
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Scope != "session_and_home" || got.Home == nil || len(got.Erases) == 0 || len(got.Keeps) == 0 {
		t.Fatalf("result = %s", raw)
	}
	if f.cancels != 1 || f.resets != 1 || f.retires != 1 {
		t.Fatalf("reset did not run its declared scope: %+v", f)
	}
	last := f.audits[len(f.audits)-1]
	if last.ActorType != audit.ActorBot || last.ActorID != "b-owner" || last.Status != audit.StatusSucceeded {
		t.Fatalf("audit = %+v", last)
	}
}
