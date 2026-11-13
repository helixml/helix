package metering

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/websearch"
)

type MeteringSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	inner    *websearch.MockProvider
	provider *Provider
	ctx      context.Context
	// log is the order things happened in: emitted events, debits and
	// records.
	log []string
	// rows are the usage log rows written.
	rows []types.WebSearchRequest
}

func TestMeteringSuite(t *testing.T) {
	suite.Run(t, new(MeteringSuite))
}

const (
	testOrgID     = "org_1"
	testUserID    = "usr_1"
	testRequestID = "req_1"
	testWalletID  = "wal_1"
	testKeyName   = "ci key"
)

var enabled = config.Stripe{BillingEnabled: true, MinimumInferenceBalance: 0.01}

func (s *MeteringSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.inner = websearch.NewMockProvider(s.ctrl)
	s.provider = New(s.inner, testPrices, enabled, s.store)
	s.ctx = websearch.WithCaller(context.Background(), websearch.Caller{
		UserID: testUserID, OrgID: testOrgID, RequestID: testRequestID,
		Source: types.WebSearchSourceAPI, APIKeyName: testKeyName,
	})
	s.log = nil
	s.rows = nil
	s.store.EXPECT().CreateWebSearchRequest(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, row *types.WebSearchRequest) error {
			s.log = append(s.log, "record")
			s.rows = append(s.rows, *row)
			return nil
		})
}

// row returns the one usage log row written.
func (s *MeteringSuite) row() types.WebSearchRequest {
	s.Require().Len(s.rows, 1, "one usage log row per call")
	return s.rows[0]
}

// orgWallet expects the org's wallet to be read, with balance.
func (s *MeteringSuite) orgWallet(balance float64) {
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), testOrgID).AnyTimes().
		Return(&types.Wallet{ID: testWalletID, OrgID: testOrgID, Balance: balance}, nil)
}

// expectDebit expects one debit of amount for the test request.
func (s *MeteringSuite) expectDebit(amount float64) {
	s.store.EXPECT().UpdateWalletBalance(gomock.Any(), testWalletID, gomock.Any(), types.TransactionMetadata{
		WebSearchRequestID: testRequestID,
		TransactionType:    types.TransactionTypeUsage,
	}).DoAndReturn(func(_ context.Context, _ string, got float64, _ types.TransactionMetadata) (*types.Wallet, error) {
		s.InDelta(amount, got, 1e-12)
		s.log = append(s.log, "debit")
		return &types.Wallet{ID: testWalletID}, nil
	})
}

func (s *MeteringSuite) requireNoMoreCredits(err error) {
	e := websearch.AsError(err)
	s.Equal(http.StatusPaymentRequired, e.Status)
	s.Equal(websearch.TagNoMoreCredits, e.Tag)
}

func (s *MeteringSuite) TestSearchReportsAndDebitsCost() {
	s.orgWallet(1)
	s.inner.EXPECT().Search(gomock.Any(), gomock.Any()).Return(&websearch.SearchResponse{
		Results: append(pages(11, websearch.Result{}), websearch.Result{Text: str("t")}),
	}, nil)
	s.expectDebit(-0.010)

	resp, err := s.provider.Search(s.ctx, &websearch.SearchRequest{Query: "q", Type: "fast"})
	s.Require().NoError(err)
	s.Equal(&websearch.CostDollars{Total: 0.010, Contents: &websearch.ContentsCost{Text: ptr(0.001)}}, resp.CostDollars)
}

func (s *MeteringSuite) TestUserWalletWithoutOrg() {
	s.store.EXPECT().GetWalletByUser(gomock.Any(), testUserID).Times(2).Return(&types.Wallet{ID: testWalletID, UserID: testUserID, Balance: 1}, nil)
	s.inner.EXPECT().Answer(gomock.Any(), gomock.Any()).Return(&websearch.AnswerResponse{Answer: "a"}, nil)
	s.expectDebit(-0.005)

	ctx := websearch.WithCaller(context.Background(), websearch.Caller{UserID: testUserID, RequestID: testRequestID, Source: types.WebSearchSourceAPI})
	resp, err := s.provider.Answer(ctx, &websearch.AnswerRequest{Query: "q"})
	s.Require().NoError(err)
	s.Equal(&websearch.CostDollars{Total: 0.005}, resp.CostDollars)
}

