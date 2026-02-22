//go:build !nokodit

package server

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/kodit"
	"github.com/helixml/kodit/infrastructure/provider"
	"github.com/rs/zerolog/log"
)

// koditResult holds everything produced by initKodit.
type koditResult struct {
	service    services.KoditServicer
	mcpBackend *KoditMCPBackend
	closer     io.Closer
}

// initKodit creates the kodit client, service, and MCP backend.
// When kodit is disabled in config it returns a disabled service and nil closer.
func initKodit(cfg *config.ServerConfig, gitRepoService *services.GitRepositoryService) (*koditResult, error) {
	if !cfg.Kodit.Enabled || gitRepoService == nil {
		log.Info().Msg("Kodit code intelligence service disabled")
		return &koditResult{
			service:    services.NewDisabledKoditService(),
			mcpBackend: NewKoditMCPBackend(nil, false),
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

	return &koditResult{
		service:    svc,
		mcpBackend: NewKoditMCPBackend(koditClient, true),
		closer:     koditClient,
	}, nil
}
