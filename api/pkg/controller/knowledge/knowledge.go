package knowledge

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	gocron "github.com/go-co-op/gocron/v2"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/controller/knowledge/crawler"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type Manager interface {
	NextRun(ctx context.Context, knowledgeID string) (time.Time, error)
}

type Reconciler struct {
	config       *config.ServerConfig
	store        store.Store
	filestore    filestore.FileStore
	extractor    extract.Extractor // Unstructured.io or equivalent
	httpClient   *http.Client
	ragClient    rag.RAG                                   // Default server RAG client
	newRagClient func(settings *types.RAGSettings) rag.RAG // Custom RAG server client constructor
	newCrawler   func(k *types.Knowledge) (crawler.Crawler, error)
	cron         gocron.Scheduler
	wg           sync.WaitGroup
}

func New(config *config.ServerConfig, store store.Store, filestore filestore.FileStore, extractor extract.Extractor, ragClient rag.RAG, b *browser.Browser) (*Reconciler, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	return &Reconciler{
		config:     config,
		store:      store,
		filestore:  filestore,
		cron:       s,
		extractor:  extractor,
		httpClient: http.DefaultClient,
		ragClient:  ragClient,
		newRagClient: func(settings *types.RAGSettings) rag.RAG {
			return rag.NewLlamaindex(settings)
		},
		newCrawler: func(k *types.Knowledge) (crawler.Crawler, error) {
			return crawler.NewCrawler(b, k)
		},
	}, nil
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
				log.Warn().Err(err).Msg("failed to index knowledge")
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

func (r *Reconciler) reset(ctx context.Context) error {
	data, err := r.store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		State: types.KnowledgeStateIndexing,
	})
	if err != nil {
		return fmt.Errorf("failed to get knowledge entries, error: %w", err)
	}

	for _, k := range data {
		k.State = types.KnowledgeStatePending

		_, err = r.store.UpdateKnowledge(ctx, k)
		if err != nil {
			log.Error().Err(err).Msg("failed to reset knowledge back into pending during reset")
		}
	}

	return nil
}
