package server

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// TestIsOrphanedWaitingInteraction exercises every branch of the boot / live /
// orphan matrix for the queue-side liveness guard that unwedges a session whose
// desktop died mid-turn. The bug it protects against: a state=waiting turn whose
// agent has gone away never gets message_completed, so the prompt-queue treats
// the session as perpetually busy and the desktop never resumes.
func TestIsOrphanedWaitingInteraction(t *testing.T) {
	now := time.Date(2026, 7, 6, 18, 0, 0, 0, time.UTC)
	stale := now.Add(-desktopResumeReapStaleThreshold - time.Minute) // comfortably past threshold
	fresh := now.Add(-5 * time.Second)                               // just created / mid-boot

	sessionWithThread := func() *types.Session {
		s := &types.Session{}
		s.Metadata.ZedThreadID = "ccc26138-d4dc-4cb5-9126-55424d500609"
		return s
	}
	sessionNoThread := func() *types.Session {
		s := &types.Session{}
		s.Metadata.ZedThreadID = ""
		return s
	}
	waiting := func(updated time.Time) *types.Interaction {
		return &types.Interaction{State: types.InteractionStateWaiting, Updated: updated}
	}

	cases := []struct {
		name    string
		session *types.Session
		latest  *types.Interaction
		wsLive  bool
		want    bool
	}{
		{
			name:    "orphaned: stale waiting, no WS, thread established -> reap",
			session: sessionWithThread(), latest: waiting(stale), wsLive: false, want: true,
		},
		{
			name:    "live WS present -> genuinely busy, do not reap",
			session: sessionWithThread(), latest: waiting(stale), wsLive: true, want: false,
		},
		{
			name:    "boot race: thread not yet established -> never reap",
			session: sessionNoThread(), latest: waiting(stale), wsLive: false, want: false,
		},
		{
			name:    "fresh in-flight turn within staleness window -> protected",
			session: sessionWithThread(), latest: waiting(fresh), wsLive: false, want: false,
		},
		{
			name:    "latest interaction not waiting -> nothing to reap",
			session: sessionWithThread(), latest: &types.Interaction{State: types.InteractionStateComplete, Updated: stale}, wsLive: false, want: false,
		},
		{
			name:    "nil latest -> false",
			session: sessionWithThread(), latest: nil, wsLive: false, want: false,
		},
		{
			name:    "nil session -> false",
			session: nil, latest: waiting(stale), wsLive: false, want: false,
		},
		{
			name:    "exactly at threshold (not strictly past) -> not yet orphaned",
			session: sessionWithThread(), latest: waiting(now.Add(-desktopResumeReapStaleThreshold)), wsLive: false, want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isOrphanedWaitingInteraction(tc.session, tc.latest, tc.wsLive, now)
			if got != tc.want {
				t.Fatalf("isOrphanedWaitingInteraction = %v, want %v", got, tc.want)
			}
		})
	}
}
