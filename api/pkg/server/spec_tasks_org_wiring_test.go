package server

import (
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	helixstore "github.com/helixml/helix/api/pkg/store"
)

// Compile-time wiring assertions for the org spec-task runtime impl.
// These pin that the server-side collaborators satisfy the narrow ports
// runtimehelix.NewSpecTasks expects, so the composition in
// initHelixOrgHandler can't silently drift from the port contracts.
var (
	_ helix.SpecTaskStore    = (helixstore.Store)(nil)
	_ helix.SpecTaskWorkflow = specTaskWorkflow{}
)
