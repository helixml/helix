//go:build !nokodit

package server

import (
	"fmt"
	"io"
	"os"
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

	// deferredStart is non-nil when kodit.New() was deferred to run in a
	// goroutine (e.g. because the embedding provider points at Helix's own
	// API, which is not yet listening). Call it AFTER the HTTP server is up.
	deferredStart func()
}

// StartDeferred runs any pending deferred kodit.New() call. Safe to call
// multiple times; subsequent calls are no-ops.
func (k *KoditResult) StartDeferred() {
	if k == nil || k.deferredStart == nil {
		return
	}
	start := k.deferredStart
	k.deferredStart = nil
	start()
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

	// Determine whether kodit needs to call back into Helix's own /v1/embeddings
	// endpoint for its embedding provider. If so, kodit.New()'s dimension probe
	// will fail unless the HTTP listener is up — so we defer the kodit.New()
	// call to a goroutine that runs once StartDeferred is called (typically
	// right after the HTTP listener binds).
	needsDeferredStart := false
	if !useLocalTextEmbedding && isLoopbackURL(textEmbedBaseURL) {
		needsDeferredStart = true
	}
	if visionEmbedBaseURL != "" && isLoopbackURL(visionEmbedBaseURL) {
		needsDeferredStart = true
	}

	// Register Git URL and common service wiring now, regardless of deferral.
	gitRepoService.SetKoditGitURL(cfg.Kodit.GitURL)

	if !needsDeferredStart {
		koditClient, err := kodit.New(koditOpts...)
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

	// Deferred path: return a placeholder service that swaps to the real one
	// once kodit.New() completes in a goroutine (after the HTTP server is up).
	deferred := services.NewDeferredKoditService()
	mcpBackend := NewKoditMCPBackend(nil, true, store)
	gitRepoService.SetKoditService(deferred)

	result := &KoditResult{
		Service:    deferred,
		mcpBackend: mcpBackend,
	}

	result.deferredStart = func() {
		go func() {
			log.Info().Msg("Starting deferred kodit init (external embedding provider via Helix proxy)")
			koditClient, err := kodit.New(koditOpts...)
			if err != nil {
				log.Error().Err(err).Msg("deferred kodit init failed — Code Intelligence will remain disabled until reconfigured and restarted")
				return
			}
			svc := services.NewKoditService(koditClient)
			deferred.Set(svc)
			mcpBackend.SetClient(koditClient, true)
			log.Info().
				Str("kodit_git_url", cfg.Kodit.GitURL).
				Msg("Deferred Kodit init completed — Code Intelligence enabled")
		}()
	}

	return result, nil
}

// isLoopbackURL returns true when the given URL's host resolves to localhost
// (or 127.0.0.1/[::1]). When kodit's embedding provider points at such a host
// it targets Helix's own API, which requires deferred init so the HTTP
// listener can bind first.
func isLoopbackURL(rawURL string) bool {
	// Simple substring checks to avoid importing net/url for a one-liner.
	// The env var is controlled by operators so exotic schemes are unlikely.
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		if containsHost(rawURL, host) {
			return true
		}
	}
	return false
}

func containsHost(url, host string) bool {
	// Match "://host" or "://host:" or "@host" to avoid false positives on paths.
	for _, prefix := range []string{"://", "@"} {
		idx := indexOf(url, prefix+host)
		if idx < 0 {
			continue
		}
		after := idx + len(prefix) + len(host)
		if after == len(url) {
			return true
		}
		c := url[after]
		if c == ':' || c == '/' || c == '?' || c == '#' {
			return true
		}
	}
	return false
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
