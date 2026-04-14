//go:build !nokodit

package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

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
	deferred       *services.DeferredKoditService
	cfg            *config.ServerConfig
	store          store.Store
	gitRepoService *services.GitRepositoryService
	modelDir       string

	// mu serialises Reinit calls and protects client. client is the currently
	// active kodit.Client (may be nil during a reinit or before the first
	// async init completes).
	mu     sync.Mutex
	client *kodit.Client
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

	// Use a deferred (atomically-swappable) service everywhere so that Reinit
	// can swap the real inner service without having to rewire downstream
	// consumers that hold a reference to KoditResult.Service.
	deferred := services.NewDeferredKoditService()
	mcpBackend := NewKoditMCPBackend(nil, true, store)
	gitRepoService.SetKoditGitURL(cfg.Kodit.GitURL)
	gitRepoService.SetKoditService(deferred)

	result := &KoditResult{
		Service:        deferred,
		mcpBackend:     mcpBackend,
		deferred:       deferred,
		cfg:            cfg,
		store:          store,
		gitRepoService: gitRepoService,
	}

	opts, modelDir, decision, err := buildKoditOpts(cfg, store)
	if err != nil {
		return nil, err
	}
	result.modelDir = modelDir

	if err := preflightORT(decision); err != nil {
		return nil, err
	}

	// Fast path: no external embedding configured, so kodit.New() only touches
	// local files — no listener dependency, run synchronously.
	if !decision.UseExternalText && !decision.UseExternalVision {
		client, err := kodit.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize kodit client: %w", err)
		}
		result.installClient(client)
		log.Info().
			Str("kodit_git_url", cfg.Kodit.GitURL).
			Msg("Initialized Kodit code intelligence service")
		return result, nil
	}

	// External embedding configured: kodit.New()'s dimension probe hits the
	// configured endpoint. When that endpoint is Helix's own proxy (the usual
	// case) the listener isn't bound yet at init time, so run kodit.New
	// asynchronously with a retry loop.
	go func() {
		client, err := newKoditClientWithRetry(opts)
		if err != nil {
			log.Error().Err(err).Msg("kodit init failed — code intelligence will remain disabled until reconfigured")
			return
		}
		result.installClient(client)
		log.Info().
			Str("kodit_git_url", cfg.Kodit.GitURL).
			Msg("Initialized Kodit code intelligence service (external embedding)")
	}()

	return result, nil
}

// Reinit rebuilds the kodit client from the current System Settings. Called
// when an admin changes the kodit embedding provider/model so the new choice
// takes effect without a process restart. kodit itself detects any embedding
// dimension change and drops + rebuilds the vector tables, re-queuing each
// registered repository for indexing.
//
// The swap is ordered to keep things safe:
//  1. Park the public service on a disabled service so new queries get a
//     clean "unavailable" response while the swap is in flight.
//  2. Close the old kodit client (drains workers, closes DB) so old writes
//     can't race with the new client's table rebuild.
//  3. Build new opts from current config + settings.
//  4. Call kodit.New — if embedding dim changed, this drops and recreates
//     the vector tables and enqueues a sync for every registered repo.
//  5. Install the new client into the deferred service and MCP backend.
func (k *KoditResult) Reinit(ctx context.Context) error {
	if k == nil || k.deferred == nil {
		return fmt.Errorf("kodit reinit: not initialised")
	}
	k.mu.Lock()
	defer k.mu.Unlock()

	log.Info().Msg("kodit reinit requested")

	// Park service on disabled for the duration of the swap.
	k.deferred.Set(services.NewDisabledKoditService())

	// Close old client to drain workers and free DB connections before the
	// new client probes dimensions (which may drop/recreate vector tables).
	if k.client != nil {
		if err := k.client.Close(); err != nil {
			log.Warn().Err(err).Msg("error closing old kodit client during reinit (continuing)")
		}
		k.client = nil
		k.closer = nil
	}

	opts, _, decision, err := buildKoditOpts(k.cfg, k.store)
	if err != nil {
		return fmt.Errorf("kodit reinit: build opts: %w", err)
	}
	if err := preflightORT(decision); err != nil {
		return fmt.Errorf("kodit reinit: preflight: %w", err)
	}

	// Use retry only when an external embedding endpoint is involved. For a
	// pure-local reconfiguration there's no listener dependency to wait on.
	var newClient *kodit.Client
	if decision.UseExternalText || decision.UseExternalVision {
		newClient, err = newKoditClientWithRetry(opts)
	} else {
		newClient, err = kodit.New(opts...)
	}
	if err != nil {
		return fmt.Errorf("kodit reinit: new client: %w", err)
	}

	k.installClient(newClient)
	log.Info().Msg("kodit reinit complete; repositories will be re-indexed in background")
	return nil
}

// installClient atomically promotes the given kodit client to the active one.
// Safe to call either under mu (from Reinit) or once from the async init
// goroutine; callers coordinate ownership themselves.
func (k *KoditResult) installClient(client *kodit.Client) {
	k.client = client
	k.closer = client
	k.deferred.Set(services.NewKoditService(client))
	k.mcpBackend.setClient(client)
}

// buildKoditOpts reads the current System Settings and returns the kodit
// options needed to start a client, along with the model directory (so the
// caller can run a pre-flight ORT check) and the decision about which
// embedding providers are external vs built-in.
func buildKoditOpts(cfg *config.ServerConfig, s store.Store) ([]kodit.Option, string, koditEmbeddingDecision, error) {
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
		return nil, "", koditEmbeddingDecision{}, fmt.Errorf("load system settings for kodit: %w", err)
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

	return koditOpts, modelDir, decision, nil
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
