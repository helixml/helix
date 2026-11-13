package native

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/sourcegraph/conc/pool"

	"github.com/helixml/helix/api/pkg/searxng"
	"github.com/helixml/helix/api/pkg/websearch"
)

const (
	// discoverMaxPages caps the SearXNG pages fetched per query.
	discoverMaxPages = 5
	// discoverConcurrency bounds the queries run against SearXNG at once.
	discoverConcurrency = 4
	// siteOperatorCap is the most site: operators added to one query; longer
	// domain lists rely on the post-filter alone.
	siteOperatorCap = 10
	// Merging is weighted reciprocal rank fusion: a URL scores
	// Σ weight/(rrfK+rank) over the queries that returned it, and the
	// user's query weighs userQueryWeight against 1 for each other query.
	rrfK            = 60
	userQueryWeight = 2.0
)

// SearXNGDiscoverer finds candidates through SearXNG. Filters SearXNG cannot
// express exactly are sent as coarse hints and then enforced on the results.
type SearXNGDiscoverer struct {
	client *searxng.Client
	now    func() time.Time
}

func NewSearXNGDiscoverer(client *searxng.Client) *SearXNGDiscoverer {
	return &SearXNGDiscoverer{client: client, now: time.Now}
}

// categoryRewrites are keywords appended to the query for Exa categories
// SearXNG has no equivalent for, steering keyword engines toward the kind of
// page Exa returns (people → LinkedIn profiles, financial report → filings).
var categoryRewrites = map[string]string{
	websearch.CategoryCompany:         "company",
	websearch.CategoryPeople:          "linkedin",
	websearch.CategoryPersonalSite:    "personal blog",
	websearch.CategoryPublication:     "article",
	websearch.CategoryFinancialReport: "annual report 10-K",
}

// countryLanguages maps ISO 3166-1 alpha-2 countries to SearXNG locales.
var countryLanguages = map[string]string{
	"AR": "es-AR", "AT": "de-AT", "AU": "en-AU", "BE": "fr-BE", "BR": "pt-BR",
	"CA": "en-CA", "CH": "de-CH", "CN": "zh-CN", "CZ": "cs-CZ", "DE": "de-DE",
	"DK": "da-DK", "ES": "es-ES", "FI": "fi-FI", "FR": "fr-FR", "GB": "en-GB",
	"GR": "el-GR", "HK": "zh-HK", "IE": "en-IE", "IL": "he-IL", "IN": "en-IN",
	"IT": "it-IT", "JP": "ja-JP", "KR": "ko-KR", "MX": "es-MX", "NL": "nl-NL",
	"NO": "nb-NO", "NZ": "en-NZ", "PL": "pl-PL", "PT": "pt-PT", "RU": "ru-RU",
	"SE": "sv-SE", "TR": "tr-TR", "TW": "zh-TW", "UA": "uk-UA", "US": "en-US",
	"ZA": "en-ZA",
}

// discoverPlan is a DiscoverRequest translated into SearXNG terms.
type discoverPlan struct {
	base     searxng.Query // everything but Q and PageNo
	suffix   string        // category keywords and site: operators
	filter   candidateFilter
	warnings []websearch.Warning
}

func (d *SearXNGDiscoverer) Discover(ctx context.Context, req *DiscoverRequest) (*DiscoverResult, error) {
	if len(req.Queries) == 0 {
		return nil, errors.New("discover: no queries")
	}
	if req.NumResults < 1 {
		return nil, fmt.Errorf("discover: numResults must be positive, got %d", req.NumResults)
	}
	plan, err := d.plan(req)
	if err != nil {
		return nil, err
	}

	runs := make([]queryRun, len(req.Queries))
	errs := make([]error, len(req.Queries))
	p := pool.New().WithMaxGoroutines(discoverConcurrency)
	for i, q := range req.Queries {
		p.Go(func() { runs[i], errs[i] = d.runQuery(ctx, plan, q, req.NumResults) })
	}
	p.Wait()

	warnings := plan.warnings
	ranked := make([][]Candidate, len(req.Queries))
	var failed []error
	unresponsive := map[string]string{} // engine → reason
	for i, err := range errs {
		path := queryPath(i)
		if err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", path, err))
			warnings = append(warnings, partialResults(path, "this query failed and its results are missing: "+err.Error()))
			continue
		}
		ranked[i] = runs[i].candidates
		if runs[i].pageErr != nil {
			warnings = append(warnings, partialResults(path, fmt.Sprintf("stopped paging early: %v", runs[i].pageErr)))
		}
		for _, e := range runs[i].unresponsive {
			unresponsive[e.Name] = e.Reason
		}
	}
	if len(failed) == len(req.Queries) {
		return nil, &websearch.Error{
			Status: http.StatusServiceUnavailable, Tag: websearch.TagUpstreamError,
			Message: "search engines unavailable: " + errors.Join(failed...).Error(),
		}
	}
	if len(unresponsive) > 0 {
		warnings = append(warnings, unresponsiveWarning(unresponsive))
	}

	candidates := mergeRanked(ranked, req.NumResults)
	if w, ok := plan.filter.undatedWarning(candidates); ok {
		warnings = append(warnings, w)
	}
	return &DiscoverResult{Candidates: candidates, Warnings: warnings}, nil
}

