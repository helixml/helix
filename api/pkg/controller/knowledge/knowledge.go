package knowledge

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	gocron "github.com/go-co-op/gocron/v2"
	"github.com/helixml/kodit/domain/snippet"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/controller/knowledge/crawler"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type Manager interface {
	NextRun(ctx context.Context, knowledgeID string) (time.Time, error)
	GetStatus(knowledgeID string) types.KnowledgeProgress // Ephemeral progress status for the knowledge
}

var _ Manager = &Reconciler{}

type Reconciler struct {
	config       *config.ServerConfig
	store        store.Store
	filestore    filestore.FileStore
	extractor    extract.Extractor // Unstructured.io or equivalent
	httpClient   *http.Client
	ragClient    rag.RAG                                   // Default server RAG client
	newRagClient func(settings *types.RAGSettings) rag.RAG // Custom RAG server client constructor
	newCrawler   func(k *types.Knowledge) (crawler.Crawler, error)
	oauthManager *oauth.Manager              // OAuth manager for SharePoint and other OAuth-based sources
	koditService services.KoditServicer      // Optional: set when Kodit is the RAG provider
	progressMu   *sync.RWMutex
	progress     map[string]types.KnowledgeProgress
	cron         gocron.Scheduler
	wg           sync.WaitGroup
}

func New(config *config.ServerConfig, store store.Store, filestore filestore.FileStore, extractor extract.Extractor, ragClient rag.RAG, b *browser.Browser, oauthManager *oauth.Manager, koditService ...services.KoditServicer) (*Reconciler, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	// Create HTTP client with optional TLS skip verify for enterprise environments
	httpClient := &http.Client{}
	if config.Tools.TLSSkipVerify {
		// Clone the default transport to preserve all default settings
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		httpClient.Transport = transport
	}

	r := &Reconciler{
		config:       config,
		store:        store,
		filestore:    filestore,
		cron:         s,
		extractor:    extractor,
		httpClient:   httpClient,
		ragClient:    ragClient,
		oauthManager: oauthManager,
		newRagClient: func(settings *types.RAGSettings) rag.RAG {
			// this is somewhat confusingly named, but it's only used for custom RAG
			// servers (not our own llamaindex)
			return rag.NewLlamaindex(settings)
		},
		// newCrawler: ,
		progressMu: &sync.RWMutex{},
		progress:   make(map[string]types.KnowledgeProgress),
	}

	r.newCrawler = func(k *types.Knowledge) (crawler.Crawler, error) {
		// Provide an ability for the crawler to update the progress
		updateProgress := func(progress types.KnowledgeProgress) {
			r.updateKnowledgeProgress(k.ID, progress)
		}

		// Construct the crawler
		return crawler.NewCrawler(b, k, updateProgress)
	}

	if len(koditService) > 0 && koditService[0] != nil {
		r.koditService = koditService[0]
	}

	return r, nil
}

func (r *Reconciler) Start(ctx context.Context) error {
	err := r.reset(ctx)
	if err != nil {
		log.Error().
			Err(err).
			Msg("knowledge state reset failed")
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		r.runIndexer(ctx)
	}()

	wg.Add(1)
	go func() {
		if err := r.startCron(ctx); err != nil {
			log.Error().Err(err).Msg("failed to start reconciling cron")
		}
	}()

	wg.Add(1)
	go func() {
		r.runCronManager(ctx)
	}()

	if r.koditService != nil && r.koditService.IsEnabled() {
		wg.Add(1)
		go func() {
			r.runKoditStatusChecker(ctx)
		}()
	}

	wg.Wait()

	return nil
}

func (r *Reconciler) runIndexer(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
			err := r.index(ctx)
			if err != nil {
				// Check for specific error types and log them appropriately
				if strings.Contains(err.Error(), "embedding model error") {
					log.Error().
						Err(err).
						Str("error_type", "embedding_model_error").
						Msg("Failed to index knowledge due to embedding model availability issue. Check that the requested embedding model is loaded in the vLLM service.")
				} else {
					log.Warn().Err(err).Msg("Failed to index knowledge")
				}
			}
		}
	}
}

// runCronManager is responsible for reconciling the cron jobs in the database
// with the actual cron jobs that are running.
func (r *Reconciler) runCronManager(ctx context.Context) {
	err := r.reconcileCronJobs(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to reconcile cron jobs")
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			err := r.reconcileCronJobs(ctx)
			if err != nil {
				log.Warn().Err(err).Msg("failed to reconcile cron jobs")
			}
		}
	}
}

// runKoditStatusChecker periodically checks Kodit for the real indexing status
// of knowledge sources that are in "indexing" state and updates them to "ready"
// or "error" when Kodit finishes.
func (r *Reconciler) runKoditStatusChecker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			if err := r.checkKoditStatuses(ctx); err != nil {
				log.Warn().Err(err).Msg("failed to check kodit indexing statuses")
			}
		}
	}
}

