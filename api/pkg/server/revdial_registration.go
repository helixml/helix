package server

import (
	"strings"
	"sync/atomic"
	"time"
)

// revDialLivenessWindow is how long a RevDial control connection counts as
// alive after its last pong. The API pings every 15s, so three missed pongs
// release the id for a reconnecting client.
const revDialLivenessWindow = 45 * time.Second

// runnerTokenMayRegisterRevDial reports whether a runner-token caller may
// register a RevDial tunnel under runnerID. Sandbox hosts register
// "hydra-{id}" (Hydra) and "sandbox-{id}"; every other tunnel (notably
// "desktop-{session}") is registered with a key bound to its session.
func runnerTokenMayRegisterRevDial(runnerID string) bool {
	for _, prefix := range []string{"hydra-", "sandbox-"} {
		if rest, ok := strings.CutPrefix(runnerID, prefix); ok && rest != "" {
			return true
		}
	}
	return false
}

// revDialLiveness records when a RevDial control connection last answered a
// ping.
type revDialLiveness struct {
	lastPong atomic.Int64 // unix nanoseconds
}

func newRevDialLiveness(now time.Time) *revDialLiveness {
	l := &revDialLiveness{}
	l.touch(now)
	return l
}

func (l *revDialLiveness) touch(now time.Time) {
	l.lastPong.Store(now.UnixNano())
}

func (l *revDialLiveness) alive(now time.Time) bool {
	return now.Sub(time.Unix(0, l.lastPong.Load())) < revDialLivenessWindow
}
