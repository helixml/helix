package native

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/websearch"
)

// charsPerSentence converts the deprecated highlights.numSentences.
const charsPerSentence = 1333

// renderConfig is what rendering contents depends on besides the options.
type renderConfig struct {
	textLimit       int    // text.maxCharacters when the request sets none
	query           string // highlights query when the request sets none; "" uses the page title
	highlightScores bool   // Exa returns highlightScores on /findSimilar only
}

// pageOptions say which pages behind search hits to fetch and how to filter them.
type pageOptions struct {
	contents    *websearch.ContentsOptions // nil: no contents requested
	needText    bool                       // fetch without contents, for filters, context or synthesis
	includeText []string
	excludeText []string
	render      renderConfig
}

// subpageTally counts the subpages renderContents tried to fetch and those it could not.
type subpageTally struct{ tried, failed int }

func (t *subpageTally) add(o subpageTally) {
	t.tried += o.tried
	t.failed += o.failed
}

func (t subpageTally) warnings() []websearch.Warning {
	return unfetchedWarnings(t.failed, t.tried, "subpages")
}

// unfetchedWarnings reports that failed of total pages could not be fetched.
func unfetchedWarnings(failed, total int, what string) []websearch.Warning {
	if failed == 0 {
		return nil
	}
	return []websearch.Warning{partialResults("", fmt.Sprintf("%d of %d %s could not be fetched", failed, total, what))}
}

func (p *Provider) Contents(ctx context.Context, req *websearch.ContentsRequest) (*websearch.ContentsResponse, error) {
	start := time.Now()
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.Compliance != nil {
		return nil, websearch.FeatureDisabled("compliance", "compliance modes are a property of the deployment")
	}
	opts := &req.ContentsOptions
	if err := p.checkContents(opts, ""); err != nil {
		return nil, err
	}
	// A document id is the page's normalised URL, so ids and urls load alike.
	field, targets := "urls", req.URLs
	if len(req.IDs) > 0 {
		field, targets = "ids", req.IDs
	}
	keys := make([]string, len(targets))
	for i, u := range targets {
		n, err := websearch.NormalizeURL(u)
		if err != nil {
			return nil, websearch.InvalidRequest("%s[%d] is not a valid URL: %v", field, i, err)
		}
		keys[i] = n
	}

	render := renderConfig{textLimit: defaultContentsTextChars}
	pol, textOpts := policyFor(opts), textOptions(opts)
	hits := make([]hit, len(targets))
	statuses := make([]websearch.ContentStatus, len(targets))
	tallies := make([]subpageTally, len(targets))
	err := p.forEach(ctx, len(targets), func(ctx context.Context, i int) error {
		statuses[i] = websearch.ContentStatus{ID: targets[i]}
		d, err := p.loadDoc(ctx, keys[i], pol, textOpts)
		if fe, ok := pageFailure(err); ok {
			statuses[i].Status = websearch.ContentStatusError
			statuses[i].Error = &websearch.ContentError{Tag: fe.Tag, HTTPStatusCode: fe.HTTPStatusCode}
			return nil
		}
		if err != nil {
			return err
		}
		hits[i] = hit{result: newResult(d), doc: d}
		if tallies[i], err = p.renderContents(ctx, &hits[i].result, d, opts, render); err != nil {
			return err
		}
		statuses[i].Status = websearch.ContentStatusSuccess
		statuses[i].Source = d.source
		return nil
	})
	if err != nil {
		return nil, err
	}
	var subpages subpageTally
	for _, t := range tallies {
		subpages.add(t)
	}
	hits = slices.DeleteFunc(hits, func(h hit) bool { return h.doc == nil })
	return &websearch.ContentsResponse{
		RequestID:  system.GenerateRequestID(),
		Results:    results(hits),
		Statuses:   statuses,
		Warnings:   append(contentsWarnings(opts, ""), subpages.warnings()...),
		Context:    buildContext(hits, req.Context, render.textLimit),
		SearchTime: elapsedMs(start),
	}, nil
}