func (s *MeteringSuite) TestBelowMinimumBalanceIsRefused() {
	s.orgWallet(0.009)

	_, err := s.provider.Search(s.ctx, &websearch.SearchRequest{Query: "q"})
	s.requireNoMoreCredits(err)
	_, err = s.provider.Contents(s.ctx, &websearch.ContentsRequest{URLs: []string{"https://a"}})
	s.requireNoMoreCredits(err)
	_, err = s.provider.FindSimilar(s.ctx, &websearch.FindSimilarRequest{URL: "https://a"})
	s.requireNoMoreCredits(err)
	_, err = s.provider.Answer(s.ctx, &websearch.AnswerRequest{Query: "q"})
	s.requireNoMoreCredits(err)
	s.requireNoMoreCredits(s.provider.StreamSearch(s.ctx, &websearch.SearchRequest{Query: "q"}, s.emitSearch))
	s.requireNoMoreCredits(s.provider.StreamAnswer(s.ctx, &websearch.AnswerRequest{Query: "q"}, s.emitAnswer))
	s.Equal(slices.Repeat([]string{"record"}, 6), s.log, "nothing emitted, every refusal recorded")
}

func (s *MeteringSuite) TestFailedCallIsNotCharged() {
	s.orgWallet(1)
	s.inner.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	_, err := s.provider.Contents(s.ctx, &websearch.ContentsRequest{URLs: []string{"https://a"}})
	s.ErrorContains(err, "boom")
}

func (s *MeteringSuite) TestNothingReturnedIsNotCharged() {
	s.orgWallet(1)
	s.inner.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(&websearch.ContentsResponse{
		Statuses: []websearch.ContentStatus{{ID: "https://a", Status: websearch.ContentStatusError}},
	}, nil)

	resp, err := s.provider.Contents(s.ctx, &websearch.ContentsRequest{URLs: []string{"https://a"}})
	s.Require().NoError(err)
	s.Equal(&websearch.CostDollars{}, resp.CostDollars)
}

func (s *MeteringSuite) TestFindSimilarIsPricedAsASearch() {
	s.orgWallet(1)
	s.inner.EXPECT().FindSimilar(gomock.Any(), gomock.Any()).Return(&websearch.FindSimilarResponse{
		Results: pages(12, websearch.Result{Highlights: []string{"h"}}),
	}, nil)
	s.expectDebit(-0.021)

	resp, err := s.provider.FindSimilar(s.ctx, &websearch.FindSimilarRequest{URL: "https://a"})
	s.Require().NoError(err)
	s.Equal(&websearch.CostDollars{Total: 0.021, Contents: &websearch.ContentsCost{Highlights: ptr(0.012)}}, resp.CostDollars)
}

func (s *MeteringSuite) TestDebitFailureFailsTheCall() {
	s.orgWallet(0.02)
	s.inner.EXPECT().Answer(gomock.Any(), gomock.Any()).Return(&websearch.AnswerResponse{}, nil)
	s.store.EXPECT().UpdateWalletBalance(gomock.Any(), testWalletID, gomock.Any(), gomock.Any()).
		Return(nil, errors.New(types.ErrorInsufficientBalance))

	resp, err := s.provider.Answer(s.ctx, &websearch.AnswerRequest{Query: "q"})
	s.Nil(resp)
	s.ErrorContains(err, "debit web search request req_1")
}

func (s *MeteringSuite) TestBillingDisabledReportsCostWithoutDebit() {
	provider := New(s.inner, testPrices, config.Stripe{MinimumInferenceBalance: 0.01}, s.store)
	s.inner.EXPECT().Search(gomock.Any(), gomock.Any()).Return(&websearch.SearchResponse{}, nil)

	resp, err := provider.Search(s.ctx, &websearch.SearchRequest{Query: "q", Type: websearch.SearchTypeDeep})
	s.Require().NoError(err)
	s.Equal(&websearch.CostDollars{Total: 0.012}, resp.CostDollars)
}

func (s *MeteringSuite) TestSystemCallerIsNotBilled() {
	s.inner.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(&websearch.ContentsResponse{
		Results: []websearch.Result{{Text: str("t")}},
	}, nil)

	ctx := websearch.WithCaller(context.Background(), websearch.Caller{UserID: "runner-system", RequestID: testRequestID, Source: types.WebSearchSourceAPI, System: true})
	resp, err := s.provider.Contents(ctx, &websearch.ContentsRequest{URLs: []string{"https://a"}})
	s.Require().NoError(err)
	s.Equal(0.001, resp.CostDollars.Total)
}

