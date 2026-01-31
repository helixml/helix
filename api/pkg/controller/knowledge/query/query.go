package query

import (
	"context"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	helix_langchain "github.com/helixml/helix/api/pkg/openai/langchain"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/helixml/helix/api/pkg/openai"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/schema"
)

var model = "openai/gpt-oss-20b"

type Query struct {
	store        store.Store
	apiClient    openai.Client
	getRAGClient func(ctx context.Context, knowledge *types.Knowledge) (rag.RAG, error)
	model        string
}

type Config struct {
	store        store.Store
	apiClient    openai.Client
	getRAGClient func(ctx context.Context, knowledge *types.Knowledge) (rag.RAG, error)
	model        string
}

func New(cfg *Config) *Query {
	return &Query{
		store:        cfg.store,
		apiClient:    cfg.apiClient,
		getRAGClient: cfg.getRAGClient,
		model:        cfg.model,
	}
}

func (q *Query) Answer(ctx context.Context, prompt, appID string, assistant *types.AssistantConfig) (string, error) {
	llm, err := helix_langchain.New(q.apiClient, q.model)
	if err != nil {
		return "", fmt.Errorf("error creating LLM client: %w", err)
	}

	knowledge, err := q.listKnowledge(ctx, appID, assistant)
	if err != nil {
		return "", fmt.Errorf("error listing knowledge: %w", err)
	}

	spew.Dump(knowledge)

	docs, err := q.getDocuments(ctx, prompt, knowledge)
	if err != nil {
		return "", fmt.Errorf("error getting documents: %w", err)
	}

	spew.Dump(docs)

	stuffQAChain := chains.LoadStuffQA(llm)

	answer, err := chains.Call(context.Background(), stuffQAChain, map[string]any{
		"input_documents": docs,
		"question":        prompt,
	})
	if err != nil {
		return "", fmt.Errorf("error calling QA chain: %w", err)
	}

	spew.Dump(answer)

	intf, ok := answer["text"]
	if !ok {
		return "", fmt.Errorf("no answer found")
	}

	answerStr, ok := intf.(string)
	if !ok {
		return "", fmt.Errorf("answer is not a string")
	}

	return answerStr, nil
}

// getDocuments retrieves data from the database and converts it into a slice of schema.Document
func (q *Query) getDocuments(ctx context.Context, prompt string, knowledges []*types.Knowledge) ([]schema.Document, error) {
	var (
		documents []schema.Document
	)

	for _, knowledge := range knowledges {
		switch {
		// If the knowledge is a content, add it to the background knowledge
		// without anything else (no database to search in)
		case knowledge.Source.Text != nil:
			documents = append(documents, schema.Document{
				PageContent: *knowledge.Source.Text,
			})

		default:
			ragClient, err := q.getRAGClient(ctx, knowledge)
			if err != nil {
				return nil, fmt.Errorf("error getting RAG client: %w", err)
			}

			pipeline := types.TextPipeline
			if knowledge.RAGSettings.EnableVision {
				pipeline = types.VisionPipeline
			}
			ragResults, err := ragClient.Query(ctx, &types.SessionRAGQuery{
				Prompt:            prompt,
				DataEntityID:      knowledge.GetDataEntityID(),
				DistanceThreshold: knowledge.RAGSettings.Threshold,
				DistanceFunction:  knowledge.RAGSettings.DistanceFunction,
				MaxResults:        knowledge.RAGSettings.ResultsCount,
				Pipeline:          pipeline,
			})
			if err != nil {
				return nil, fmt.Errorf("error querying RAG: %w", err)
			}

			for _, result := range ragResults {
				documents = append(documents, schema.Document{
					PageContent: result.Content,
				})
			}
		}
	}

	return documents, nil
}

// listKnowledge retrieves all knowledge for an assistant to query documents for
func (q *Query) listKnowledge(ctx context.Context, appID string, assistant *types.AssistantConfig) ([]*types.Knowledge, error) {
	var knowledge []*types.Knowledge
	for _, k := range assistant.Knowledge {

		k, err := q.store.LookupKnowledge(ctx, &store.LookupKnowledgeQuery{
			Name:  k.Name,
			AppID: appID,
		})
		if err != nil {
			return nil, fmt.Errorf("error getting knowledge: %w", err)
		}

		// Skip knowledge that is not ready
		if k.State != types.KnowledgeStateReady {
			continue
		}

		knowledge = append(knowledge, k)
	}

	return knowledge, nil
}
