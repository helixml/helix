package rag

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/qdrant/go-client/qdrant"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
	"github.com/sourcegraph/conc/pool"
)

const (
	defaultQdrantCollection = "helix-documents"
	defaultQdrantLimit      = 10
	defaultQdrantDistance   = qdrant.Distance_Cosine

	qdrantFieldDataEntityID    = "data_entity_id"
	qdrantFieldDocumentID      = "document_id"
	qdrantFieldDocumentGroupID = "document_group_id"
	qdrantFieldContent         = "content"
	qdrantFieldContentOffset   = "content_offset"
	qdrantFieldSource          = "source"
)

type Qdrant struct {
	cfg             *config.ServerConfig
	providerManager manager.ProviderManager
	client          *qdrant.Client
	collection      string
	ready           chan struct{}
}

var _ RAG = &Qdrant{}

func NewQdrant(cfg *config.ServerConfig, providerManager manager.ProviderManager) (*Qdrant, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host:                   cfg.RAG.Qdrant.Host,
		Port:                   cfg.RAG.Qdrant.Port,
		APIKey:                 cfg.RAG.Qdrant.APIKey,
		UseTLS:                 cfg.RAG.Qdrant.UseTLS,
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}

	collection := cfg.RAG.Qdrant.Collection
	if collection == "" {
		collection = defaultQdrantCollection
	}

	q := &Qdrant{
		cfg:             cfg,
		providerManager: providerManager,
		client:          client,
		collection:      collection,
		ready:           make(chan struct{}),
	}

	go q.ensureCollectionReady()

	return q, nil
}

func (q *Qdrant) ensureCollectionReady() {
	ctx := context.Background()

	dimensions, err := q.getDimensions(q.cfg.RAG.Qdrant.EmbeddingsModel)
	if err != nil {
		log.Error().Err(err).Msg("failed to get dimensions for Qdrant collection")
		return
	}

	collectionExists, err := q.client.CollectionExists(ctx, q.collection)
	if err != nil {
		log.Error().Err(err).Msg("failed to check if Qdrant collection exists")
		return
	}

	if !collectionExists {
		log.Info().
			Str("collection", q.collection).
			Int("dimensions", int(dimensions)).
			Msg("creating Qdrant collection")

		err = q.client.CreateCollection(ctx, &qdrant.CreateCollection{
			CollectionName: q.collection,
			VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
				Size:     uint64(dimensions),
				Distance: defaultQdrantDistance,
			}),
		})
		if err != nil {
			log.Error().Err(err).Msg("failed to create Qdrant collection")
			return
		}

		for _, field := range []string{qdrantFieldDataEntityID, qdrantFieldDocumentID} {
			_, err = q.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
				CollectionName: q.collection,
				FieldName:      field,
				FieldType:      qdrant.FieldType_FieldTypeKeyword.Enum(),
			})
			if err != nil {
				log.Warn().Err(err).Msgf("failed to create %s index", field)
			}
		}
	}

	log.Info().Str("collection", q.collection).Msg("Qdrant collection is ready")
	close(q.ready)
}

func (q *Qdrant) ensureReady(ctx context.Context) error {
	select {
	case <-q.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *Qdrant) Index(ctx context.Context, indexReqs ...*types.SessionRAGIndexChunk) error {
	if err := q.ensureReady(ctx); err != nil {
		return err
	}

	if len(indexReqs) == 0 {
		return nil
	}

	points, err := q.createPoints(ctx, indexReqs)
	if err != nil {
		return err
	}

	if len(points) == 0 {
		log.Warn().Msg("no points to insert into Qdrant (all embeddings failed or returned empty)")
		return nil
	}

	start := time.Now()

	log.Info().
		Int("points", len(points)).
		Msg("inserting points into Qdrant")

	_, err = q.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: q.collection,
		Points:         points,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert points: %w", err)
	}

	log.Info().
		Int("duration_ms", int(time.Since(start).Milliseconds())).
		Int("points", len(points)).
		Msg("points inserted into Qdrant")

	return nil
}

