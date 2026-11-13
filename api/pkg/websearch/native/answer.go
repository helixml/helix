package native

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/websearch"
)

// answerNumResults is how many search results an answer is drawn from.
const answerNumResults = 8

func (p *Provider) Answer(ctx context.Context, req *websearch.AnswerRequest) (*websearch.AnswerResponse, error) {
	return p.answer(ctx, req, nil)
}

// StreamAnswer emits the answer as OpenAI-style chunks (an outputSchema
// answer as one chunk of JSON), then the citations.
func (p *Provider) StreamAnswer(ctx context.Context, req *websearch.AnswerRequest, emit func(websearch.AnswerEvent) error) error {
	role := "assistant"
	onDelta := func(d string) error {
		ev := websearch.AnswerEvent{Choices: []websearch.AnswerChoice{{Delta: websearch.AnswerDelta{Role: role, Content: d}}}}
		role = ""
		return emit(ev)
	}
	structured := len(req.OutputSchema) > 0
	stream := onDelta
	if structured {
		stream = nil // partial JSON is not streamed; the whole object is sent below
	}
	resp, err := p.answer(ctx, req, stream)
	if err != nil {
		return err
	}
	if structured {
		b, err := json.Marshal(resp.Answer)
		if err != nil {
			return fmt.Errorf("encode answer: %w", err)
		}
		if err := onDelta(string(b)); err != nil {
			return err
		}
	}
	return emit(websearch.AnswerEvent{Citations: resp.Citations})
}

// answer searches, then answers from the fetched results. onDelta receives
// a string answer as it is generated.
func (p *Provider) answer(ctx context.Context, req *websearch.AnswerRequest, onDelta func(string) error) (*websearch.AnswerResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if p.opts.Synthesizer == nil {
		return nil, websearch.FeatureDisabled("answer", "no LLM is configured")
	}
	tier, searchType := answerModel(req.Model)
	_, hits, err := p.runSearch(ctx, &websearch.SearchRequest{
		Query:          req.Query,
		Type:           searchType,
		NumResults:     answerNumResults,
		UserLocation:   req.UserLocation,
		UserIdentifier: req.UserIdentifier,
		Contents:       &websearch.ContentsOptions{Text: &websearch.TextOption{Enabled: true}},
	})
	if err != nil {
		return nil, err
	}
	srcs, err := sources(hits)
	if err != nil {
		return nil, err
	}
	hits = slices.DeleteFunc(hits, func(h hit) bool { return h.doc == nil })
	answer, err := p.opts.Synthesizer.Answer(ctx, &AnswerSynthesis{
		Tier:         tier,
		Query:        req.Query,
		SystemPrompt: req.SystemPrompt,
		OutputSchema: req.OutputSchema,
		Sources:      srcs,
	}, onDelta)
	if err != nil {
		return nil, err
	}
	citations := results(hits)
	if !req.Text {
		for i := range citations {
			citations[i].Text = nil
		}
	}
	return &websearch.AnswerResponse{RequestID: system.GenerateRequestID(), Answer: answer, Citations: citations}, nil
}

// answerModel maps an Exa answer model to the LLM tier and search type.
func answerModel(model string) (Tier, string) {
	switch model {
	case websearch.AnswerModelExaFast:
		return TierFast, websearch.SearchTypeAuto
	case websearch.AnswerModelExaPro:
		return TierStrong, websearch.SearchTypeAuto
	case websearch.AnswerModelExaResearch:
		return TierStrong, websearch.SearchTypeDeep
	default:
		return TierDefault, websearch.SearchTypeAuto
	}
}
