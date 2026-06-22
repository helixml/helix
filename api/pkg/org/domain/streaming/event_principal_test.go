package streaming_test

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// TestEventSourcePrincipalLiftsWorkerSource pins B6.3 on the Event
// side: today Event.Source is a orgchart.WorkerID typed string (the
// publishing Worker, or empty for system-emitted). SourcePrincipal
// lifts that into a typed streaming.Principal so downstream code
// (dispatcher, worker_log, /ui/topics) can read a Principal without
// inferring from value shape per-call.
//
// As the alpha grows transports that emit Events with non-Worker
// senders (an inbound email whose sender hasn't been resolved to a
// Worker), the underlying Source field type widens — this accessor
// is the first step.
func TestEventSourcePrincipalLiftsWorkerSource(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		source string
		want   streaming.Principal
	}{
		{"empty → zero-Principal (system-emitted)", "", streaming.Principal{}},
		{"worker source → Worker principal", "w-alice", streaming.NewPrincipalWorker("w-alice")},
		{"owner source → Worker principal", "w-owner", streaming.NewPrincipalWorker("w-owner")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e, err := streaming.NewEvent("e-1", "s-x", orgchart.WorkerID(tc.source), "body", time.Now(), "org-test")
			if err != nil {
				t.Fatalf("new event: %v", err)
			}
			if got := e.SourcePrincipal(); got != tc.want {
				t.Errorf("SourcePrincipal() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
