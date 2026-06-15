package activation_test

import (
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// TestStreamIDIsDeterministicFromWorkerID pins the wire-level shape
// of the activation-Stream ID. Every consumer of a Worker's transcript
// — worker_log, the owner-chat bridge, the streams page, hire_worker — uses
// this constructor to find the same Stream the Spawner writes to.
// Changing the shape silently is a data-loss bug; this test makes the
// shape part of the public contract.
func TestStreamIDIsDeterministicFromWorkerID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   orgchart.WorkerID
		want streaming.StreamID
	}{
		{"owner", "w-owner", "s-activations-w-owner"},
		{"ai", "w-alice", "s-activations-w-alice"},
		{"hyphenated slug", "w-product-lead", "s-activations-w-product-lead"},
		{"empty falls through to prefix only — caller's job to validate", "", "s-activations-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := activation.StreamID(tc.in)
			if got != tc.want {
				t.Fatalf("activation.StreamID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