func (r *Reconciler) checkKoditStatuses(ctx context.Context) error {
	data, err := r.store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		State: types.KnowledgeStateIndexing,
	})
	if err != nil {
		return fmt.Errorf("failed to list indexing knowledge: %w", err)
	}

	for _, k := range data {
		// Only check filestore-backed knowledge (Kodit indexes local directories)
		if k.Source.Filestore == nil {
			continue
		}

		dataEntityID := k.GetDataEntityID()
		entity, err := r.store.GetDataEntity(ctx, dataEntityID)
		if err != nil {
			log.Warn().Err(err).Str("knowledge_id", k.ID).Msg("failed to get data entity for kodit status check")
			continue
		}
		if entity.KoditRepositoryID == nil {
			continue
		}

		summary, err := r.koditService.GetRepositoryStatus(ctx, *entity.KoditRepositoryID)
		if err != nil {
			log.Warn().Err(err).Str("knowledge_id", k.ID).Int64("kodit_repo_id", *entity.KoditRepositoryID).Msg("failed to get kodit status")
			continue
		}

		koditStatus := summary.Status()
		switch koditStatus {
		case snippet.IndexStatusCompleted:
			k.State = types.KnowledgeStateReady
			k.Message = ""
			if _, err := r.store.UpdateKnowledge(ctx, k); err != nil {
				log.Error().Err(err).Str("knowledge_id", k.ID).Msg("failed to update knowledge to ready after kodit completed")
			} else {
				log.Info().Str("knowledge_id", k.ID).Msg("kodit indexing completed, knowledge is ready")
				if err := r.deleteOldVersions(ctx, k); err != nil {
					log.Warn().Err(err).Str("knowledge_id", k.ID).Msg("failed to delete old versions after kodit completed")
				}
			}

		case snippet.IndexStatusCompletedWithErrors:
			k.State = types.KnowledgeStateReady
			k.Message = summary.Message()
			if _, err := r.store.UpdateKnowledge(ctx, k); err != nil {
				log.Error().Err(err).Str("knowledge_id", k.ID).Msg("failed to update knowledge after kodit completed with errors")
			} else {
				log.Warn().Str("knowledge_id", k.ID).Str("message", k.Message).Msg("kodit indexing completed with errors")
				if err := r.deleteOldVersions(ctx, k); err != nil {
					log.Warn().Err(err).Str("knowledge_id", k.ID).Msg("failed to delete old versions after kodit completed with errors")
				}
			}

		case snippet.IndexStatusFailed:
			k.State = types.KnowledgeStateError
			k.Message = summary.Message()
			if _, err := r.store.UpdateKnowledge(ctx, k); err != nil {
				log.Error().Err(err).Str("knowledge_id", k.ID).Msg("failed to update knowledge to error after kodit failed")
			} else {
				log.Error().Str("knowledge_id", k.ID).Str("message", k.Message).Msg("kodit indexing failed")
			}

		default:
			// Still in progress (pending, in_progress) — do nothing
		}
	}

	return nil
}

func (r *Reconciler) reset(ctx context.Context) error {
	data, err := r.store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		State: types.KnowledgeStateIndexing,
	})
	if err != nil {
		return fmt.Errorf("failed to get knowledge entries, error: %w", err)
	}

	// When the RAG provider is Kodit, knowledge in "indexing" state should not
	// be reset to "pending" — Kodit manages its own indexing state and the
	// status checker loop will transition them to "ready" when Kodit finishes.
	isKoditRAG := r.koditService != nil && r.koditService.IsEnabled()

	for _, k := range data {
		if isKoditRAG && k.Source.Filestore != nil {
			// Kodit knowledge: leave in "indexing" state; the status checker will handle it.
			log.Info().Str("knowledge_id", k.ID).Msg("skipping reset for kodit-indexed knowledge")
			continue
		}

		// Note: We don't reset knowledge sources in "Preparing" state
		// as they are waiting for explicit user action
		k.State = types.KnowledgeStatePending

		_, err = r.store.UpdateKnowledge(ctx, k)
		if err != nil {
			log.Error().Err(err).Msg("failed to reset knowledge back into pending during reset")
		}
	}

	return nil
}

func (r *Reconciler) GetStatus(knowledgeID string) types.KnowledgeProgress {
	r.progressMu.RLock()
	defer r.progressMu.RUnlock()

	if progress, ok := r.progress[knowledgeID]; ok {
		return progress
	}

	// No progress yet or already finished
	return types.KnowledgeProgress{
		Step:           "",
		Progress:       0,
		Message:        "",
		ElapsedSeconds: 0,
	}
}

func (r *Reconciler) updateKnowledgeProgress(knowledgeID string, progress types.KnowledgeProgress) {
	r.progressMu.Lock()
	defer r.progressMu.Unlock()

	r.progress[knowledgeID] = progress
}

func (r *Reconciler) resetKnowledgeProgress(knowledgeID string) {
	r.progressMu.Lock()
	defer r.progressMu.Unlock()

	delete(r.progress, knowledgeID)
}
