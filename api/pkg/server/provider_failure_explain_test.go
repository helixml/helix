package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"

	"go.uber.org/mock/gomock"
)

// The generic ACP abort message is all Zed could send before the cause
// travelled with AcpThreadEvent::Error. Older sandboxes still send it, so
// Helix must be able to explain it from what it recorded itself.
type ProviderFailureExplainSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestProviderFailureExplainSuite(t *testing.T) {
	suite.Run(t, new(ProviderFailureExplainSuite))
}

func (s *ProviderFailureExplainSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: s.store}
}

const upstream502 = "error, status code: 502, status: 502 Bad Gateway, message: upstream unavailable"

func llmCall(age time.Duration, errMsg string) *types.LLMCall {
	return &types.LLMCall{
		Model:   "qwen3.8-27b",
		Created: time.Now().Add(-age),
		Error:   errMsg,
	}
}

// A specific error already tells the user what happened; rewriting it would
// destroy information.
func (s *ProviderFailureExplainSuite) TestSpecificErrorUntouched() {
	specific := "compilation failed: syntax error"
	got, _ := s.server.maybeExplainProviderFailure(context.Background(), "ses_1", specific)
	s.Equal(specific, got)
}

// The incident case: the provider 502s, the harness exhausts its retries, and
// the turn dies. The user must see the 502, not a pointer to a deleted log.
func (s *ProviderFailureExplainSuite) TestRecentProviderFailureExplainsAbort() {
	s.store.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).Return([]*types.LLMCall{
		llmCall(4*time.Second, upstream502),
		llmCall(5*time.Second, upstream502),
		llmCall(6*time.Second, upstream502),
		llmCall(30*time.Second, ""),
	}, int64(4), nil)

	got, failure := s.server.maybeExplainProviderFailure(context.Background(), "ses_1", genericAbort)
	s.Equal("The AI model was temporarily unavailable, so this task did not run.", got)
	s.NotNil(failure)
	s.NotContains(got, "exited mid-turn")
	s.NotContains(got, "Zed.log")
}

// A single failure is reported without the "consecutive" clause — claiming a
// pattern from one data point would be a fabrication.
func (s *ProviderFailureExplainSuite) TestSingleFailureOmitsAttemptCount() {
	s.store.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).Return([]*types.LLMCall{
		llmCall(2*time.Second, upstream502),
		llmCall(20*time.Second, ""),
	}, int64(2), nil)

	failure, ok := s.server.recentProviderFailure(context.Background(), "ses_1")
	s.True(ok)
	s.Contains(failure.detail, "upstream unavailable")
	s.NotContains(failure.detail, "consecutive")
}

// A failure from earlier in the session that the agent already recovered from
// is not the cause of an abort happening now. Blaming it would send the user
// after the wrong problem.
func (s *ProviderFailureExplainSuite) TestStaleFailureIsNotBlamed() {
	s.store.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).Return([]*types.LLMCall{
		llmCall(30*time.Minute, upstream502),
	}, int64(1), nil)

	got, _ := s.server.maybeExplainProviderFailure(context.Background(), "ses_1", genericAbort)
	s.Equal(genericAbort, got)
}

// Every recent call succeeded, so the abort has some other cause (a real agent
// crash, or max tokens). Leave the message alone rather than invent one.
func (s *ProviderFailureExplainSuite) TestNoFailedCallsLeavesMessageAlone() {
	s.store.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).Return([]*types.LLMCall{
		llmCall(2*time.Second, ""),
		llmCall(9*time.Second, ""),
	}, int64(2), nil)

	got, _ := s.server.maybeExplainProviderFailure(context.Background(), "ses_1", genericAbort)
	s.Equal(genericAbort, got)
}

// A store failure must degrade to the original message, never to an error.
func (s *ProviderFailureExplainSuite) TestStoreErrorDegradesGracefully() {
	s.store.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).
		Return(nil, int64(0), errors.New("db down"))

	got, _ := s.server.maybeExplainProviderFailure(context.Background(), "ses_1", genericAbort)
	s.Equal(genericAbort, got)
}

// A provider error long enough to swamp the UI is truncated, but the head —
// where the status code lives — survives.
func (s *ProviderFailureExplainSuite) TestLongErrorIsTruncated() {
	long := upstream502
	for len(long) < 600 {
		long += " padding"
	}
	s.store.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).Return([]*types.LLMCall{
		llmCall(time.Second, long),
	}, int64(1), nil)

	failure, ok := s.server.recentProviderFailure(context.Background(), "ses_1")
	s.True(ok)
	s.Contains(failure.detail, "502 Bad Gateway")
	s.Contains(failure.detail, "…")
	s.Less(len(failure.detail), 400)
}

func (s *ProviderFailureExplainSuite) TestRawTransportErrorIsExplained() {
	s.store.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).Return([]*types.LLMCall{
		llmCall(time.Second, "status code 524"),
	}, int64(1), nil)

	got, failure := s.server.maybeExplainProviderFailure(context.Background(), "ses_1", `agent turn aborted: Internal error: status code 524: {"service":"session","errorName":"APIError"}`)
	s.Contains(got, "temporarily unavailable")
	s.NotNil(failure)
	s.NotContains(got, "524")
}

func (s *ProviderFailureExplainSuite) TestRawClientTransportErrorIsPreserved() {
	raw := `agent turn aborted: Internal error: status code 402: payment required`
	got, failure := s.server.maybeExplainProviderFailure(context.Background(), "ses_1", raw)
	s.Equal(raw, got)
	s.Nil(failure)
}
