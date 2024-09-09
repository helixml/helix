package rag

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"

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
}

var _ RAG = &Typesense{}

func NewTypesense(settings *types.RAGSettings) (*Typesense, error) {
	client := typesense.NewClient(
		typesense.WithServer(settings.Typesense.URL),
		typesense.WithAPIKey(settings.Typesense.APIKey),
		typesense.WithNumRetries(3),
	)

	collection := settings.Typesense.Collection
	if collection == "" {
		collection = defaultCollection
	}

	t := &Typesense{
		client:     client,
		collection: collection,
	}

	err := t.ensureCollection(context.Background())
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (t *Typesense) Index(ctx context.Context, indexReq *types.SessionRAGIndexChunk) error {
	_, err := t.client.Collection(t.collection).Documents().Create(ctx, indexReq)
	if err != nil {
		return err
	}

	return nil
}

func (t *Typesense) Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error) {
	// TODO: implement hybrid search https://typesense.org/docs/26.0/api/vector-search.html#hybrid-search
	searchParameters := &api.SearchCollectionParams{
		Q:             pointer.String(q.Prompt),
		QueryBy:       pointer.String("content"),
		FilterBy:      pointer.String("data_entity_id:" + q.DataEntityID),
		SortBy:        pointer.String("content_offset:asc"),
		ExcludeFields: pointer.String("embedding"), // Don't return the raw floating point numbers in the vector field in the search API response, to save on network bandwidth.
	}

	results, err := t.client.Collection(t.collection).Documents().Search(ctx, searchParameters)
	if err != nil {
		return nil, err
	}

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
	params := &api.DeleteDocumentsParams{
		FilterBy: pointer.String("data_entity_id:" + r.DataEntityID),
	}
	_, err := t.client.Collection(t.collection).Documents().Delete(ctx, params)
	if err != nil {
		return err
	}

	return nil
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

	_, err := t.client.Collections().Create(ctx, schema)
	if err != nil {
		return err
	}

	return nil
}