// contentsWarnings reports deprecated contents fields that have no effect.
// prefix is the path of the contents options in the request.
func contentsWarnings(o *websearch.ContentsOptions, prefix string) []websearch.Warning {
	if o == nil {
		return nil
	}
	var w []websearch.Warning
	ignored := func(path string) {
		w = append(w, websearch.Warning{Code: websearch.WarningIgnoredField, Path: prefix + path, Message: path + " is deprecated and ignored"})
	}
	if o.Highlights.Requested() && o.Highlights.HighlightsPerURL > 0 {
		ignored("highlights.highlightsPerUrl")
	}
	if o.Extras != nil {
		if o.Extras.RichLinks > 0 {
			ignored("extras.richLinks")
		}
		if o.Extras.CodeBlocks > 0 {
			ignored("extras.codeBlocks")
		}
	}
	return w
}

// fetchHits fetches the pages behind hits when contents or text are needed,
// renders the requested contents onto them, and applies the text filters.
// A page that fails to fetch leaves its hit without contents.
func (p *Provider) fetchHits(ctx context.Context, hits []hit, po pageOptions) ([]hit, []websearch.Warning, error) {
	filtering := len(po.includeText) > 0 || len(po.excludeText) > 0
	if po.contents == nil && !po.needText && !filtering {
		return hits, nil, nil
	}
	opts := po.contents
	if opts == nil {
		opts = &websearch.ContentsOptions{}
	}
	pol, textOpts := policyFor(opts), textOptions(opts)
	tallies := make([]subpageTally, len(hits))
	err := p.forEach(ctx, len(hits), func(ctx context.Context, i int) error {
		d, err := p.loadDoc(ctx, hits[i].result.URL, pol, textOpts)
		if _, ok := pageFailure(err); ok {
			return nil
		}
		if err != nil {
			return err
		}
		hits[i].doc = d
		tallies[i], err = p.renderContents(ctx, &hits[i].result, d, opts, po.render)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	failed := 0
	var subpages subpageTally
	for i, h := range hits {
		if h.doc == nil {
			failed++
		}
		subpages.add(tallies[i])
	}
	warnings := append(unfetchedWarnings(failed, len(hits), "results"), subpages.warnings()...)
	if filtering {
		hits = slices.DeleteFunc(hits, func(h hit) bool {
			return h.doc == nil || !matchesText(h.doc.page.Text, po.includeText, po.excludeText)
		})
	}
	return hits, warnings, nil
}

// matchesText reports whether text contains every include phrase and no
// exclude phrase, ignoring case.
func matchesText(text string, include, exclude []string) bool {
	text = strings.ToLower(text)
	for _, s := range include {
		if !strings.Contains(text, strings.ToLower(s)) {
			return false
		}
	}
	for _, s := range exclude {
		if strings.Contains(text, strings.ToLower(s)) {
			return false
		}
	}
	return true
}

// renderContents fills r from d: page metadata r lacks, and the contents o asks for.
func (p *Provider) renderContents(ctx context.Context, r *websearch.Result, d *doc, o *websearch.ContentsOptions, rc renderConfig) (subpageTally, error) {
	page := d.page
	if r.Title == "" {
		r.Title = page.Title
	}
	if r.Author == nil && page.Author != "" {
		r.Author = &page.Author
	}
	if r.PublishedDate == nil {
		r.PublishedDate = page.PublishedDate
	}
	if r.Image == "" {
		r.Image = page.Image
	}
	if r.Favicon == "" {
		r.Favicon = page.Favicon
	}

	if o.Text.Requested() {
		limit := o.Text.MaxCharacters
		if limit == 0 {
			limit = rc.textLimit
		}
		text := clipChars(page.Text, limit)
		r.Text = &text
	}
	if h := o.Highlights; h.Requested() {
		maxChars := h.MaxCharacters
		if maxChars == 0 {
			maxChars = h.NumSentences * charsPerSentence
		}
		query := h.Query
		if query == "" {
			query = rc.query
		}
		if query == "" {
			query = page.Title
		}
		highlights, scores, err := Highlights(ctx, p.opts.Ranker, query, page.Text, maxChars)
		if err != nil {
			return subpageTally{}, err
		}
		r.Highlights = highlights
		if rc.highlightScores {
			r.HighlightScores = scores
		}
	}
	// A page with no text has nothing to summarise and gets no summary.
	if s := o.Summary; s.Requested() && strings.TrimSpace(page.Text) != "" {
		summary, err := p.opts.Synthesizer.Summarize(ctx, page.Text, s.Query, s.Schema)
		if err != nil {
			return subpageTally{}, fmt.Errorf("summarize %s: %w", d.raw.URL, err)
		}
		r.Summary = &summary
	}
	if e := o.Extras; e != nil {
		r.Extras = extras(page, e)
	}
	if o.Subpages == 0 {
		return subpageTally{}, nil
	}
	subpages, tally, err := p.subpages(ctx, d, o, rc)
	if err != nil {
		return subpageTally{}, err
	}
	r.Subpages = subpages
	return tally, nil
}

func extras(page *Page, e *websearch.ExtrasOptions) *websearch.Extras {
	var out websearch.Extras
	for _, l := range page.Links[:min(e.Links, len(page.Links))] {
		out.Links = append(out.Links, l.URL)
	}
	for _, l := range page.ImageLinks[:min(e.ImageLinks, len(page.ImageLinks))] {
		out.ImageLinks = append(out.ImageLinks, l.URL)
	}
	out.RichImageLinks = slices.Clone(page.ImageLinks[:min(e.RichImageLinks, len(page.ImageLinks))])
	if out.Links == nil && out.ImageLinks == nil && len(out.RichImageLinks) == 0 {
		return nil
	}
	return &out
}

// subpages fetches up to o.Subpages same-host pages linked from d, best
// subpageTarget match first. Subpages that fail to fetch are left out and
// counted in the tally.
func (p *Provider) subpages(ctx context.Context, d *doc, o *websearch.ContentsOptions, rc renderConfig) ([]websearch.Result, subpageTally, error) {
	links := subpageLinks(d.raw.URL, d.page.Links, o.SubpageTarget, o.Subpages)
	sub := *o
	sub.Subpages = 0
	pol, textOpts := policyFor(o), textOptions(o)
	pages := make([]*websearch.Result, len(links))
	err := p.forEach(ctx, len(links), func(ctx context.Context, i int) error {
		sd, err := p.loadDoc(ctx, links[i], pol, textOpts)
		if _, ok := pageFailure(err); ok {
			return nil
		}
		if err != nil {
			return err
		}
		r := newResult(sd)
		if _, err := p.renderContents(ctx, &r, sd, &sub, rc); err != nil {
			return err
		}
		pages[i] = &r
		return nil
	})
	if err != nil {
		return nil, subpageTally{}, err
	}
	tally := subpageTally{tried: len(links)}
	var out []websearch.Result
	for _, r := range pages {
		if r == nil {
			tally.failed++
			continue
		}
		out = append(out, *r)
	}
	return out, tally, nil
}

// subpageLinks picks up to n distinct same-host links from pageURL's links,
// ordered by how many targets each mentions in its URL or anchor, then by
// position on the page.
func subpageLinks(pageURL string, links []Link, targets []string, n int) []string {
	host := hostOf(pageURL)
	type candidate struct {
		url   string
		score int
	}
	seen := map[string]bool{pageURL: true}
	var cands []candidate
	for _, l := range links {
		u, err := websearch.NormalizeURL(l.URL)
		if err != nil || seen[u] || hostOf(u) != host {
			continue
		}
		seen[u] = true
		haystack := strings.ToLower(u + " " + l.Anchor)
		score := 0
		for _, t := range targets {
			if t != "" && strings.Contains(haystack, strings.ToLower(t)) {
				score++
			}
		}
		cands = append(cands, candidate{url: u, score: score})
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].score > cands[b].score })
	out := make([]string, 0, min(n, len(cands)))
	for _, c := range cands[:min(n, len(cands))] {
		out = append(out, c.url)
	}
	return out
}

// buildContext concatenates the fetched pages' text for the deprecated
// context option.
func buildContext(hits []hit, o *websearch.ContextOption, textLimit int) string {
	if !o.Requested() {
		return ""
	}
	var b strings.Builder
	for _, h := range hits {
		if h.doc == nil {
			continue
		}
		fmt.Fprintf(&b, "Title: %s\nURL: %s\n%s\n\n", h.result.Title, h.result.URL, clipChars(h.doc.page.Text, textLimit))
	}
	return clipChars(strings.TrimSpace(b.String()), o.MaxCharacters)
}
