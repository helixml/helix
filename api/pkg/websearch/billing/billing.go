// Package billing charges web search to Helix credits. It wraps whichever
// provider serves the deployment, so every consumer (the API, agent skills,
// the CLI through the API) is priced from Helix's table and billed the same
// way: what the provider costs Helix is not what the caller pays.
package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/wallet"
	"github.com/helixml/helix/api/pkg/websearch"
)

// Provider prices every response, reports the price as costDollars and,
// when Stripe billing is enabled, debits it from the caller's wallet. A
// caller below the minimum balance is refused before any work is done; a
// failed call is not charged.
type Provider struct {
	inner  websearch.Provider
	prices config.WebSearchPrices
	stripe config.Stripe
	store  store.Store
}

var _ websearch.Provider = (*Provider)(nil)

func New(inner websearch.Provider, prices config.WebSearchPrices, stripe config.Stripe, store store.Store) *Provider {
	return &Provider{inner: inner, prices: prices, stripe: stripe, store: store}
}

func (p *Provider) Search(ctx context.Context, req *websearch.SearchRequest) (*websearch.SearchResponse, error) {
	base, err := searchBase(p.prices, req.Type)
	if err != nil {
		return nil, err
	}
	caller, err := p.authorize(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := p.inner.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	resp.CostDollars = resultsCost(p.prices, base, resp.Results)
	if err := p.charge(ctx, caller, resp.CostDollars); err != nil {
		return nil, err
	}
	return resp, nil
}

// StreamSearch prices the results streamed and reports the cost on the done
// event; the debit follows the stream.
func (p *Provider) StreamSearch(ctx context.Context, req *websearch.SearchRequest, emit func(websearch.SearchEvent) error) error {
	base, err := searchBase(p.prices, req.Type)
	if err != nil {
		return err
	}
	caller, err := p.authorize(ctx)
	if err != nil {
		return err
	}
	var (
		results []websearch.Result
		cost    *websearch.CostDollars
	)
	err = p.inner.StreamSearch(ctx, req, func(ev websearch.SearchEvent) error {
		results = append(results, ev.Results...)
		if ev.Type == websearch.SearchEventDone {
			cost = resultsCost(p.prices, base, results)
			ev.CostDollars = cost
		}
		return emit(ev)
	})
	if err != nil {
		return err
	}
	if cost == nil {
		return fmt.Errorf("web search stream ended without a %s event", websearch.SearchEventDone)
	}
	return p.charge(ctx, caller, cost)
}

func (p *Provider) Contents(ctx context.Context, req *websearch.ContentsRequest) (*websearch.ContentsResponse, error) {
	caller, err := p.authorize(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := p.inner.Contents(ctx, req)
	if err != nil {
		return nil, err
	}
	resp.CostDollars = contentsCost(p.prices, resp.Results)
	if err := p.charge(ctx, caller, resp.CostDollars); err != nil {
		return nil, err
	}
	return resp, nil
}

func (p *Provider) FindSimilar(ctx context.Context, req *websearch.FindSimilarRequest) (*websearch.FindSimilarResponse, error) {
	caller, err := p.authorize(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := p.inner.FindSimilar(ctx, req)
	if err != nil {
		return nil, err
	}
	resp.CostDollars = resultsCost(p.prices, p.prices.FindSimilar, resp.Results)
	if err := p.charge(ctx, caller, resp.CostDollars); err != nil {
		return nil, err
	}
	return resp, nil
}

func (p *Provider) Answer(ctx context.Context, req *websearch.AnswerRequest) (*websearch.AnswerResponse, error) {
	caller, err := p.authorize(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := p.inner.Answer(ctx, req)
	if err != nil {
		return nil, err
	}
	resp.CostDollars = p.answerCost()
	if err := p.charge(ctx, caller, resp.CostDollars); err != nil {
		return nil, err
	}
	return resp, nil
}

// StreamAnswer debits once the answer has streamed and ends with the cost
// chunk. An upstream cost chunk (the exa provider's) is Helix's cost, not
// the caller's price, and is dropped.
func (p *Provider) StreamAnswer(ctx context.Context, req *websearch.AnswerRequest, emit func(websearch.AnswerEvent) error) error {
	caller, err := p.authorize(ctx)
	if err != nil {
		return err
	}
	err = p.inner.StreamAnswer(ctx, req, func(ev websearch.AnswerEvent) error {
		if ev.CostDollars != nil {
			return nil
		}
		return emit(ev)
	})
	if err != nil {
		return err
	}
	cost := p.answerCost()
	if err := p.charge(ctx, caller, cost); err != nil {
		return err
	}
	return emit(websearch.AnswerEvent{CostDollars: cost})
}

// answerCost is the flat /answer price: its searches and fetches are
// included, not itemised.
func (p *Provider) answerCost() *websearch.CostDollars {
	return &websearch.CostDollars{Total: p.prices.Answer}
}

// authorize returns the caller and refuses one whose wallet is below the
// minimum balance.
func (p *Provider) authorize(ctx context.Context) (websearch.Caller, error) {
	caller := websearch.CallerFrom(ctx)
	if caller.RequestID == "" || caller.UserID == "" {
		return caller, fmt.Errorf("web search caller must have a user id and request id, got %+v", caller)
	}
	if caller.System {
		return caller, nil
	}
	ok, err := wallet.HasEnoughBalance(ctx, p.store, p.stripe, caller.OrgID, caller.UserID, true)
	if errors.Is(err, wallet.ErrNoActiveSubscription) {
		return caller, &websearch.Error{
			Status:  http.StatusPaymentRequired,
			Tag:     websearch.TagNoMoreCredits,
			Message: "Web search needs an active Helix subscription for this organization.",
		}
	}
	if err != nil {
		return caller, fmt.Errorf("check web search balance: %w", err)
	}
	if !ok {
		return caller, &websearch.Error{
			Status:  http.StatusPaymentRequired,
			Tag:     websearch.TagNoMoreCredits,
			Message: "You have run out of Helix credits. Top up your balance to continue using web search.",
		}
	}
	return caller, nil
}

// charge debits cost from the caller's wallet when billing is enabled.
func (p *Provider) charge(ctx context.Context, caller websearch.Caller, cost *websearch.CostDollars) error {
	if !p.stripe.BillingEnabled || caller.System || cost.Total == 0 {
		return nil
	}
	w, err := wallet.For(ctx, p.store, caller.OrgID, caller.UserID)
	if err != nil {
		return fmt.Errorf("get wallet for web search: %w", err)
	}
	_, err = p.store.UpdateWalletBalance(ctx, w.ID, -cost.Total, types.TransactionMetadata{
		WebSearchRequestID: caller.RequestID,
		TransactionType:    types.TransactionTypeUsage,
	})
	if err != nil {
		return fmt.Errorf("debit web search request %s: %w", caller.RequestID, err)
	}
	return nil
}
