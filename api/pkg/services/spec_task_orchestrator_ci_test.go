package services

import (
	"context"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/require"
)

// recordedNotify is one captured CINotifier invocation.
type recordedNotify struct {
	taskID  string
	prID    string
	message string
}

// recordingCINotifier captures every NotifyCIResult call so tests can
// assert what (and how many times) the orchestrator notified.
type recordingCINotifier struct {
	mu    sync.Mutex
	calls []recordedNotify
	err   error
}

func (r *recordingCINotifier) NotifyCIResult(_ context.Context, task *types.SpecTask, repo *types.RepoPR, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedNotify{
		taskID:  task.ID,
		prID:    repo.PRID,
		message: message,
	})
	return r.err
}

func (r *recordingCINotifier) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// orchestrator under test — no store, no git service; we exercise the
// pure transition logic on handleCIStatusTransition directly.
func newOrchestratorForCITest(notifier CINotifier) *SpecTaskOrchestrator {
	return &SpecTaskOrchestrator{
		ciNotifier: notifier,
	}
}

func TestCITransition_FirstObservation_DoesNotNotify(t *testing.T) {
	notifier := &recordingCINotifier{}
	o := newOrchestratorForCITest(notifier)

	task := &types.SpecTask{ID: "task-1"}
	pr := &types.RepoPR{PRID: "pr-1", PRNumber: 7, RepositoryName: "owner/repo"}

	// prev is empty (first observation) — never notify.
	o.handleCIStatusTransition(context.Background(), task, pr, "", CIStatusPassed)
	o.handleCIStatusTransition(context.Background(), task, pr, "", CIStatusFailed)
	o.handleCIStatusTransition(context.Background(), task, pr, "", CIStatusRunning)

	require.Equal(t, 0, notifier.count(), "first observation must not notify")
}

func TestCITransition_RunningToPassed_NotifiesOnce(t *testing.T) {
	notifier := &recordingCINotifier{}
	o := newOrchestratorForCITest(notifier)

	task := &types.SpecTask{ID: "task-1"}
	pr := &types.RepoPR{
		PRID: "pr-1", PRNumber: 7, RepositoryName: "owner/repo",
		CIURL: "https://example.com/checks",
	}

	o.handleCIStatusTransition(context.Background(), task, pr, CIStatusRunning, CIStatusPassed)

	require.Equal(t, 1, notifier.count())
	require.Contains(t, notifier.calls[0].message, "CI passed")
	require.Contains(t, notifier.calls[0].message, "PR #7 (owner/repo)")
	require.Contains(t, notifier.calls[0].message, "https://example.com/checks")
}

func TestCITransition_RunningToFailed_NotifiesOnce(t *testing.T) {
	notifier := &recordingCINotifier{}
	o := newOrchestratorForCITest(notifier)

	task := &types.SpecTask{ID: "task-1"}
	pr := &types.RepoPR{
		PRID: "pr-2", PRNumber: 12, RepositoryName: "owner/repo",
		CIURL: "https://example.com/run/42",
	}

	o.handleCIStatusTransition(context.Background(), task, pr, CIStatusRunning, CIStatusFailed)

	require.Equal(t, 1, notifier.count())
	require.Contains(t, notifier.calls[0].message, "CI failed")
	require.Contains(t, notifier.calls[0].message, "https://example.com/run/42")
	require.Contains(t, notifier.calls[0].message, "Please investigate")
}

func TestCITransition_NoOpTransitions_DoNotNotify(t *testing.T) {
	notifier := &recordingCINotifier{}
	o := newOrchestratorForCITest(notifier)

	task := &types.SpecTask{ID: "task-1"}
	pr := &types.RepoPR{PRID: "pr-1", PRNumber: 1, RepositoryName: "r"}

	// Steady-state and other transitions — none should notify.
	cases := [][2]string{
		{CIStatusPassed, CIStatusPassed},
		{CIStatusFailed, CIStatusFailed},
		{CIStatusPassed, CIStatusFailed}, // not from running — silent (transition was missed)
		{CIStatusFailed, CIStatusPassed},
		{CIStatusRunning, CIStatusRunning},
		{CIStatusRunning, CIStatusNone},
	}
	for _, c := range cases {
		o.handleCIStatusTransition(context.Background(), task, pr, c[0], c[1])
	}

	require.Equal(t, 0, notifier.count(), "non-running-terminal transitions must not notify")
}

func TestCITransition_NilNotifier_NoPanic(t *testing.T) {
	o := newOrchestratorForCITest(nil)
	task := &types.SpecTask{ID: "task-1"}
	pr := &types.RepoPR{PRID: "pr-1", PRNumber: 1, RepositoryName: "r"}

	require.NotPanics(t, func() {
		o.handleCIStatusTransition(context.Background(), task, pr, CIStatusRunning, CIStatusPassed)
	})
}