// queryPath is the request path of the i-th query: the user's query, then
// additionalQueries and LLM expansions, which are reported alike.
func queryPath(i int) string {
	if i == 0 {
		return "query"
	}
	return fmt.Sprintf("additionalQueries[%d]", i-1)
}

// unresponsiveWarning lists the engines that failed while others answered.
func unresponsiveWarning(engines map[string]string) websearch.Warning {
	names := make([]string, 0, len(engines))
	for name := range engines {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = fmt.Sprintf("%s (%s)", name, engines[name])
	}
	return partialResults("", "results are from the other engines; these did not respond: "+strings.Join(parts, ", "))
}

func (d *SearXNGDiscoverer) plan(req *DiscoverRequest) (*discoverPlan, error) {
	plan := &discoverPlan{
		base:   searxng.Query{SafeSearch: searxng.SafeSearchOff},
		filter: candidateFilter{start: req.StartPublishedDate, end: req.EndPublishedDate},
	}
	var suffix []string

	switch req.Category {
	case "":
	case websearch.CategoryNews:
		plan.base.Categories = []string{"news"}
	default:
		rewrite, ok := categoryRewrites[req.Category]
		if !ok {
			return nil, websearch.InvalidRequest("category: unknown value %q", req.Category)
		}
		suffix = append(suffix, rewrite)
		plan.warnings = append(plan.warnings, websearch.Warning{
			Code: websearch.WarningApproximatedField, Path: "category",
			Message: fmt.Sprintf("%q is approximated by adding %q to a general web search", req.Category, rewrite),
		})
	}

	var err error
	if plan.filter.include, err = parseDomainRules("includeDomains", req.IncludeDomains); err != nil {
		return nil, err
	}
	if plan.filter.exclude, err = parseDomainRules("excludeDomains", req.ExcludeDomains); err != nil {
		return nil, err
	}
	ops, opWarnings := siteOperators(plan.filter.include, plan.filter.exclude)
	suffix = append(suffix, ops...)
	plan.warnings = append(plan.warnings, opWarnings...)
	plan.suffix = strings.Join(suffix, " ")

	if req.StartPublishedDate != nil {
		plan.base.TimeRange = timeRangeSince(d.now().Sub(*req.StartPublishedDate))
	}

	// Without a known country the query's own language is searched in;
	// SearXNG's default mixes every language in, and keyword engines then
	// return pages in unrelated languages.
	plan.base.Language = searxng.LanguageAuto
	if req.Country != "" {
		if lang, ok := countryLanguages[strings.ToUpper(req.Country)]; ok {
			plan.base.Language = lang
		} else {
			plan.warnings = append(plan.warnings, websearch.Warning{
				Code: websearch.WarningIgnoredField, Path: "userLocation.country",
				Message: fmt.Sprintf("no search locale for country %q; searching in the query's language", req.Country),
			})
		}
	}

	if req.Moderation {
		plan.base.SafeSearch = searxng.SafeSearchStrict
	}
	return plan, nil
}

// timeRangeSince returns the smallest SearXNG time range reaching back age,
// or "" when only the post-filter can apply it.
func timeRangeSince(age time.Duration) string {
	const day = 24 * time.Hour
	switch {
	case age <= day:
		return searxng.TimeRangeDay
	case age <= 7*day:
		return searxng.TimeRangeWeek
	case age <= 31*day:
		return searxng.TimeRangeMonth
	case age <= 366*day:
		return searxng.TimeRangeYear
	}
	return ""
}