func (q *Qdrant) createPoints(ctx context.Context, indexReqs []*types.SessionRAGIndexChunk) ([]*qdrant.PointStruct, error) {
	var points []*qdrant.PointStruct

	client, err := q.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: q.cfg.RAG.Qdrant.Provider,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding client: %w", err)
	}

	p := pool.New().
		WithMaxGoroutines(q.cfg.RAG.Qdrant.EmbeddingsConcurrency).
		WithErrors()

	mu := sync.Mutex{}

	for _, indexReq := range indexReqs {
		indexReq := indexReq
		p.Go(func() error {
			start := time.Now()
			generated, err := client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
				Model: openai.EmbeddingModel(q.cfg.RAG.Qdrant.EmbeddingsModel),
				Input: indexReq.Content,
			})
			if err != nil {
				log.Error().
					Err(err).
					Str("model", q.cfg.RAG.Qdrant.EmbeddingsModel).
					Int("content_length", len(indexReq.Content)).
					Str("knowledge_id", indexReq.DataEntityID).
					Msg("failed to create embeddings")
				return err
			}

			if len(generated.Data) == 0 {
				log.Error().
					Str("knowledge_id", indexReq.DataEntityID).
					Msg("no embeddings returned for indexReq")
				return nil
			}

			log.Info().
				Str("knowledge_id", indexReq.DataEntityID).
				Str("model", q.cfg.RAG.Qdrant.EmbeddingsModel).
				Int("content_length", len(indexReq.Content)).
				Int("duration_ms", int(time.Since(start).Milliseconds())).
				Msg("created embeddings")

			point := &qdrant.PointStruct{
				Id:      qdrant.NewIDUUID(uuid.New().String()),
				Vectors: qdrant.NewVectors(generated.Data[0].Embedding...),
				Payload: qdrant.NewValueMap(map[string]any{
					qdrantFieldDataEntityID:    indexReq.DataEntityID,
					qdrantFieldDocumentID:      indexReq.DocumentID,
					qdrantFieldDocumentGroupID: indexReq.DocumentGroupID,
					qdrantFieldContent:         indexReq.Content,
					qdrantFieldContentOffset:   indexReq.ContentOffset,
					qdrantFieldSource:          indexReq.Source,
				}),
			}

			mu.Lock()
			points = append(points, point)
			mu.Unlock()

			return nil
		})
	}

	err = p.Wait()
	if err != nil {
		return nil, err
	}

	return points, nil
}

func (q *Qdrant) Query(ctx context.Context, query *types.SessionRAGQuery) ([]*types.SessionRAGResult, error) {
	if err := q.ensureReady(ctx); err != nil {
		return nil, err
	}

	client, err := q.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: q.cfg.RAG.Qdrant.Provider,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding client: %w", err)
	}

	generated, err := client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(q.cfg.RAG.Qdrant.EmbeddingsModel),
		Input: query.Prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create query embeddings: %w", err)
	}

	if len(generated.Data) == 0 {
		return nil, fmt.Errorf("no embeddings returned for query")
	}

	vector := make([]float32, len(generated.Data[0].Embedding))
	for i, v := range generated.Data[0].Embedding {
		vector[i] = float32(v)
	}

	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch(qdrantFieldDataEntityID, query.DataEntityID),
		},
	}

	if len(query.DocumentIDList) > 0 {
		filter.Must = append(filter.Must, qdrant.NewMatchKeywords(qdrantFieldDocumentID, query.DocumentIDList...))
	}

	limit := uint64(defaultQdrantLimit)
	if query.MaxResults > 0 {
		limit = uint64(query.MaxResults)
	}

	searchResult, err := q.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: q.collection,
		Query:          qdrant.NewQuery(vector...),
		Filter:         filter,
		Limit:          &limit,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query Qdrant: %w", err)
	}

	log.Info().Int("num_results", len(searchResult)).Msg("Qdrant results")

	results := make([]*types.SessionRAGResult, 0, len(searchResult))
	for _, hit := range searchResult {
		result := &types.SessionRAGResult{
			DocumentGroupID: getPayloadString(hit.Payload, qdrantFieldDocumentGroupID),
			DocumentID:      getPayloadString(hit.Payload, qdrantFieldDocumentID),
			Source:          getPayloadString(hit.Payload, qdrantFieldSource),
			Content:         getPayloadString(hit.Payload, qdrantFieldContent),
			ContentOffset:   int(getPayloadInt(hit.Payload, qdrantFieldContentOffset)),
			Distance:        float64(1 - float32(hit.Score)),
		}
		results = append(results, result)
	}

	return results, nil
}

func (q *Qdrant) Delete(ctx context.Context, req *types.DeleteIndexRequest) error {
	if err := q.ensureReady(ctx); err != nil {
		return err
	}

	_, err := q.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: q.collection,
		Points: qdrant.NewPointsSelectorFilter(&qdrant.Filter{
			Must: []*qdrant.Condition{
				qdrant.NewMatch(qdrantFieldDataEntityID, req.DataEntityID),
			},
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to delete points: %w", err)
	}

	return nil
}

func (q *Qdrant) getDimensions(model string) (types.Dimensions, error) {
	if q.cfg.RAG.Qdrant.Dimensions != 0 {
		return q.cfg.RAG.Qdrant.Dimensions, nil
	}

	return getDimensions(model)
}

func getPayloadString(payload map[string]*qdrant.Value, key string) string {
	val, ok := payload[key]
	if !ok || val == nil {
		return ""
	}

	strVal := val.GetStringValue()
	return strVal
}

func getPayloadInt(payload map[string]*qdrant.Value, key string) int64 {
	val, ok := payload[key]
	if !ok || val == nil {
		return 0
	}

	return val.GetIntegerValue()
}
