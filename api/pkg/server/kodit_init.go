//go:build !nokodit

package server

import (
	"context"
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

	// External embedding is opt-in: only activates when an admin has set both a
	// provider AND a model in Admin > System Settings. Out of the box (no
	// settings configured), kodit uses its built-in local models — ONNX for
	// text and SigLIP2 for vision. These are not state-of-the-art but work
	// without any external dependency.
	settings, err := store.GetSystemSettings(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load system settings for kodit init: %w", err)
	}
	useExternalText := settings.KoditTextEmbeddingProvider != "" && settings.KoditTextEmbeddingModel != ""
	useExternalVision := settings.KoditVisionEmbeddingProvider != "" && settings.KoditVisionEmbeddingModel != ""

	// Text embedding provider.
	if useExternalText {
		textEmbedBaseURL := cfg.Kodit.TextEmbeddingBaseURL
		if textEmbedBaseURL == "" {
			textEmbedBaseURL = cfg.Kodit.LLMBaseURL
		}
		textEmbedAPIKey := cfg.Kodit.TextEmbeddingAPIKey
		if textEmbedAPIKey == "" {
			textEmbedAPIKey = cfg.Kodit.LLMAPIKey
		}
		textEmbedder := provider.NewOpenAIProviderFromConfig(provider.OpenAIConfig{
			APIKey:         textEmbedAPIKey,
			BaseURL:        textEmbedBaseURL,
			EmbeddingModel: cfg.Kodit.TextEmbeddingModel, // "kodit-text-embedding" placeholder routed by Helix
		})
		koditOpts = append(koditOpts, kodit.WithEmbeddingProvider(textEmbedder))
		log.Info().
			Str("provider", settings.KoditTextEmbeddingProvider).
			Str("model", settings.KoditTextEmbeddingModel).
			Msg("Kodit text embedding: external provider via Helix proxy")
	} else {
		embedder := provider.NewHugotEmbedding(modelDir)
		koditOpts = append(koditOpts, kodit.WithEmbeddingProvider(embedder))
		log.Info().Str("model_dir", modelDir).Msg("Kodit text embedding: built-in local ONNX model (configure external in System Settings for better results)")
	}

	// Vision embedding provider.
	if useExternalVision {
		visionEmbedBaseURL := cfg.Kodit.VisionEmbeddingBaseURL
		if visionEmbedBaseURL == "" {
			visionEmbedBaseURL = cfg.Kodit.LLMBaseURL
		}
		visionEmbedAPIKey := cfg.Kodit.VisionEmbeddingAPIKey
		if visionEmbedAPIKey == "" {
			visionEmbedAPIKey = cfg.Kodit.LLMAPIKey
		}
		visionEmbedder := provider.NewOpenAIVisionProvider(provider.OpenAIConfig{
			APIKey:         visionEmbedAPIKey,
			BaseURL:        visionEmbedBaseURL,
			EmbeddingModel: cfg.Kodit.VisionEmbeddingModel, // "kodit-vision-embedding" placeholder routed by Helix
		})
		koditOpts = append(koditOpts, kodit.WithVisionEmbedder(visionEmbedder))
		log.Info().
			Str("provider", settings.KoditVisionEmbeddingProvider).
			Str("model", settings.KoditVisionEmbeddingModel).
			Msg("Kodit vision embedding: external provider via Helix proxy")
	} else {
		log.Info().Msg("Kodit vision embedding: built-in local SigLIP2 (configure external in System Settings for better results)")
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
	// this gives operators a clear, actionable message. Needed whenever we
	// fall back to a local model (text ONNX or vision SigLIP2).
	ortLibDir := os.Getenv("ORT_LIB_DIR")
	if ortLibDir != "" && (!useExternalText || !useExternalVision) {
		ortPath := filepath.Join(ortLibDir, "libonnxruntime.so")
		if _, err := os.Stat(ortPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("kodit is enabled but %s not found — ensure the container image was built with the ORT build stage (see Dockerfile)", ortPath)
		}
	}

	gitRepoService.SetKoditGitURL(cfg.Kodit.GitURL)

	// Fast path: no external embedding configured, so kodit.New() only touches
	// local files — no listener dependency, run synchronously.
	if !useExternalText && !useExternalVision {
		client, err := kodit.New(koditOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize kodit client: %w", err)
		}
		svc := services.NewKoditService(client)
		gitRepoService.SetKoditService(svc)
		log.Info().
			Str("kodit_git_url", cfg.Kodit.GitURL).
			Msg("Initialized Kodit code intelligence service")
		return &KoditResult{
			Service:    svc,
			mcpBackend: NewKoditMCPBackend(client, true, store),
			closer:     client,
		}, nil
	}

	// External embedding configured: kodit.New()'s dimension probe hits the
	// configured endpoint. When that endpoint is Helix's own proxy (the usual
	// case) the listener isn't bound yet, so run kodit.New asynchronously with
	// a retry loop and hand back a placeholder service that swaps in once the
	// probe succeeds.
	svc := services.NewDeferredKoditService()
	mcpBackend := NewKoditMCPBackend(nil, true, store)
	gitRepoService.SetKoditService(svc)

	result := &KoditResult{
		Service:    svc,
		mcpBackend: mcpBackend,
	}

	go func() {
		client, err := newKoditClientWithRetry(koditOpts)
		if err != nil {
			log.Error().Err(err).Msg("kodit init failed — code intelligence will remain disabled until reconfigured and restarted")
			return
		}
		svc.Set(services.NewKoditService(client))
		mcpBackend.setClient(client)
		result.closer = client
		log.Info().
			Str("kodit_git_url", cfg.Kodit.GitURL).
			Msg("Initialized Kodit code intelligence service (external embedding)")
	}()

	return result, nil
}

// newKoditClientWithRetry calls kodit.New with a retry loop. Used when the
// embedding endpoint is Helix's own listener — the first few attempts hit
// connection-refused while the listener binds.
func newKoditClientWithRetry(opts []kodit.Option) (*kodit.Client, error) {
	const maxAttempts = 30
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
