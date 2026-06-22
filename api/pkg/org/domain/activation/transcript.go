package activation

import (
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// TranscriptID returns the deterministic id of a Worker's transcript —
// the append-only, observable log of everything the Worker did across
// its activations. One per Worker; created at hire time, written to by
// the Spawner, read by anyone with a subscription (typically the hiring
// Worker). It is a record, not a communication channel: appends are
// observed, never dispatched.
//
// This is the single canonical place that derives the
// `s-transcript-<workerID>` convention. Every caller — hire_worker,
// worker_log, the owner-chat bridge, the transcripts UI, bootstrap —
// routes through here. (The `s-` prefix marks it as a row in the shared
// topics substrate, alongside s-team-/s-dm- channels; the `transcript`
// part is its role.)
func TranscriptID(workerID orgchart.WorkerID) streaming.TopicID {
	return streaming.TopicID("s-transcript-" + string(workerID))
}