func (s *MeteringSuite) TestCallerIsRequired() {
	for _, caller := range []websearch.Caller{
		{UserID: testUserID, Source: types.WebSearchSourceAPI},
		{RequestID: testRequestID, Source: types.WebSearchSourceAPI},
		{UserID: testUserID, RequestID: testRequestID},
	} {
		ctx := websearch.WithCaller(context.Background(), caller)
		_, err := s.provider.Answer(ctx, &websearch.AnswerRequest{Query: "q"})
		s.ErrorContains(err, "must have a user id, request id and source")
	}
	s.Empty(s.rows, "a call that cannot be attributed is not recorded")
}

func (s *MeteringSuite) TestUnpricedSearchTypeIsRefused() {
	_, err := s.provider.Search(s.ctx, &websearch.SearchRequest{Query: "q", Type: "neural"})
	s.Equal(websearch.TagFeatureDisabled, websearch.AsError(err).Tag)
	s.Equal(websearch.TagFeatureDisabled, websearch.AsError(s.provider.StreamSearch(s.ctx, &websearch.SearchRequest{Query: "q", Type: "neural"}, s.emitSearch)).Tag)
}

func (s *MeteringSuite) emitSearch(ev websearch.SearchEvent) error {
	entry := ev.Type
	if ev.CostDollars != nil {
		entry += " cost"
	}
	s.log = append(s.log, entry)
	if ev.Type == websearch.SearchEventDone {
		s.Equal(&websearch.CostDollars{Total: 0.017, Contents: &websearch.ContentsCost{Text: ptr(0.003)}}, ev.CostDollars)
	}
	return nil
}

func (s *MeteringSuite) emitAnswer(ev websearch.AnswerEvent) error {
	switch {
	case ev.CostDollars != nil:
		s.Equal(websearch.AnswerEvent{CostDollars: &websearch.CostDollars{Total: 0.005}}, ev)
		s.log = append(s.log, "cost")
	case ev.Citations != nil:
		s.log = append(s.log, "citations")
	default:
		s.log = append(s.log, "delta")
	}
	return nil
}

func (s *MeteringSuite) TestStreamSearchCostsTheDoneEventThenDebits() {
	s.orgWallet(1)
	s.inner.EXPECT().StreamSearch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *websearch.SearchRequest, emit func(websearch.SearchEvent) error) error {
			for _, ev := range []websearch.SearchEvent{
				{Type: websearch.SearchEventResults, Results: append(pages(9, websearch.Result{}), pages(3, websearch.Result{Text: str("t")})...)},
				{Type: websearch.SearchEventTextDelta, Delta: str("x")},
				// The upstream's own cost is replaced, not added to.
				{Type: websearch.SearchEventDone, CostDollars: &websearch.CostDollars{Total: 0.5}},
			} {
				if err := emit(ev); err != nil {
					return err
				}
			}
			return nil
		})
	s.expectDebit(-0.017) // deep-lite, 2 results beyond ten, 3 texts

	s.Require().NoError(s.provider.StreamSearch(s.ctx, &websearch.SearchRequest{Query: "q", Type: websearch.SearchTypeDeepLite}, s.emitSearch))
	s.Equal([]string{"results", "text-delta", "done cost", "debit", "record"}, s.log)
}

func (s *MeteringSuite) TestStreamSearchWithoutDoneIsNotCharged() {
	s.orgWallet(1)
	s.inner.EXPECT().StreamSearch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *websearch.SearchRequest, emit func(websearch.SearchEvent) error) error {
			return emit(websearch.SearchEvent{Type: websearch.SearchEventResults})
		})

	err := s.provider.StreamSearch(s.ctx, &websearch.SearchRequest{Query: "q"}, s.emitSearch)
	s.ErrorContains(err, "without a done event")
}

func (s *MeteringSuite) TestStreamAnswerDebitsThenEndsWithCostChunk() {
	s.orgWallet(1)
	s.inner.EXPECT().StreamAnswer(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *websearch.AnswerRequest, emit func(websearch.AnswerEvent) error) error {
			for _, ev := range []websearch.AnswerEvent{
				{Choices: []websearch.StreamChoice{{Delta: websearch.StreamDelta{Content: "a"}}}},
				{Citations: []websearch.Result{{URL: "https://a"}}},
				{CostDollars: &websearch.CostDollars{Total: 0.5}}, // the exa provider's cost chunk
			} {
				if err := emit(ev); err != nil {
					return err
				}
			}
			return nil
		})
	s.expectDebit(-0.005)

	s.Require().NoError(s.provider.StreamAnswer(s.ctx, &websearch.AnswerRequest{Query: "q"}, s.emitAnswer))
	s.Equal([]string{"delta", "citations", "debit", "cost", "record"}, s.log, "one cost chunk, Helix's, after the debit; recorded when the stream ends")
}