type queryRun struct {
	candidates   []Candidate // filtered, deduplicated, in SearXNG rank order
	pageErr      error       // a later page failed; candidates are from earlier pages
	unresponsive []searxng.UnresponsiveEngine
}

// runQuery pages through SearXNG until numResults candidates survive the
// filter, a page adds nothing new, or discoverMaxPages is reached.
// Candidate URLs are normalised.
func (d *SearXNGDiscoverer) runQuery(ctx context.Context, plan *discoverPlan, q string, numResults int) (queryRun, error) {
	query := plan.base
	query.Q = strings.TrimSpace(q + " " + plan.suffix)
	seen := map[string]bool{}
	var run queryRun

	for page := 1; page <= discoverMaxPages && len(run.candidates) < numResults; page++ {
		query.PageNo = page
		resp, err := d.client.Search(ctx, query)
		if err != nil {
			if page == 1 {
				return queryRun{}, err
			}
			run.pageErr = fmt.Errorf("page %d: %w", page, err)
			break
		}
		run.unresponsive = append(run.unresponsive, resp.UnresponsiveEngines...)
		added := 0
		for _, r := range resp.Results {
			norm, err := websearch.NormalizeURL(r.URL)
			if err != nil {
				return queryRun{}, fmt.Errorf("result url %q: %w", r.URL, err)
			}
			if seen[norm] {
				continue
			}
			seen[norm] = true
			added++
			c := Candidate{URL: norm, Title: r.Title, Snippet: r.Content, PublishedDate: r.PublishedDate, Image: r.Thumbnail}
			if c.Image == "" {
				c.Image = r.ImgSrc
			}
			keep, err := plan.filter.keep(norm, c.PublishedDate)
			if err != nil {
				return queryRun{}, err
			}
			if keep {
				run.candidates = append(run.candidates, c)
			}
		}
		if added == 0 {
			break
		}
	}
	return run, nil
}

// mergeRanked fuses per-query rankings (index 0 is the user's query) and
// returns the top n. Each URL keeps the fields of its best-ranked
// occurrence, with a missing date or image taken from another.
func mergeRanked(ranked [][]Candidate, n int) []Candidate {
	type entry struct {
		c        Candidate
		bestRank int
		score    float64
	}
	byURL := map[string]*entry{}
	var order []*entry
	for qi, list := range ranked {
		weight := 1.0
		if qi == 0 {
			weight = userQueryWeight
		}
		for rank, c := range list {
			e, ok := byURL[c.URL]
			if !ok {
				e = &entry{c: c, bestRank: rank}
				byURL[c.URL] = e
				order = append(order, e)
			} else if rank < e.bestRank {
				prev := e.c
				e.c, e.bestRank = c, rank
				fillMissing(&e.c, prev)
			} else {
				fillMissing(&e.c, c)
			}
			e.score += weight / float64(rrfK+rank+1)
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].score > order[j].score })
	if len(order) > n {
		order = order[:n]
	}
	out := make([]Candidate, len(order))
	for i, e := range order {
		out[i] = e.c
	}
	return out
}

func fillMissing(dst *Candidate, src Candidate) {
	if dst.PublishedDate == nil {
		dst.PublishedDate = src.PublishedDate
	}
	if dst.Image == "" {
		dst.Image = src.Image
	}
}

// domainRule is one includeDomains/excludeDomains entry: `host` matches the
// host and its subdomains, `*.host` only its subdomains, and `host/path`
// the host and subdomains under that path.
type domainRule struct {
	host       string
	subdomains bool // `*.host`
	path       string
}

