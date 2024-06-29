package runner

import (
	"github.com/helixml/helix/api/pkg/config"
	"github.com/puzpuzpuz/xsync/v3"
)

// TODO: add used/free memory management interface
// and reuse it between old and new controllers

type LLMController struct {
	cfg *config.RunnerConfig

	// the map of model instances that we have loaded
	// and are currently running
	instances *xsync.MapOf[string, LLMModelInstance]
}
