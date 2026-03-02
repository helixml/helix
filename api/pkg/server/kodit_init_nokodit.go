//go:build nokodit

package server

import (
	"io"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/rs/zerolog/log"
)

// koditResult holds everything produced by initKodit.
type koditResult struct {
	service    services.KoditServicer
	mcpBackend *KoditMCPBackend
	closer     io.Closer
}

// initKodit returns a disabled kodit service when built without kodit support.
func initKodit(_ *config.ServerConfig, _ *services.GitRepositoryService) (*koditResult, error) {
	log.Info().Msg("Kodit code intelligence service not available (nokodit build)")
	return &koditResult{
		service:    services.NewDisabledKoditService(),
		mcpBackend: newKoditMCPBackend(),
	}, nil
}
