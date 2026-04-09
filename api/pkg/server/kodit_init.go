//go:build !nokodit

package server

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/kodit"
	"github.com/helixml/kodit/infrastructure/provider"
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

// InitKodit creates the kodit client, service, and MCP backend.
// When kodit is disabled in config it returns a disabled service and nil closer.
func InitKodit(cfg *config.ServerConfig, gitRepoService *services.GitRepositoryService, store store.Store) (*KoditResult, error) {
	if !cfg.Kodit.Enabled || gitRepoService == nil {
		log.Info().Msg("Kodit code intelligence service disabled")
		return &KoditResult{
			Service:    services.NewDisabledKoditService(),
			mcpBackend: NewKoditMCPBackend(nil, false, store),
		}, nil
	}

	if cfg.Kodit.DatabaseURL == "" {
		return nil, fmt.Errorf("KODIT_DB_URL is required when kodit is enabled")
	}

	var koditOpts []kodit.Option
	koditOpts = append(koditOpts, kodit.WithPostgresVectorchord(cfg.Kodit.DatabaseURL))

	// Data directory (for cloned repos, model cache, etc.)
	dataDir := cfg.Kodit.DataDir
	if dataDir == "" {
		dataDir = filepath.Join(cfg.FileStore.LocalFSPath, "kodit")
	}
	koditOpts = append(koditOpts, kodit.WithDataDir(dataDir))

	// Embedding provider: local ONNX model loaded from disk.
	modelDir := cfg.Kodit.ModelDir
	if modelDir == "" {
		modelDir = filepath.Join(dataDir, "models")
	}
	koditOpts = append(koditOpts, kodit.WithModelDir(modelDir))
	embedder := provider.NewHugotEmbedding(modelDir)
	koditOpts = append(koditOpts, kodit.WithEmbeddingProvider(embedder))

	// LLM text provider for enrichments (separate from embedding).
	if cfg.Kodit.LLMBaseURL != "" {
		p := provider.NewOpenAIProviderFromConfig(provider.OpenAIConfig{
			APIKey:    cfg.Kodit.LLMAPIKey,
			BaseURL:   cfg.Kodit.LLMBaseURL,
			ChatModel: cfg.Kodit.LLMChatModel,
		})
		koditOpts = append(koditOpts, kodit.WithTextProvider(p))
	} else {
		koditOpts = append(koditOpts, kodit.WithSkipProviderValidation())
	}

	if cfg.Kodit.WorkerCount > 0 {
		koditOpts = append(koditOpts, kodit.WithWorkerCount(cfg.Kodit.WorkerCount))
	}

	// Pass helix's zerolog logger to kodit so log output is consistent.
	koditOpts = append(koditOpts, kodit.WithLogger(log.Logger))

	koditClient, err := kodit.New(koditOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kodit client: %w", err)
	}

	svc := services.NewKoditService(koditClient)
	gitRepoService.SetKoditService(svc)
	gitRepoService.SetKoditGitURL(cfg.Kodit.GitURL)

	log.Info().
		Str("kodit_git_url", cfg.Kodit.GitURL).
		Msg("Initialized Kodit code intelligence service (in-process)")

	return &KoditResult{
		Service:    svc,
		mcpBackend: NewKoditMCPBackend(koditClient, true, store),
		closer:     koditClient,
	}, nil
}
