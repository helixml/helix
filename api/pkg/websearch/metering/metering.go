// Package metering is the one place every web search call passes through,
// whichever provider serves the deployment and whoever calls it (the API,
// agent skills, the CLI through the API). It checks the request against the
// API contract, prices the response from Helix's table (what the provider
// costs Helix is not what the caller pays), charges it to Helix credits, and
// records the call in the usage log.
package metering

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

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
//
// Every call is recorded once its outcome is known, failures included:
// after the response, and for a stream when the stream ends. The record is
// part of the call, as the debit is: a call whose record cannot be written
// fails.
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

// maxQueryChars bounds the query the log keeps; a request body can hold a
// megabyte of it.
const maxQueryChars = 1000

// call is one metered request: the row recorded for it, filled in as it
// runs.
type call struct {
	caller websearch.Caller
	row    types.WebSearchRequest
}

// meter runs serve as one call and records its outcome.
func (p *Provider) meter(ctx context.Context, row types.WebSearchRequest, serve func(*call) error) error {
	caller := websearch.CallerFrom(ctx)
	if caller.RequestID == "" || caller.UserID == "" || caller.Source == "" {
		return fmt.Errorf("web search caller must have a user id, request id and source, got %+v", caller)
	}
	row.ID = caller.RequestID
	row.Created = time.Now()
	row.OrganizationID = caller.OrgID
	row.UserID = caller.UserID
	row.APIKeyName = caller.APIKeyName
	row.Source = caller.Source
	if runes := []rune(row.Query); len(runes) > maxQueryChars {
		row.Query = string(runes[:maxQueryChars])
	}
	c := &call{caller: caller, row: row}

	err := serve(c)
	c.row.Status = http.StatusOK
	if err != nil {
		e := websearch.AsError(err)
		c.row.Status, c.row.ErrorTag = e.Status, e.Tag
	}
	// The record outlives a client that has gone away.
	if recordErr := p.store.CreateWebSearchRequest(context.WithoutCancel(ctx), &c.row); recordErr != nil {
		return fmt.Errorf("record web search request %s: %w", caller.RequestID, recordErr)
	}
	return err
}

// timed runs the provider's part of the call, which is what its duration
// measures.
func (c *call) timed(run func() error) error {
	start := time.Now()
	err := run()
	c.row.DurationMs = time.Since(start).Milliseconds()
	return err
}

