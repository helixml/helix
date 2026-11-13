package billing

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/websearch"
)

type BillingSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	inner    *websearch.MockProvider
	provider *Provider
	ctx      context.Context
	// log is the order things happened in: emitted events and debits.
	log []string
}

func TestBillingSuite(t *testing.T) {
	suite.Run(t, new(BillingSuite))
}

const (
	testOrgID     = "org_1"
	testUserID    = "usr_1"
	testRequestID = "req_1"
	testWalletID  = "wal_1"
)

var enabled = config.Stripe{BillingEnabled: true, MinimumInferenceBalance: 0.01}

func (s *BillingSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.inner = websearch.NewMockProvider(s.ctrl)
	s.provider = New(s.inner, testPrices, enabled, s.store)
	s.ctx = websearch.WithCaller(context.Background(), websearch.Caller{UserID: testUserID, OrgID: testOrgID, RequestID: testRequestID})
	s.log = nil
}

// orgWallet expects the org's wallet to be read, with balance.
func (s *BillingSuite) orgWallet(balance float64) {
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), testOrgID).AnyTimes().
		Return(&types.Wallet{ID: testWalletID, OrgID: testOrgID, Balance: balance}, nil)
}

// expectDebit expects one debit of amount for the test request.
func (s *BillingSuite) expectDebit(amount float64) {
	s.store.EXPECT().UpdateWalletBalance(gomock.Any(), testWalletID, gomock.Any(), types.TransactionMetadata{
		WebSearchRequestID: testRequestID,
		TransactionType:    types.TransactionTypeUsage,
	}).DoAndReturn(func(_ context.Context, _ string, got float64, _ types.TransactionMetadata) (*types.Wallet, error) {
		s.InDelta(amount, got, 1e-12)
		s.log = append(s.log, "debit")
		return &types.Wallet{ID: testWalletID}, nil
	})
}

func (s *BillingSuite) requireNoMoreCredits(err error) {
	e := websearch.AsError(err)
	s.Equal(http.StatusPaymentRequired, e.Status)
	s.Equal(websearch.TagNoMoreCredits, e.Tag)
}

func (s *BillingSuite) TestSearchReportsAndDebitsCost() {
	s.orgWallet(1)
	s.inner.EXPECT().Search(gomock.Any(), gomock.Any()).Return(&websearch.SearchResponse{
		Results: append(pages(11, websearch.Result{}), websearch.Result{Text: str("t")}),
	}, nil)
	s.expectDebit(-0.010)

	resp, err := s.provider.Search(s.ctx, &websearch.SearchRequest{Query: "q", Type: "fast"})
	s.Require().NoError(err)
	s.Equal(&websearch.CostDollars{Total: 0.010, Contents: &websearch.ContentsCost{Text: ptr(0.001)}}, resp.CostDollars)
}

func (s *BillingSuite) TestUserWalletWithoutOrg() {
	s.store.EXPECT().GetWalletByUser(gomock.Any(), testUserID).Times(2).Return(&types.Wallet{ID: testWalletID, UserID: testUserID, Balance: 1}, nil)
	s.inner.EXPECT().Answer(gomock.Any(), gomock.Any()).Return(&websearch.AnswerResponse{Answer: "a"}, nil)
	s.expectDebit(-0.005)

	ctx := websearch.WithCaller(context.Background(), websearch.Caller{UserID: testUserID, RequestID: testRequestID})
	resp, err := s.provider.Answer(ctx, &websearch.AnswerRequest{Query: "q"})
	s.Require().NoError(err)
	s.Equal(&websearch.CostDollars{Total: 0.005}, resp.CostDollars)
}

func (s *BillingSuite) TestBelowMinimumBalanceIsRefused() {
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
	s.Empty(s.log, "nothing emitted")
}

func (s *BillingSuite) TestFailedCallIsNotCharged() {
	s.orgWallet(1)
	s.inner.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	_, err := s.provider.Contents(s.ctx, &websearch.ContentsRequest{URLs: []string{"https://a"}})
	s.ErrorContains(err, "boom")
}

func (s *BillingSuite) TestNothingReturnedIsNotCharged() {
	s.orgWallet(1)
	s.inner.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(&websearch.ContentsResponse{
		Statuses: []websearch.ContentStatus{{ID: "https://a", Status: websearch.ContentStatusError}},
	}, nil)

	resp, err := s.provider.Contents(s.ctx, &websearch.ContentsRequest{URLs: []string{"https://a"}})
	s.Require().NoError(err)
	s.Equal(&websearch.CostDollars{}, resp.CostDollars)
}

func (s *BillingSuite) TestFindSimilarIsPricedAsASearch() {
	s.orgWallet(1)
	s.inner.EXPECT().FindSimilar(gomock.Any(), gomock.Any()).Return(&websearch.FindSimilarResponse{
		Results: pages(12, websearch.Result{Highlights: []string{"h"}}),
	}, nil)
	s.expectDebit(-0.021)

	resp, err := s.provider.FindSimilar(s.ctx, &websearch.FindSimilarRequest{URL: "https://a"})
	s.Require().NoError(err)
	s.Equal(&websearch.CostDollars{Total: 0.021, Contents: &websearch.ContentsCost{Highlights: ptr(0.012)}}, resp.CostDollars)
}

