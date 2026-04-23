//go:build nokodit

package server

import (
	"context"
	"io"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/rs/zerolog/log"
)

// KoditResult holds everything produced by InitKodit.
// It is exported so that serve.go can initialize kodit once and share the
// service between the RAG factory and the API server.
type KoditResult struct {
	Service    services.KoditServicer
	mcpBackend *KoditMCPBackend
	closer     io.Closer
}

// InitKodit returns a disabled kodit service when built without kodit support.
func InitKodit(_ *config.ServerConfig, _ *services.GitRepositoryService, _ store.Store) (*KoditResult, error) {
	log.Info().Msg("Kodit code intelligence service not available (nokodit build)")
	return &KoditResult{
		Service:    services.NewDisabledKoditService(),
		mcpBackend: newKoditMCPBackend(),
	}, nil
}

// Reinit is a no-op for the nokodit build.
func (k *KoditResult) Reinit(_ context.Context) error { return nil }