func (p *Provider) Search(ctx context.Context, req *websearch.SearchRequest) (*websearch.SearchResponse, error) {
	var resp *websearch.SearchResponse
	err := p.meter(ctx, searchRow(req, false), func(c *call) error {
		base, err := p.searchBase(ctx, c, req)
		if err != nil {
			return err
		}
		if err := c.timed(func() (err error) { resp, err = p.inner.Search(ctx, req); return err }); err != nil {
			return err
		}
		c.row.Results = len(resp.Results)
		resp.CostDollars = resultsCost(p.prices, base, resp.Results)
		return p.charge(ctx, c, resp.CostDollars)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// StreamSearch prices the results streamed and reports the cost on the done
// event; the debit follows the stream.
func (p *Provider) StreamSearch(ctx context.Context, req *websearch.SearchRequest, emit func(websearch.SearchEvent) error) error {
	return p.meter(ctx, searchRow(req, true), func(c *call) error {
		base, err := p.searchBase(ctx, c, req)
		if err != nil {
			return err
		}
		var (
			results []websearch.Result
			cost    *websearch.CostDollars
		)
		err = c.timed(func() error {
			return p.inner.StreamSearch(ctx, req, func(ev websearch.SearchEvent) error {
				results = append(results, ev.Results...)
				if ev.Type == websearch.SearchEventDone {
					cost = resultsCost(p.prices, base, results)
					ev.CostDollars = cost
				}
				return emit(ev)
			})
		})
		c.row.Results = len(results)
		if err != nil {
			return err
		}
		if cost == nil {
			return fmt.Errorf("web search stream ended without a %s event", websearch.SearchEventDone)
		}
		return p.charge(ctx, c, cost)
	})
}

func searchRow(req *websearch.SearchRequest, stream bool) types.WebSearchRequest {
	searchType := req.Type
	if searchType == "" {
		searchType = websearch.SearchTypeAuto
	}
	return types.WebSearchRequest{Endpoint: types.WebSearchEndpointSearch, Query: req.Query, SearchType: searchType, Stream: stream}
}

// searchBase checks a /search request and its caller, and returns its base
// price.
func (p *Provider) searchBase(ctx context.Context, c *call, req *websearch.SearchRequest) (float64, error) {
	if err := req.Validate(); err != nil {
		return 0, err
	}
	base, err := searchBase(p.prices, req.Type)
	if err != nil {
		return 0, err
	}
	return base, p.authorize(ctx, c.caller)
}

func (p *Provider) Contents(ctx context.Context, req *websearch.ContentsRequest) (*websearch.ContentsResponse, error) {
	var resp *websearch.ContentsResponse
	err := p.meter(ctx, contentsRow(req), func(c *call) error {
		if err := req.Validate(); err != nil {
			return err
		}
		if err := p.authorize(ctx, c.caller); err != nil {
			return err
		}
		if err := c.timed(func() (err error) { resp, err = p.inner.Contents(ctx, req); return err }); err != nil {
			return err
		}
		c.row.Results = len(resp.Results)
		resp.CostDollars = contentsCost(p.prices, resp.Results)
		return p.charge(ctx, c, resp.CostDollars)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// contentsRow names a /contents request by its first page and how many
// more it asked for.
func contentsRow(req *websearch.ContentsRequest) types.WebSearchRequest {
	pages := append(append([]string{}, req.URLs...), req.IDs...)
	var query string
	if len(pages) > 0 {
		query = pages[0]
	}
	if len(pages) > 1 {
		query += fmt.Sprintf(" (+%d more)", len(pages)-1)
	}
	return types.WebSearchRequest{Endpoint: types.WebSearchEndpointContents, Query: query}
}

func (p *Provider) FindSimilar(ctx context.Context, req *websearch.FindSimilarRequest) (*websearch.FindSimilarResponse, error) {
	var resp *websearch.FindSimilarResponse
	err := p.meter(ctx, types.WebSearchRequest{Endpoint: types.WebSearchEndpointFindSimilar, Query: req.URL}, func(c *call) error {
		if err := req.Validate(); err != nil {
			return err
		}
		if err := p.authorize(ctx, c.caller); err != nil {
			return err
		}
		if err := c.timed(func() (err error) { resp, err = p.inner.FindSimilar(ctx, req); return err }); err != nil {
			return err
		}
		c.row.Results = len(resp.Results)
		resp.CostDollars = resultsCost(p.prices, p.prices.FindSimilar, resp.Results)
		return p.charge(ctx, c, resp.CostDollars)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (p *Provider) Answer(ctx context.Context, req *websearch.AnswerRequest) (*websearch.AnswerResponse, error) {
	var resp *websearch.AnswerResponse
	err := p.meter(ctx, answerRow(req, false), func(c *call) error {
		if err := req.Validate(); err != nil {
			return err
		}
		if err := p.authorize(ctx, c.caller); err != nil {
			return err
		}
		if err := c.timed(func() (err error) { resp, err = p.inner.Answer(ctx, req); return err }); err != nil {
			return err
		}
		c.row.Results = len(resp.Citations)
		resp.CostDollars = p.answerCost()
		return p.charge(ctx, c, resp.CostDollars)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// StreamAnswer debits once the answer has streamed and ends with the cost
// chunk. An upstream cost chunk (the exa provider's) is Helix's cost, not
// the caller's price, and is dropped.
func (p *Provider) StreamAnswer(ctx context.Context, req *websearch.AnswerRequest, emit func(websearch.AnswerEvent) error) error {
	return p.meter(ctx, answerRow(req, true), func(c *call) error {
		if err := req.Validate(); err != nil {
			return err
		}
		if err := p.authorize(ctx, c.caller); err != nil {
			return err
		}
		err := c.timed(func() error {
			return p.inner.StreamAnswer(ctx, req, func(ev websearch.AnswerEvent) error {
				if ev.CostDollars != nil {
					return nil
				}
				c.row.Results += len(ev.Citations)
				return emit(ev)
			})
		})
		if err != nil {
			return err
		}
		cost := p.answerCost()
		if err := p.charge(ctx, c, cost); err != nil {
			return err
		}
		return emit(websearch.AnswerEvent{CostDollars: cost})
	})
}

func answerRow(req *websearch.AnswerRequest, stream bool) types.WebSearchRequest {
	return types.WebSearchRequest{Endpoint: types.WebSearchEndpointAnswer, Query: req.Query, Stream: stream}
}

// answerCost is the flat /answer price: its searches and fetches are
// included, not itemised.
func (p *Provider) answerCost() *websearch.CostDollars {
	return &websearch.CostDollars{Total: p.prices.Answer}
}

// authorize refuses a caller whose wallet is below the minimum balance.
func (p *Provider) authorize(ctx context.Context, caller websearch.Caller) error {
	if caller.System {
		return nil
	}
	ok, err := wallet.HasEnoughBalance(ctx, p.store, p.stripe, caller.OrgID, caller.UserID, true)
	if errors.Is(err, wallet.ErrNoActiveSubscription) {
		return &websearch.Error{
			Status:  http.StatusPaymentRequired,
			Tag:     websearch.TagNoMoreCredits,
			Message: "Web search needs an active Helix subscription for this organization.",
		}
	}
	if err != nil {
		return fmt.Errorf("check web search balance: %w", err)
	}
	if !ok {
		return &websearch.Error{
			Status:  http.StatusPaymentRequired,
			Tag:     websearch.TagNoMoreCredits,
			Message: "You have run out of Helix credits. Top up your balance to continue using web search.",
		}
	}
	return nil
}

// charge debits cost from the caller's wallet when billing is enabled, and
// records what was charged.
func (p *Provider) charge(ctx context.Context, c *call, cost *websearch.CostDollars) error {
	if !p.stripe.BillingEnabled || c.caller.System || cost.Total == 0 {
		return nil
	}
	w, err := wallet.For(ctx, p.store, c.caller.OrgID, c.caller.UserID)
	if err != nil {
		return fmt.Errorf("get wallet for web search: %w", err)
	}
	_, err = p.store.UpdateWalletBalance(ctx, w.ID, -cost.Total, types.TransactionMetadata{
		WebSearchRequestID: c.caller.RequestID,
		TransactionType:    types.TransactionTypeUsage,
	})
	if err != nil {
		return fmt.Errorf("debit web search request %s: %w", c.caller.RequestID, err)
	}
	c.row.CostDollars = cost.Total
	return nil
}