func (s *BillingSuite) TestDebitFailureFailsTheCall() {
	s.orgWallet(0.02)
	s.inner.EXPECT().Answer(gomock.Any(), gomock.Any()).Return(&websearch.AnswerResponse{}, nil)
	s.store.EXPECT().UpdateWalletBalance(gomock.Any(), testWalletID, gomock.Any(), gomock.Any()).
		Return(nil, errors.New(types.ErrorInsufficientBalance))

	resp, err := s.provider.Answer(s.ctx, &websearch.AnswerRequest{Query: "q"})
	s.Nil(resp)
	s.ErrorContains(err, "debit web search request req_1")
}

func (s *BillingSuite) TestBillingDisabledReportsCostWithoutDebit() {
	provider := New(s.inner, testPrices, config.Stripe{MinimumInferenceBalance: 0.01}, s.store)
	s.inner.EXPECT().Search(gomock.Any(), gomock.Any()).Return(&websearch.SearchResponse{}, nil)

	resp, err := provider.Search(s.ctx, &websearch.SearchRequest{Query: "q", Type: websearch.SearchTypeDeep})
	s.Require().NoError(err)
	s.Equal(&websearch.CostDollars{Total: 0.012}, resp.CostDollars)
}

func (s *BillingSuite) TestSystemCallerIsNotBilled() {
	s.inner.EXPECT().Contents(gomock.Any(), gomock.Any()).Return(&websearch.ContentsResponse{
		Results: []websearch.Result{{Text: str("t")}},
	}, nil)

	ctx := websearch.WithCaller(context.Background(), websearch.Caller{UserID: "runner-system", RequestID: testRequestID, System: true})
	resp, err := s.provider.Contents(ctx, &websearch.ContentsRequest{URLs: []string{"https://a"}})
	s.Require().NoError(err)
	s.Equal(0.001, resp.CostDollars.Total)
}

func (s *BillingSuite) TestCallerIsRequired() {
	for _, caller := range []websearch.Caller{{UserID: testUserID}, {RequestID: testRequestID}} {
		ctx := websearch.WithCaller(context.Background(), caller)
		_, err := s.provider.Answer(ctx, &websearch.AnswerRequest{Query: "q"})
		s.ErrorContains(err, "must have a user id and request id")
	}
}

func (s *BillingSuite) TestUnpricedSearchTypeIsRefused() {
	_, err := s.provider.Search(s.ctx, &websearch.SearchRequest{Query: "q", Type: "neural"})
	s.Equal(websearch.TagFeatureDisabled, websearch.AsError(err).Tag)
	s.Equal(websearch.TagFeatureDisabled, websearch.AsError(s.provider.StreamSearch(s.ctx, &websearch.SearchRequest{Query: "q", Type: "neural"}, s.emitSearch)).Tag)
}

func (s *BillingSuite) emitSearch(ev websearch.SearchEvent) error {
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

func (s *BillingSuite) emitAnswer(ev websearch.AnswerEvent) error {
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

func (s *BillingSuite) TestStreamSearchCostsTheDoneEventThenDebits() {
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
	s.Equal([]string{"results", "text-delta", "done cost", "debit"}, s.log)
}

func (s *BillingSuite) TestStreamSearchWithoutDoneIsNotCharged() {
	s.orgWallet(1)
	s.inner.EXPECT().StreamSearch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *websearch.SearchRequest, emit func(websearch.SearchEvent) error) error {
			return emit(websearch.SearchEvent{Type: websearch.SearchEventResults})
		})

	err := s.provider.StreamSearch(s.ctx, &websearch.SearchRequest{Query: "q"}, s.emitSearch)
	s.ErrorContains(err, "without a done event")
}

func (s *BillingSuite) TestStreamAnswerDebitsThenEndsWithCostChunk() {
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
	s.Equal([]string{"delta", "citations", "debit", "cost"}, s.log, "one cost chunk, Helix's, after the debit")
}

func (s *BillingSuite) TestStreamAnswerFailureIsNotCharged() {
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
	s.Equal([]string{"delta"}, s.log, "no cost chunk")
}

func (s *BillingSuite) TestOrgWithoutRequiredSubscriptionIsRefused() {
	s.provider = New(s.inner, testPrices, config.Stripe{BillingEnabled: true, RequireActiveSubscription: true}, s.store)
	s.store.EXPECT().GetWalletByOrg(gomock.Any(), testOrgID).Return(&types.Wallet{ID: testWalletID, OrgID: testOrgID, Balance: 100}, nil)

	_, err := s.provider.Search(s.ctx, &websearch.SearchRequest{Query: "q"})
	s.requireNoMoreCredits(err)
}