func (s *MeteringSuite) TestStreamAnswerFailureIsNotCharged() {
	s.orgWallet(1)
	s.inner.EXPECT().StreamAnswer(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *websearch.AnswerRequest, emit func(websearch.AnswerEvent) error) error {
			if err := emit(websearch.AnswerEvent{Choices: []websearch.StreamChoice{{}}}); err != nil {
				return err
			}
			return errors.New("model overloaded")
		})

	err := s.provider.StreamAnswer(s.ctx, &websearch.AnswerRequest{Query: "q"}, s.emitAnswer)
	s.ErrorContains(err, "model overloaded")
	s.Equal([]string{"delta", "record"}, s.log, "no cost chunk")
}

func (s *MeteringSuite) TestOrgWithoutRequiredSubscriptionIsRefused() {
	s.provider = New(s.inner, testPrices, config.Stripe{BillingEnabled: true, RequireActiveSubscription: true}, s.store)
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), testOrgID).Return(&types.Wallet{ID: testWalletID, OrgID: testOrgID, Balance: 100}, nil)

	_, err := s.provider.Search(s.ctx, &websearch.SearchRequest{Query: "q"})
	s.requireNoMoreCredits(err)
}

func (s *MeteringSuite) TestSuccessIsRecordedWithWhatWasCharged() {
	s.orgWallet(1)
	s.inner.EXPECT().Search(gomock.Any(), gomock.Any()).Return(&websearch.SearchResponse{
		Results: append(pages(11, websearch.Result{}), websearch.Result{Text: str("t")}),
	}, nil)
	s.expectDebit(-0.010)

	_, err := s.provider.Search(s.ctx, &websearch.SearchRequest{Query: "golang generics"})
	s.Require().NoError(err)
	row := s.row()
	s.WithinDuration(time.Now(), row.Created, time.Minute)
	row.Created = time.Time{}
	s.Equal(types.WebSearchRequest{
		ID: testRequestID, OrganizationID: testOrgID, UserID: testUserID, APIKeyName: testKeyName,
		Source: types.WebSearchSourceAPI, Endpoint: types.WebSearchEndpointSearch,
		Query: "golang generics", SearchType: websearch.SearchTypeAuto,
		Status: http.StatusOK, Results: 12, CostDollars: 0.010,
	}, row)
}

func (s *MeteringSuite) TestFailureIsRecordedUncharged() {
	s.orgWallet(1)
	s.inner.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(nil, &websearch.Error{Status: http.StatusBadGateway, Tag: websearch.TagUpstreamError, Message: "down"})

	_, err := s.provider.Contents(s.ctx, &websearch.ContentsRequest{URLs: []string{"https://a", "https://b", "https://c"}})
	s.Require().Error(err)
	row := s.row()
	s.Equal(types.WebSearchEndpointContents, row.Endpoint)
	s.Equal("https://a (+2 more)", row.Query)
	s.Equal(http.StatusBadGateway, row.Status)
	s.Equal(websearch.TagUpstreamError, row.ErrorTag)
	s.Zero(row.CostDollars)
}

func (s *MeteringSuite) TestOutOfCreditsIsRecorded() {
	s.orgWallet(0)

	_, err := s.provider.FindSimilar(s.ctx, &websearch.FindSimilarRequest{URL: "https://a"})
	s.requireNoMoreCredits(err)
	row := s.row()
	s.Equal(types.WebSearchEndpointFindSimilar, row.Endpoint)
	s.Equal("https://a", row.Query)
	s.Equal(http.StatusPaymentRequired, row.Status)
	s.Equal(websearch.TagNoMoreCredits, row.ErrorTag)
	s.Zero(row.DurationMs, "the provider was never called")
}

func (s *MeteringSuite) TestInvalidRequestIsRefusedAndRecorded() {
	_, err := s.provider.Search(s.ctx, &websearch.SearchRequest{Query: ""})
	s.Equal(websearch.TagInvalidRequest, websearch.AsError(err).Tag)
	s.Equal(http.StatusBadRequest, s.row().Status)
	s.Equal(websearch.TagInvalidRequest, s.row().ErrorTag)
}

