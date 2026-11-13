package metering

import (
	"fmt"
	"math"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/websearch"
)

// freeResults is how many /search and /findSimilar results the base price
// covers.
const freeResults = 10

// searchBase is the base price of a /search of type typ. Legacy types are
// priced as Exa prices them, which is not always the type Helix runs them as
// (instant runs as auto but is priced as lite).
func searchBase(p config.WebSearchPrices, typ string) (float64, error) {
	switch typ {
	case websearch.SearchTypeLite, "instant":
		return p.SearchLite, nil
	case "", websearch.SearchTypeAuto, "fast":
		return p.Search, nil
	case websearch.SearchTypeDeep, websearch.SearchTypeDeepLite, "deep-reasoning":
		return p.SearchDeep, nil
	default:
		return 0, websearch.FeatureDisabled("type", fmt.Sprintf("search type %q has no price", typ))
	}
}

// resultsCost prices a /search or /findSimilar response: base, each result
// beyond the free ones, and the content returned.
func resultsCost(p config.WebSearchPrices, base float64, results []websearch.Result) *websearch.CostDollars {
	c := contentsCost(p, results)
	c.Total = dollars(c.Total + base + float64(max(len(results)-freeResults, 0))*p.ResultOver10)
	return c
}

// contentsCost prices the text, highlights and summaries returned, per page,
// subpages included. A page that failed to fetch, or a content type not
// returned, costs nothing.
func contentsCost(p config.WebSearchPrices, results []websearch.Result) *websearch.CostDollars {
	var n pageContents
	n.count(results)
	c := &websearch.CostDollars{}
	if n == (pageContents{}) {
		return c
	}
	charge := func(pages int) *float64 {
		if pages == 0 {
			return nil
		}
		v := dollars(float64(pages) * p.ContentPage)
		return &v
	}
	c.Contents = &websearch.ContentsCost{Text: charge(n.text), Highlights: charge(n.highlights), Summary: charge(n.summary)}
	c.Total = dollars(float64(n.text+n.highlights+n.summary) * p.ContentPage)
	return c
}

// pageContents counts the pages that returned each content type.
type pageContents struct {
	text, highlights, summary int
}

func (n *pageContents) count(results []websearch.Result) {
	for _, r := range results {
		if r.Text != nil {
			n.text++
		}
		if len(r.Highlights) > 0 {
			n.highlights++
		}
		if r.Summary != nil {
			n.summary++
		}
		n.count(r.Subpages)
	}
}

// dollars rounds to millionths of a dollar, so sums of prices print as the
// prices do instead of carrying float noise into responses and the ledger.
func dollars(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
