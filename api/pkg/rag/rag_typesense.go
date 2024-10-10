package rag

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/avast/retry-go/v4"
	"github.com/rs/zerolog/log"
	"github.com/typesense/typesense-go/v2/typesense"
	"github.com/typesense/typesense-go/v2/typesense/api"
	"github.com/typesense/typesense-go/v2/typesense/api/pointer"
)

const (
	defaultCollection = "helix-documents"
	defaultModelName  = "ts/all-MiniLM-L12-v2"
)

type Typesense struct {
	client     *typesense.Client
	collection string
	ready      chan struct{}
}

var _ RAG = &Typesense{}

func NewTypesense(settings *types.RAGSettings) (*Typesense, error) {
	client := typesense.NewClient(
		typesense.WithServer(settings.Typesense.URL),
		typesense.WithAPIKey(settings.Typesense.APIKey),
		typesense.WithNumRetries(3),
		typesense.WithConnectionTimeout(300*time.Second),
	)

	collection := settings.Typesense.Collection
	if collection == "" {
		collection = defaultCollection
	}

	t := &Typesense{
		client:     client,
		collection: collection,
		ready:      make(chan struct{}),
	}

	go t.waitForTypesense()

	return t, nil
}

func (t *Typesense) waitForTypesense() {
	err := retry.Do(func() error {
		healthy, err := t.client.Health(context.Background(), 5*time.Second)
		if err != nil {
			return err
		}

		if !healthy {
			return fmt.Errorf("typesense is not healthy yet")
		}

		return t.ensureCollection(context.Background())
	},
		retry.Attempts(0),
		retry.Delay(2*time.Second),
		retry.MaxDelay(10*time.Second),
		retry.LastErrorOnly(true),
		retry.MaxDelay(10*time.Second),
		retry.OnRetry(func(n uint, err error) {
			log.Warn().
				Err(err).
				Uint("retries", n).
				Msg("waiting for typesense to come up")
		}),
	)

	if err != nil {
		log.Error().Err(err).Msg("failed to connect to typesense")
		return
	}

	log.Info().Msg("typesense is up and collection is ready")
	close(t.ready)
}

func (t *Typesense) ensureReady(ctx context.Context) error {
	select {
	case <-t.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *Typesense) Index(ctx context.Context, indexReqs ...*types.SessionRAGIndexChunk) error {
	if err := t.ensureReady(ctx); err != nil {
		return err
	}
	return t.index(ctx, indexReqs...)
}

func (t *Typesense) index(ctx context.Context, indexReqs ...*types.SessionRAGIndexChunk) error {
	if len(indexReqs) == 0 {
		return fmt.Errorf("no index requests provided")
	}

	if len(indexReqs) == 1 {
		_, err := t.client.Collection(t.collection).Documents().Create(ctx, indexReqs[0])
		return err
	}

	// For multiple index requests, we need to use the import API

	params := &api.ImportDocumentsParams{
		Action:    pointer.String("create"),
		BatchSize: pointer.Int(len(indexReqs)),
	}

	var docs []interface{}
	for _, indexReq := range indexReqs {
		docs = append(docs, indexReq)
	}

	_, err := t.client.Collection(t.collection).Documents().Import(ctx, docs, params)
	if err != nil {
		return fmt.Errorf("error importing documents: %w", err)
	}

	return nil
}

func (t *Typesense) Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error) {
	if err := t.ensureReady(ctx); err != nil {
		return nil, err
	}

	// TODO: implement hybrid search https://typesense.org/docs/26.0/api/vector-search.html#hybrid-search
	searchParameters := &api.SearchCollectionParams{
		Q:             pointer.String(q.Prompt),
		QueryBy:       pointer.String("embedding,content"),
		FilterBy:      pointer.String("data_entity_id:" + q.DataEntityID),
		SortBy:        pointer.String("_text_match:desc,_vector_distance:asc"),
		ExcludeFields: pointer.String("embedding"), // Don't return the raw floating point numbers in the vector field in the search API response, to save on network bandwidth.
	}

	results, err := t.client.Collection(t.collection).Documents().Search(ctx, searchParameters)
	if err != nil {
		return nil, err
	}

	log.Info().Int("num_results", len(*results.Hits)).Msg("typesense results")

	var ragResults []*types.SessionRAGResult
	for _, hit := range *results.Hits {

		ragResult := &types.SessionRAGResult{
			// DataEntityID:    hit.Document["data_entity_id"].(string),
			DocumentGroupID: getStrVariable(&hit, "document_group_id"),
			DocumentID:      getStrVariable(&hit, "document_id"),
			Source:          getStrVariable(&hit, "source"),
			Content:         getStrVariable(&hit, "content"),
			ContentOffset:   getIntVariable(&hit, "content_offset"),
		}
		ragResults = append(ragResults, ragResult)
	}

	return ragResults, nil
}