func parseDomainRules(field string, entries []string) ([]domainRule, error) {
	rules := make([]domainRule, 0, len(entries))
	for i, raw := range entries {
		s := strings.ToLower(strings.TrimSpace(raw))
		s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
		var r domainRule
		if rest, ok := strings.CutPrefix(s, "*."); ok {
			r.subdomains = true
			s = rest
		}
		host, path, hasPath := strings.Cut(s, "/")
		r.host = host
		if hasPath && path != "" {
			r.path = "/" + path
		}
		if r.host == "" || strings.ContainsAny(r.host, "*?#: ") {
			return nil, websearch.InvalidRequest("%s[%d]: invalid domain %q", field, i, raw)
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (r domainRule) matches(u *url.URL) bool {
	h := u.Hostname()
	hostOK := strings.HasSuffix(h, "."+r.host) || (!r.subdomains && h == r.host)
	if !hostOK || r.path == "" {
		return hostOK
	}
	p := u.Path
	return strings.HasPrefix(p, r.path) &&
		(len(p) == len(r.path) || strings.HasSuffix(r.path, "/") || p[len(r.path)] == '/')
}

// siteOperators builds the site: hints sent upstream. Every site: operator
// covers a superset of its rule, so the post-filter keeps results exact.
// Include lists are sent whole or not at all (a partial list would hide the
// rest); excludes send their plain-host entries up to the cap, since
// `-site:` cannot express a path or subdomain-only rule.
func siteOperators(include, exclude []domainRule) ([]string, []websearch.Warning) {
	var ops []string
	var warnings []websearch.Warning

	if hosts := distinctHosts(include, false); len(hosts) > siteOperatorCap {
		warnings = append(warnings, websearch.Warning{
			Code: websearch.WarningApproximatedField, Path: "includeDomains",
			Message: fmt.Sprintf("%d domains exceed the %d the search engines accept; results are filtered after searching, so fewer may match", len(hosts), siteOperatorCap),
		})
	} else if len(hosts) > 0 {
		terms := make([]string, len(hosts))
		for i, h := range hosts {
			terms[i] = "site:" + h
		}
		ops = append(ops, strings.Join(terms, " OR "))
	}

	hosts := distinctHosts(exclude, true)
	if len(hosts) > siteOperatorCap {
		warnings = append(warnings, websearch.Warning{
			Code: websearch.WarningApproximatedField, Path: "excludeDomains",
			Message: fmt.Sprintf("only the first %d of %d domains are excluded by the search engines; the rest are filtered after searching, so fewer results may return", siteOperatorCap, len(hosts)),
		})
		hosts = hosts[:siteOperatorCap]
	}
	for _, h := range hosts {
		ops = append(ops, "-site:"+h)
	}
	return ops, warnings
}

func distinctHosts(rules []domainRule, plainOnly bool) []string {
	var hosts []string
	seen := map[string]bool{}
	for _, r := range rules {
		if plainOnly && (r.subdomains || r.path != "") {
			continue
		}
		if !seen[r.host] {
			seen[r.host] = true
			hosts = append(hosts, r.host)
		}
	}
	return hosts
}

// candidateFilter enforces domain and published-date filters on results.
// A result with no date passes the date filter and is reported in a warning.
type candidateFilter struct {
	include, exclude []domainRule
	start, end       *time.Time
}

func (f candidateFilter) keep(normalizedURL string, published *time.Time) (bool, error) {
	u, err := url.Parse(normalizedURL)
	if err != nil {
		return false, err
	}
	if len(f.include) > 0 && !anyRuleMatches(f.include, u) {
		return false, nil
	}
	if anyRuleMatches(f.exclude, u) {
		return false, nil
	}
	if published != nil {
		if f.start != nil && published.Before(*f.start) {
			return false, nil
		}
		if f.end != nil && published.After(*f.end) {
			return false, nil
		}
	}
	return true, nil
}

func anyRuleMatches(rules []domainRule, u *url.URL) bool {
	for _, r := range rules {
		if r.matches(u) {
			return true
		}
	}
	return false
}

func (f candidateFilter) undatedWarning(candidates []Candidate) (websearch.Warning, bool) {
	if f.start == nil && f.end == nil {
		return websearch.Warning{}, false
	}
	undated := 0
	for _, c := range candidates {
		if c.PublishedDate == nil {
			undated++
		}
	}
	if undated == 0 {
		return websearch.Warning{}, false
	}
	path := "startPublishedDate"
	if f.start == nil {
		path = "endPublishedDate"
	}
	return websearch.Warning{
		Code: websearch.WarningApproximatedField, Path: path,
		Message: fmt.Sprintf("%d of %d results have no known published date and were not date-filtered", undated, len(candidates)),
	}, true
}
