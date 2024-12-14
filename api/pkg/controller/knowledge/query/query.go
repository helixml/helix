package query

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/helixml/helix/api/pkg/openai"
	helix_langchain "github.com/helixml/helix/api/pkg/openai/langchain"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/davecgh/go-spew/spew"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/schema"
)

// var model = "meta-llama/Llama-3-8b-chat-hf"
// var model = "Qwen/Qwen2.5-7B-Instruct-Turbo" // ok
// var model = "meta-llama/Llama-3.3-70B-Instruct-Turbo-Free" // ok
// var model = "meta-llama/Llama-3.2-11B-Vision-Instruct-Turbo" // ok
var model = "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo" // ok

type Query struct {
	store        store.Store
	apiClient    openai.Client
	getRAGClient func(ctx context.Context, knowledge *types.Knowledge) (rag.RAG, error)
	model        string
}

type QueryConfig struct {
	Store        store.Store
	APIClient    openai.Client
	GetRAGClient func(ctx context.Context, knowledge *types.Knowledge) (rag.RAG, error)
	Model        string
}

func New(cfg *QueryConfig) *Query {
	return &Query{
		store:        cfg.Store,
		apiClient:    cfg.APIClient,
		getRAGClient: cfg.GetRAGClient,
		model:        cfg.Model,
	}
}

// QueryAndResearch returns a list of answers
func (q *Query) QueryAndResearch(ctx context.Context, prompt, appID string, assistant *types.AssistantConfig) ([]string, error) {

	llm, err := helix_langchain.New(q.apiClient, assistant.Model)
	if err != nil {
		return nil, fmt.Errorf("error creating LLM client: %w", err)
	}

	knowledgeList, err := q.listKnowledge(ctx, appID, assistant)
	if err != nil {
		return nil, fmt.Errorf("error listing knowledge: %w", err)
	}

	log.Info().Msg("generating variations")

	variations, err := q.createVariations(ctx, prompt, 8)
	if err != nil {
		return nil, fmt.Errorf("error creating variations: %w", err)
	}

	log.Info().Msgf("researching %d variations", len(variations))

	pool := pool.New().
		WithMaxGoroutines(len(variations)).
		WithErrors()

	var results []string
	var resultsMu sync.Mutex

	for _, variation := range variations {
		variation := variation

		pool.Go(func() error {
			answer, err := q.research(ctx, llm, variation, knowledgeList)
			if err != nil {
				log.
					Err(err).
					Str("variation", variation).
					Msg("error researching")
				return err
			}

			resultsMu.Lock()
			results = append(results, answer)
			resultsMu.Unlock()

			return nil
		})
	}

	err = pool.Wait()
	if err != nil {
		log.Warn().Err(err).Msg("error while researching variations")
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no results found")
	}

	return results, nil
}

func (q *Query) Answer(ctx context.Context, prompt, appID string, assistant *types.AssistantConfig, opts ...chains.ChainCallOption) (string, error) {
	llm, err := helix_langchain.New(q.apiClient, q.model)
	if err != nil {
		return "", fmt.Errorf("error creating LLM client: %w", err)
	}

	knowledgeList, err := q.listKnowledge(ctx, appID, assistant)
	if err != nil {
		return "", fmt.Errorf("error listing knowledge: %w", err)
	}

	log.Info().Msg("generating variations")

	variations, err := q.createVariations(ctx, prompt, 8)
	if err != nil {
		return "", fmt.Errorf("error creating variations: %w", err)
	}

	log.Info().Msgf("researching %d variations", len(variations))

	pool := pool.New().
		WithMaxGoroutines(len(variations)).
		WithErrors()

	var results []string
	var resultsMu sync.Mutex

	for _, variation := range variations {
		variation := variation

		pool.Go(func() error {
			answer, err := q.research(ctx, llm, variation, knowledgeList)
			if err != nil {
				log.
					Err(err).
					Str("variation", variation).
					Msg("error researching")
				return err
			}

			resultsMu.Lock()
			results = append(results, answer)
			resultsMu.Unlock()

			return nil
		})
	}

	err = pool.Wait()
	if err != nil {
		log.Warn().Err(err).Msg("error while researching variations")
	}

	if len(results) == 0 {
		return "", fmt.Errorf("no results found")
	}

	log.Info().Msg("combining results")

	results = append(results, `Minimum installation requirements: 4 CPUs, 8GB RAM and 50GB+ free disk space`)

	return q.combineResults(ctx, llm, prompt, results, opts...)
}

func (q *Query) research(ctx context.Context, llm *helix_langchain.LangchainAdapter, promptVariation string, knowledgeList []*types.Knowledge) (string, error) {
	docs, err := q.getDocuments(ctx, promptVariation, knowledgeList)
	if err != nil {
		return "", fmt.Errorf("error getting documents: %w", err)
	}

	log.Info().
		Str("variation", promptVariation).
		Int("documents", len(docs)).
		Msg("researching documents")

	stuffQAChain := chains.LoadStuffQA(llm)

	ctx = q.setContextAndStep(ctx, types.LLMCallStepResearchTopic)

	answer, err := chains.Call(ctx, stuffQAChain, map[string]any{
		"input_documents": docs,
		"question":        promptVariation,
	})
	if err != nil {
		return "", fmt.Errorf("error calling QA chain: %w", err)
	}

	intf, ok := answer["text"]
	if !ok {
		spew.Dump(answer)
		return "", fmt.Errorf("no answer found")
	}

	answerStr, ok := intf.(string)
	if !ok {
		spew.Dump(answer)
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
		case knowledge.Source.Content != nil:
			documents = append(documents, schema.Document{
				PageContent: *knowledge.Source.Content,
			})

		default:
			ragClient, err := q.getRAGClient(ctx, knowledge)
			if err != nil {
				return nil, fmt.Errorf("error getting RAG client: %w", err)
			}

			ragResults, err := ragClient.Query(ctx, &types.SessionRAGQuery{
				Prompt:            prompt,
				DataEntityID:      knowledge.GetDataEntityID(),
				DistanceThreshold: knowledge.RAGSettings.Threshold,
				DistanceFunction:  knowledge.RAGSettings.DistanceFunction,
				MaxResults:        knowledge.RAGSettings.ResultsCount,
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

func (q *Query) combineResults(ctx context.Context, llm *helix_langchain.LangchainAdapter, prompt string, results []string, opts ...chains.ChainCallOption) (string, error) {

	qaChain := chains.LoadStuffQA(llm)

	var docs []schema.Document
	for _, result := range results {
		if strings.Contains(result, "I don't know") {
			continue
		}
		docs = append(docs, schema.Document{
			PageContent: result,
		})
	}

	ctx = q.setContextAndStep(ctx, types.LLMCallStepCombineResults)

	req := map[string]any{
		"input_documents": docs,
		"question":        prompt,
	}

	spew.Dump(req)

	answer, err := chains.Call(ctx, qaChain, req, opts...)
	if err != nil {
		return "", fmt.Errorf("error calling QA chain: %w", err)
	}

	intf, ok := answer["text"]
	if !ok {
		spew.Dump(answer)
		return "", fmt.Errorf("no answer found")
	}

	answerStr, ok := intf.(string)
	if !ok {
		spew.Dump(answer)
		return "", fmt.Errorf("answer is not a string")
	}

	return answerStr, nil
}