func (t *Typesense) Delete(ctx context.Context, r *types.DeleteIndexRequest) error {
	if err := t.ensureReady(ctx); err != nil {
		return err
	}

	params := &api.DeleteDocumentsParams{
		FilterBy: pointer.String("data_entity_id:" + r.DataEntityID),
	}
	_, err := t.client.Collection(t.collection).Documents().Delete(ctx, params)
	return err
}

func getStrVariable(hit *api.SearchResultHit, key string) string {
	val, ok := (*hit.Document)[key]
	if !ok {
		return ""
	}

	str, ok := val.(string)
	if !ok {
		return ""
	}

	return str
}

func getIntVariable(hit *api.SearchResultHit, key string) int {
	val, ok := (*hit.Document)[key]
	if !ok {
		return 0
	}

	return int(val.(float64))
}

func (t *Typesense) ensureCollection(ctx context.Context) error {
	log.Info().Str("collection", t.collection).Msg("ensuring collection")

	// Check if collection exists
	collections, err := t.client.Collections().Retrieve(ctx)
	if err != nil {
		return err
	}

	for _, collection := range collections {
		if collection.Name == t.collection {
			log.Info().Str("collection", t.collection).Msg("collection already exists")
			return nil
		}
	}

	schema := &api.CollectionSchema{
		Name: t.collection,
		Fields: []api.Field{
			{
				Name: "data_entity_id",
				Type: "string",
			},
			{
				Name: "document_group_id",
				Type: "string",
			},
			{
				Name:  "document_id",
				Type:  "string",
				Facet: pointer.True(),
			},
			{
				Name: "source",
				Type: "string",
			},
			{
				Name: "content",
				Type: "string",
			},
			{
				Name: "content_offset",
				Type: "int32",
			},
			{
				Name: "embedding",
				Type: "float[]",
				Embed: &struct {
					From        []string `json:"from"`
					ModelConfig struct {
						AccessToken  *string `json:"access_token,omitempty"`
						ApiKey       *string `json:"api_key,omitempty"`
						ClientId     *string `json:"client_id,omitempty"`
						ClientSecret *string `json:"client_secret,omitempty"`
						ModelName    string  `json:"model_name"`
						ProjectId    *string `json:"project_id,omitempty"`
					} `json:"model_config"`
				}{
					From: []string{"content"},
					ModelConfig: struct {
						AccessToken  *string `json:"access_token,omitempty"`
						ApiKey       *string `json:"api_key,omitempty"`
						ClientId     *string `json:"client_id,omitempty"`
						ClientSecret *string `json:"client_secret,omitempty"`
						ModelName    string  `json:"model_name"`
						ProjectId    *string `json:"project_id,omitempty"`
					}{
						ModelName: defaultModelName,
					},
				},
			},
		},
		DefaultSortingField: pointer.String("content_offset"),
	}

	_, err = t.client.Collections().Create(ctx, schema)
	if err != nil {
		return err
	}

	return nil
}
