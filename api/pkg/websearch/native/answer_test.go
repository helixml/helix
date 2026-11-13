package native

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/websearch"
)

func TestAnswerModels(t *testing.T) {
	tests := []struct {
		model string
		tier  Tier
		deep  bool
	}{
		{"", TierDefault, false},
		{"exa", TierDefault, false},
		{"exa-fast", TierFast, false},
		{"exa-pro", TierStrong, false},
		{"exa-research", TierStrong, true},
	}
	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			e := newEnv(t)
			e.pipeline(nil)
			if tc.deep {
				e.synth.EXPECT().ExpandQuery(gomock.Any(), "q", "", deepExpansions).Return(nil, nil)
			}
			got := e.discover(cands("https://a.com/")...)
			e.synth.EXPECT().Answer(gomock.Any(), gomock.Any(), gomock.Nil()).DoAndReturn(
				func(_ context.Context, a *AnswerSynthesis, _ func(string) error) (any, error) {
					assert.Equal(t, tc.tier, a.Tier)
					return "42", nil
				})
			_, err := e.provider(t).Answer(context.Background(), &websearch.AnswerRequest{Query: "q", Model: tc.model})
			require.NoError(t, err)
			assert.Equal(t, answerNumResults, got.NumResults)
		})
	}
}

func TestAnswer(t *testing.T) {
	for _, withText := range []bool{false, true} {
		t.Run(map[bool]string{false: "without text", true: "with text"}[withText], func(t *testing.T) {
			e := newEnv(t)
			e.pipeline(map[string]error{"https://b.com/": &FetchError{Tag: websearch.TagCrawlNotFound, Err: errors.New("404")}})
			got := e.discover(cands("https://a.com/", "https://b.com/")...)
			schema := json.RawMessage(`{"type":"object"}`)
			e.synth.EXPECT().Answer(gomock.Any(), &AnswerSynthesis{
				Tier: TierDefault, Query: "q", SystemPrompt: "sys", OutputSchema: schema,
				Sources: []Source{{URL: "https://a.com/", Title: "Candidate https://a.com/", Text: bodyOf("https://a.com/")}},
			}, gomock.Nil()).Return(map[string]any{"x": 1.0}, nil)

			resp, err := e.provider(t).Answer(context.Background(), &websearch.AnswerRequest{
				Query: "q", SystemPrompt: "sys", OutputSchema: schema, Text: withText,
				UserLocation: &websearch.UserLocation{Country: "FR"},
			})
			require.NoError(t, err)
			assert.Equal(t, "FR", got.Country)
			assert.Equal(t, map[string]any{"x": 1.0}, resp.Answer)
			assert.True(t, strings.HasPrefix(resp.RequestID, "req_"))
			require.Len(t, resp.Citations, 1, "citations are the sources the answer was given")
			assert.Equal(t, "https://a.com/", resp.Citations[0].URL)
			if withText {
				assert.Equal(t, bodyOf("https://a.com/"), *resp.Citations[0].Text)
			} else {
				assert.Nil(t, resp.Citations[0].Text)
			}
		})
	}
}

func TestAnswerFailures(t *testing.T) {
	t.Run("needs LLM", func(t *testing.T) {
		e := newEnv(t)
		e.opts.Synthesizer = nil
		_, err := e.provider(t).Answer(context.Background(), &websearch.AnswerRequest{Query: "q"})
		requireTag(t, err, websearch.TagFeatureDisabled)
	})
	t.Run("invalid model", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.provider(t).Answer(context.Background(), &websearch.AnswerRequest{Query: "q", Model: "gpt"})
		requireTag(t, err, websearch.TagInvalidRequest)
	})
	t.Run("no page could be fetched", func(t *testing.T) {
		e := newEnv(t)
		timeout := &FetchError{Tag: websearch.TagCrawlTimeout, Err: errors.New("slow")}
		e.pipeline(map[string]error{"https://a.com/": timeout, "https://b.com/": timeout})
		e.discover(cands("https://a.com/", "https://b.com/")...)
		// No Synthesizer.Answer expectation: an answer grounded in nothing is never generated.
		_, err := e.provider(t).Answer(context.Background(), &websearch.AnswerRequest{Query: "q"})
		requireError(t, err, http.StatusBadGateway, websearch.TagUpstreamError)
	})
	t.Run("no results", func(t *testing.T) {
		e := newEnv(t)
		e.discover()
		_, err := e.provider(t).Answer(context.Background(), &websearch.AnswerRequest{Query: "q"})
		requireError(t, err, http.StatusNotFound, websearch.TagNotFound)
	})
	t.Run("search fails", func(t *testing.T) {
		e := newEnv(t)
		e.disc.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(nil, errors.New("searxng down"))
		_, err := e.provider(t).Answer(context.Background(), &websearch.AnswerRequest{Query: "q"})
		require.ErrorContains(t, err, "searxng down")
	})
}

func TestStreamAnswer(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.discover(cands("https://a.com/")...)
	e.synth.EXPECT().Answer(gomock.Any(), gomock.Any(), gomock.Not(gomock.Nil())).DoAndReturn(
		func(_ context.Context, _ *AnswerSynthesis, onDelta func(string) error) (any, error) {
			require.NoError(t, onDelta("4"))
			require.NoError(t, onDelta("2"))
			return "42", nil
		})
	var events []websearch.AnswerEvent
	err := e.provider(t).StreamAnswer(context.Background(), &websearch.AnswerRequest{Query: "q", Stream: true},
		func(ev websearch.AnswerEvent) error {
			events = append(events, ev)
			return nil
		})
	require.NoError(t, err)
	require.Len(t, events, 3)
	assert.Equal(t, websearch.StreamDelta{Role: "assistant", Content: "4"}, events[0].Choices[0].Delta)
	assert.Equal(t, websearch.StreamDelta{Content: "2"}, events[1].Choices[0].Delta)
	assert.Equal(t, "https://a.com/", events[2].Citations[0].URL)
	assert.Nil(t, events[2].Citations[0].Text)
	assert.Empty(t, events[2].Choices)
}

func TestStreamAnswerOutputSchema(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.discover(cands("https://a.com/")...)
	// The object is sent whole, so the synthesizer gets no delta callback.
	e.synth.EXPECT().Answer(gomock.Any(), gomock.Any(), gomock.Nil()).Return(map[string]any{"x": 1}, nil)
	var events []websearch.AnswerEvent
	err := e.provider(t).StreamAnswer(context.Background(), &websearch.AnswerRequest{Query: "q", OutputSchema: json.RawMessage(`{"type":"object"}`)},
		func(ev websearch.AnswerEvent) error {
			events = append(events, ev)
			return nil
		})
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, `{"x":1}`, events[0].Choices[0].Delta.Content)
	assert.NotEmpty(t, events[1].Citations)
}
