package external_agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

func TestStopIdleDesktopLeavesSessionRunningWhenStopFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	executor := NewMockExecutor(ctrl)
	mockStore := store.NewMockStore(ctrl)
	session := &types.Session{ID: "ses_live"}

	executor.EXPECT().StopDesktop(gomock.Any(), session.ID).Return(errors.New("hydra disconnected"))

	stopIdleDesktop(context.Background(), executor, mockStore, session)

	// The strict store mock has no expectations: a failed stop must not reap a
	// live interaction or mark the still-running desktop terminated_idle.
}

func TestSleepGraceHoldsShutdownsAfterHostWake(t *testing.T) {
	const idle = time.Hour
	const tick = 5 * time.Minute
	g := sleepGrace{idleTimeout: idle}
	now := time.Date(2026, 11, 2, 6, 26, 16, 0, time.UTC)

	if held, woke := g.observe(now, tick, tick); held || woke {
		t.Fatalf("normal tick: held=%v woke=%v, want neither", held, woke)
	}

	// Lid closed for 17h28m: the wall clock was stepped forward on wake while
	// the monotonic clock only saw the usual tick.
	now = now.Add(tick)
	if held, woke := g.observe(now, 17*time.Hour+28*time.Minute, tick); !held || !woke {
		t.Fatalf("wake tick: held=%v woke=%v, want both", held, woke)
	}

	// Held for a full idle timeout after the wake, then normal again.
	for elapsed := tick; elapsed < idle; elapsed += tick {
		if held, _ := g.observe(now.Add(elapsed), tick, tick); !held {
			t.Fatalf("%v after wake: not held, want held until %v", elapsed, idle)
		}
	}
	if held, _ := g.observe(now.Add(idle), tick, tick); held {
		t.Fatalf("%v after wake: still held", idle)
	}
}

func TestSleepGraceIgnoresSmallClockCorrections(t *testing.T) {
	g := sleepGrace{idleTimeout: time.Hour}
	now := time.Date(2026, 11, 2, 6, 0, 0, 0, time.UTC)
	// An NTP step of a few seconds is not a sleep.
	if held, woke := g.observe(now, 5*time.Minute+30*time.Second, 5*time.Minute); held || woke {
		t.Fatalf("30s correction: held=%v woke=%v, want neither", held, woke)
	}
}
