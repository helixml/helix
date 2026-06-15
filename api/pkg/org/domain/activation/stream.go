package activation

import (
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// StreamID returns the deterministic Stream ID where a Worker's
// activation transcript is published. One Stream per Worker; created
// at hire time, written to by the Spawner, read by anyone with a
// subscription (typically the hiring Worker).
//
// This is the single canonical place that derives the
// `s-activations-<workerID>` convention. Every caller — hire_worker,
// worker_log, the owner-chat bridge, the streams page, bootstrap —
// routes through here. Lifted from api/pkg/org/agent.ActivationStreamID
// in B5.1.
func StreamID(workerID orgchart.WorkerID) streaming.StreamID {
	return streaming.StreamID("s-activations-" + string(workerID))
}
