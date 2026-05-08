//go:build !nokodit

package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/kodit"
	"github.com/helixml/kodit/infrastructure/provider"
	"github.com/rs/zerolog/log"
)

// KoditResult holds everything produced by InitKodit. It is exported so that
// serve.go can initialize kodit once and share the service between the RAG
// factory and the API server. It also owns the Reinit() path so System
// Settings changes to the kodit embedding model can be applied without a
// process restart.
type KoditResult struct {
	Service    services.KoditServicer
	mcpBackend *KoditMCPBackend
	closer     io.Closer

	// Fields below support in-process reinit when the admin changes the kodit
	// embedding model in System Settings. They are nil when kodit is disabled.
	koditService   *services.KoditService
	cfg            *config.ServerConfig
	store          store.Store
	gitRepoService *services.GitRepositoryService

	// mu serialises Reinit calls.
	mu sync.Mutex
}

// InitKodit creates the kodit client, service, and MCP backend. When kodit is
// disabled in config it returns a disabled service and nil closer.
//
// kodit.New is called synchronously. The season-valley kodit branch defers
// the embedding-dimension probe to first use, so there is no listener
// dependency at startup even when the embedding provider is configured to
// call back into Helix's own /v1/embeddings proxy.
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

	opts, decision, err := buildKoditOpts(cfg, store)
	if err != nil {
		return nil, err
	}
	if err := preflightORT(decision); err != nil {
		return nil, err
	}

	client, err := kodit.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kodit client: %w", err)
	}

	// KoditService wraps the *kodit.Client in an atomic pointer so Reinit can
	// swap it in place — downstream consumers keep their KoditServicer
	// reference, the client under the hood changes.
	svc := services.NewKoditService(client)
	mcpBackend := NewKoditMCPBackend(client, true, store)
	gitRepoService.SetKoditGitURL(cfg.Kodit.GitURL)
	gitRepoService.SetKoditService(svc)

	log.Info().
		Str("kodit_git_url", cfg.Kodit.GitURL).
		Msg("Initialized Kodit code intelligence service")

	return &KoditResult{
		Service:        svc,
		mcpBackend:     mcpBackend,
		closer:         client,
		koditService:   svc,
		cfg:            cfg,
		store:          store,
		gitRepoService: gitRepoService,
	}, nil
}

// Reinit rebuilds the kodit client from the current System Settings. Called
// when an admin changes the kodit embedding provider/model so the new choice
// takes effect without a process restart.
//
// Ordering:
//  1. Build new opts from fresh settings + call kodit.New — if this fails
//     the live client is untouched.
//  2. Atomically swap the new client into the KoditService pointer so new
//     queries hit the new client.
//  3. Update the MCP backend to use the new client.
//  4. Close the old client to drain workers and release its DB handle.
//  5. Enqueue a sync for every registered repository. kodit's dim-change
//     detection is lazy — per embedding table, on first store method call
//     — so if we rely on the first user action to trigger it, a search
//     against a table that hasn't been touched yet since the swap (e.g.
//     vision after switching only-text users) fails with a SQL dimension
//     mismatch. Forcing a sync exercises every embedder and rebuilds every
//     stale table before a user can query against it.
func (k *KoditResult) Reinit(ctx context.Context) error {
	if k == nil || k.koditService == nil {
		return fmt.Errorf("kodit reinit: not initialised")
	}
	k.mu.Lock()
	defer k.mu.Unlock()

	log.Info().Msg("kodit reinit requested")

	opts, decision, err := buildKoditOpts(k.cfg, k.store)
	if err != nil {
		return fmt.Errorf("kodit reinit: build opts: %w", err)
	}
	if err := preflightORT(decision); err != nil {
		return fmt.Errorf("kodit reinit: preflight: %w", err)
	}

	newClient, err := kodit.New(opts...)
	if err != nil {
		return fmt.Errorf("kodit reinit: new client: %w", err)
	}

	oldClient := k.koditService.SwapClient(newClient)
	k.mcpBackend.setClient(newClient)
	k.closer = newClient

	if oldClient != nil {
		if err := oldClient.Close(); err != nil {
			log.Warn().Err(err).Msg("error closing old kodit client during reinit")
		}
	}

	// Fire-and-forget: rescan every repository so every embedding table
	// gets rebuilt before a user searches. Rescan (not Sync) is required —
	// Sync skips commits whose enrichments already exist, which would leave
	// stale-dimension vectors in the tables untouched and fail the next
	// vector query with a SQL dimension mismatch.
	go func() {
		if err := k.koditService.RescanAllRepositories(context.Background()); err != nil {
			log.Error().Err(err).Msg("kodit reinit: rescan-all failed")
			return
		}
		log.Info().Msg("kodit reinit: rescan-all enqueued")
	}()

	log.Info().Msg("kodit reinit complete; repositories will be re-indexed in background")
	return nil
}

// buildKoditOpts reads the current System Settings and returns the kodit
// options needed to start a client, along with the decision about which
// embedding providers are external vs built-in.
func buildKoditOpts(cfg *config.ServerConfig, s store.Store) ([]kodit.Option, koditEmbeddingDecision, error) {
	var koditOpts []kodit.Option
	koditOpts = append(koditOpts, kodit.WithPostgresVectorchord(cfg.Kodit.DatabaseURL))

	dataDir := cfg.Kodit.DataDir
	if dataDir == "" {
		dataDir = filepath.Join(cfg.FileStore.LocalFSPath, "kodit")
	}
	koditOpts = append(koditOpts, kodit.WithDataDir(dataDir))

	modelDir := cfg.Kodit.ModelDir
	if modelDir == "" {
		modelDir = filepath.Join(dataDir, "models")
	}
	koditOpts = append(koditOpts, kodit.WithModelDir(modelDir))

	settings, err := s.GetSystemSettings(context.Background())
	if err != nil {
		return nil, koditEmbeddingDecision{}, fmt.Errorf("load system settings for kodit: %w", err)
	}
	decision := decideKoditEmbedding(settings)

	// Text embedding provider.
	if decision.UseExternalText {
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
	if decision.UseExternalVision {
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

	koditOpts = append(koditOpts, kodit.WithLogger(log.Logger))

	return koditOpts, decision, nil
}

// preflightORT verifies the ONNX Runtime shared library is present whenever
// any built-in local model (text ONNX or vision SigLIP2) will be used. The
// underlying hugot error is cryptic; this gives operators a clear, actionable
// message.
func preflightORT(decision koditEmbeddingDecision) error {
	ortLibDir := os.Getenv("ORT_LIB_DIR")
	if ortLibDir == "" {
		return nil
	}
	if decision.UseExternalText && decision.UseExternalVision {
		return nil
	}
	ortPath := filepath.Join(ortLibDir, "libonnxruntime.so")
	if _, err := os.Stat(ortPath); os.IsNotExist(err) {
		return fmt.Errorf("kodit is enabled but %s not found — ensure the container image was built with the ORT build stage (see Dockerfile)", ortPath)
	}
	return nil
}
