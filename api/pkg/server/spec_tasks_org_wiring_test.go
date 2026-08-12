package server

import (
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/services"
	helixstore "github.com/helixml/helix/api/pkg/store"
)

// Compile-time wiring assertions for the org spec-task runtime impl.
// These pin that the server-side collaborators satisfy the narrow ports
// runtimehelix.NewSpecTasks expects, so the composition in
// initHelixOrgHandler can't silently drift from the port contracts.
var (
	_ helix.SpecTaskStore    = (helixstore.Store)(nil)
	_ helix.SpecTaskWorkflow = specTaskWorkflow{}
	// The org Publishing service is the event-publish surface the
	// attention→topic bridge needs, and the bridge is the AttentionService
	// event sink.
	_ orgEventPublisher          = (*publishing.Publishing)(nil)
	_ services.AttentionEventSink = (*attentionTopicPublisher)(nil)
)
