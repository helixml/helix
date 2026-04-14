//go:build !nokodit

package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

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

	// Model directory is always set (needed for local SigLIP2 fallback when vision embedding is not externally configured).
	modelDir := cfg.Kodit.ModelDir
	if modelDir == "" {
		modelDir = filepath.Join(dataDir, "models")
	}
	koditOpts = append(koditOpts, kodit.WithModelDir(modelDir))

	// Text embedding provider: either external (proxied through Helix) or local ONNX.
	textEmbedBaseURL := cfg.Kodit.TextEmbeddingBaseURL
	if textEmbedBaseURL == "" {
		textEmbedBaseURL = cfg.Kodit.LLMBaseURL
	}
	textEmbedAPIKey := cfg.Kodit.TextEmbeddingAPIKey
	if textEmbedAPIKey == "" {
		textEmbedAPIKey = cfg.Kodit.LLMAPIKey
	}
	useLocalTextEmbedding := textEmbedBaseURL == ""
	if useLocalTextEmbedding {
		embedder := provider.NewHugotEmbedding(modelDir)
		koditOpts = append(koditOpts, kodit.WithEmbeddingProvider(embedder))
		log.Info().Str("model_dir", modelDir).Msg("Kodit text embedding: local ONNX HugotEmbedding")
	} else {
		textEmbedModel := cfg.Kodit.TextEmbeddingModel
		if textEmbedModel == "" {
			textEmbedModel = "kodit-text-embedding"
		}
		textEmbedder := provider.NewOpenAIProviderFromConfig(provider.OpenAIConfig{
			APIKey:         textEmbedAPIKey,
			BaseURL:        textEmbedBaseURL,
			EmbeddingModel: textEmbedModel,
		})
		koditOpts = append(koditOpts, kodit.WithEmbeddingProvider(textEmbedder))
		log.Info().Str("base_url", textEmbedBaseURL).Str("model", textEmbedModel).Msg("Kodit text embedding: external provider via Helix proxy")
	}

	// Vision embedding provider: either external (proxied through Helix) or local SigLIP2 fallback.
	visionEmbedBaseURL := cfg.Kodit.VisionEmbeddingBaseURL
	if visionEmbedBaseURL == "" {
		visionEmbedBaseURL = cfg.Kodit.LLMBaseURL
	}
	visionEmbedAPIKey := cfg.Kodit.VisionEmbeddingAPIKey
	if visionEmbedAPIKey == "" {
		visionEmbedAPIKey = cfg.Kodit.LLMAPIKey
	}
	if visionEmbedBaseURL != "" {
		visionEmbedModel := cfg.Kodit.VisionEmbeddingModel
		if visionEmbedModel == "" {
			visionEmbedModel = "kodit-vision-embedding"
		}
		visionEmbedder := provider.NewOpenAIVisionProvider(provider.OpenAIConfig{
			APIKey:         visionEmbedAPIKey,
			BaseURL:        visionEmbedBaseURL,
			EmbeddingModel: visionEmbedModel,
		})
		koditOpts = append(koditOpts, kodit.WithVisionEmbedder(visionEmbedder))
		log.Info().Str("base_url", visionEmbedBaseURL).Str("model", visionEmbedModel).Msg("Kodit vision embedding: external provider via Helix proxy")
	} else {
		log.Info().Msg("Kodit vision embedding: local SigLIP2 (default)")
	}

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

	// Pre-flight: verify the ONNX Runtime shared library is present before
	// attempting to create the kodit client. The hugot error is cryptic;
	// this gives operators a clear, actionable message.
	// Only needed when using local ONNX text embedding (and always needed for
	// local SigLIP2 vision fallback).
	ortLibDir := os.Getenv("ORT_LIB_DIR")
	if ortLibDir != "" && (useLocalTextEmbedding || visionEmbedBaseURL == "") {
		ortPath := filepath.Join(ortLibDir, "libonnxruntime.so")
		if _, err := os.Stat(ortPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("kodit is enabled but %s not found — ensure the container image was built with the ORT build stage (see Dockerfile)", ortPath)
		}
	}

	gitRepoService.SetKoditGitURL(cfg.Kodit.GitURL)

	// Initialize the kodit client. When the embedding provider is configured
	// to call an external HTTP endpoint (including Helix's own /v1/embeddings
	// proxy), kodit.New() runs a dimension probe that may transiently fail if
	// the upstream is still warming up — retry a few times before giving up.
	koditClient, err := newKoditClientWithRetry(koditOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kodit client: %w", err)
	}
	svc := services.NewKoditService(koditClient)
	gitRepoService.SetKoditService(svc)
	log.Info().
		Str("kodit_git_url", cfg.Kodit.GitURL).
		Msg("Initialized Kodit code intelligence service (in-process)")
	return &KoditResult{
		Service:    svc,
		mcpBackend: NewKoditMCPBackend(koditClient, true, store),
		closer:     koditClient,
	}, nil
}

// newKoditClientWithRetry calls kodit.New with a small backoff retry loop.
// kodit.New probes the configured embedding endpoint to discover its vector
// dimension; if the upstream is briefly unavailable we want to give it a
// chance to come up rather than hard-failing the whole server.
func newKoditClientWithRetry(opts []kodit.Option) (*kodit.Client, error) {
	const maxAttempts = 5
	const delay = 2 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		client, err := kodit.New(opts...)
		if err == nil {
			return client, nil
		}
		lastErr = err
		if attempt < maxAttempts {
			log.Warn().
				Err(err).
				Int("attempt", attempt).
				Int("max_attempts", maxAttempts).
				Dur("retry_in", delay).
				Msg("kodit.New failed, retrying")
			time.Sleep(delay)
		}
	}
	return nil, lastErr
}