func (s *MeteringSuite) TestLongQueryIsTruncated() {
	s.orgWallet(1)
	s.inner.EXPECT().Answer(gomock.Any(), gomock.Any()).Return(&websearch.AnswerResponse{Citations: []websearch.Result{{}, {}}}, nil)
	s.expectDebit(-0.005)

	_, err := s.provider.Answer(s.ctx, &websearch.AnswerRequest{Query: strings.Repeat("é", maxQueryChars+5)})
	s.Require().NoError(err)
	s.Equal(strings.Repeat("é", maxQueryChars), s.row().Query)
	s.Equal(2, s.row().Results, "an answer's results are its citations")
}

func (s *MeteringSuite) TestAgentCallIsRecordedAsAgent() {
	s.orgWallet(1)
	s.inner.EXPECT().Search(gomock.Any(), gomock.Any()).Return(&websearch.SearchResponse{}, nil)
	s.expectDebit(-0.007)

	ctx := websearch.WithCaller(context.Background(), websearch.Caller{UserID: testUserID, OrgID: testOrgID, RequestID: testRequestID, Source: types.WebSearchSourceAgent})
	_, err := s.provider.Search(ctx, &websearch.SearchRequest{Query: "q"})
	s.Require().NoError(err)
	s.Equal(types.WebSearchSourceAgent, s.row().Source)
	s.Empty(s.row().APIKeyName)
}

func (s *MeteringSuite) TestBillingDisabledRecordsNothingCharged() {
	provider := New(s.inner, testPrices, config.Stripe{MinimumInferenceBalance: 0.01}, s.store)
	s.inner.EXPECT().Search(gomock.Any(), gomock.Any()).Return(&websearch.SearchResponse{}, nil)

	resp, err := provider.Search(s.ctx, &websearch.SearchRequest{Query: "q"})
	s.Require().NoError(err)
	s.Equal(0.007, resp.CostDollars.Total, "the price is still reported")
	s.Zero(s.row().CostDollars)
}

func (s *MeteringSuite) TestStreamSearchIsRecordedWhenItEnds() {
	s.orgWallet(1)
	s.inner.EXPECT().StreamSearch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *websearch.SearchRequest, emit func(websearch.SearchEvent) error) error {
			if err := emit(websearch.SearchEvent{Type: websearch.SearchEventResults, Results: pages(3, websearch.Result{})}); err != nil {
				return err
			}
			return &websearch.Error{Status: http.StatusBadGateway, Tag: websearch.TagUpstreamError, Message: "model overloaded"}
		})

	err := s.provider.StreamSearch(s.ctx, &websearch.SearchRequest{Query: "q", Type: websearch.SearchTypeDeep}, s.emitSearch)
	s.Require().Error(err)
	s.Equal([]string{"results", "record"}, s.log)
	row := s.row()
	s.True(row.Stream)
	s.Equal(websearch.SearchTypeDeep, row.SearchType)
	s.Equal(http.StatusBadGateway, row.Status)
	s.Equal(3, row.Results, "what was streamed before the failure")
	s.Zero(row.CostDollars)
}

func (s *MeteringSuite) TestRecordFailureFailsTheCall() {
	store := store.NewMockStore(s.ctrl)
	provider := New(s.inner, testPrices, config.Stripe{}, store)
	s.inner.EXPECT().Answer(gomock.Any(), gomock.Any()).Return(&websearch.AnswerResponse{}, nil)
	store.EXPECT().CreateWebSearchRequest(gomock.Any(), gomock.Any()).Return(errors.New("db down"))

	resp, err := provider.Answer(s.ctx, &websearch.AnswerRequest{Query: "q"})
	s.Nil(resp)
	s.ErrorContains(err, "record web search request req_1: db down")
}

func (s *MeteringSuite) TestRecordOutlivesTheClient() {
	store := store.NewMockStore(s.ctrl)
	provider := New(s.inner, testPrices, config.Stripe{}, store)
	ctx, cancel := context.WithCancel(s.ctx)
	s.inner.EXPECT().Search(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *websearch.SearchRequest) (*websearch.SearchResponse, error) {
		cancel()
		return nil, context.Canceled
	})
	store.EXPECT().CreateWebSearchRequest(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ *types.WebSearchRequest) error {
		return ctx.Err()
	})

	_, err := provider.Search(ctx, &websearch.SearchRequest{Query: "q"})
	s.ErrorIs(err, context.Canceled, "the call's own error, not the record's")
	s.NotContains(err.Error(), "record web search request")
}
