package websearch

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
)

// Exa contract limits (OpenAPI 2.0.0).
const (
	maxNumResults           = 100
	DefaultNumResults       = 10
	maxAdditionalQueries    = 10
	maxDomains              = 1200
	maxTextFilters          = 50
	maxTextFilterChars      = 4096
	maxContentsTargets      = 100
	maxTargetChars          = 2048
	maxAgeHoursLimit        = 720
	maxLivecrawlTimeoutMs   = 90000
	maxSearchTextChars      = 1_000_000
	maxContentsTextChars    = 5_000_000
	maxSubpages             = 100
	maxExtras               = 1000
	maxOutputSchemaProps    = 10
	maxOutputSchemaNesting  = 2
	userIdentifierMaxLength = 64
)

// Livecrawl values (deprecated upstream in favour of maxAgeHours).
const (
	LivecrawlNever     = "never"
	LivecrawlFallback  = "fallback"
	LivecrawlPreferred = "preferred"
	LivecrawlAlways    = "always"
	LivecrawlAuto      = "auto"
)

var userIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// NormalizeSearchType maps the default and legacy search types onto the ones
// Helix recognises. Unknown values pass through: the enum is open upstream,
// so whether to accept one is the provider's decision.
func NormalizeSearchType(t string) string {
	switch t {
	case "", "instant", "fast":
		return SearchTypeAuto
	case "deep-reasoning":
		return SearchTypeDeep
	default:
		return t
	}
}

func isDeep(t string) bool {
	return t == SearchTypeDeep || t == SearchTypeDeepLite
}

// Validate checks req against Exa's contract. stream without outputSchema is
// valid: Exa answers it with a normal JSON response.
func (r *SearchRequest) Validate() error {
	if r.Query == "" {
		return InvalidRequest("query must not be empty")
	}
	typ := NormalizeSearchType(r.Type)
	if len(r.AdditionalQueries) > 0 {
		if !isDeep(typ) {
			return InvalidRequest("additionalQueries is only supported with type deep or deep-lite")
		}
		if len(r.AdditionalQueries) > maxAdditionalQueries {
			return InvalidRequest("additionalQueries must have at most %d items", maxAdditionalQueries)
		}
	}
	if typ == SearchTypeLite {
		if err := r.validateLite(); err != nil {
			return err
		}
	}
	if err := validateFilters(searchFilters{
		numResults: r.NumResults, category: r.Category,
		includeDomains: r.IncludeDomains, excludeDomains: r.ExcludeDomains,
		startPublished: r.StartPublishedDate != nil, endPublished: r.EndPublishedDate != nil,
		includeText: r.IncludeText, excludeText: r.ExcludeText,
	}); err != nil {
		return err
	}
	if len(r.OutputSchema) > 0 {
		if err := validateOutputSchema(r.OutputSchema); err != nil {
			return err
		}
	}
	if r.Contents != nil {
		if err := r.Contents.validate("contents.", maxSearchTextChars); err != nil {
			return err
		}
	}
	return validateUserIdentifier(r.UserIdentifier)
}

// validateLite rejects every field Exa does not accept with type lite.
func (r *SearchRequest) validateLite() error {
	type field struct {
		set  bool
		name string
	}
	fields := []field{
		{r.StartCrawlDate != nil, "startCrawlDate"},
		{r.EndCrawlDate != nil, "endCrawlDate"},
		{len(r.IncludeText) > 0, "includeText"},
		{len(r.ExcludeText) > 0, "excludeText"},
		{len(r.OutputSchema) > 0, "outputSchema"},
		{r.SystemPrompt != "", "systemPrompt"},
		{r.Stream, "stream"},
		{r.Context != nil, "context"},
	}
	if c := r.Contents; c != nil {
		fields = append(fields,
			field{c.Summary != nil, "contents.summary"},
			field{c.Extras != nil, "contents.extras"},
			field{c.Livecrawl != "", "contents.livecrawl"},
			field{c.LivecrawlTimeout != 0, "contents.livecrawlTimeout"},
			field{c.MaxAgeHours != nil, "contents.maxAgeHours"},
			field{c.CrawledBefore != "", "contents.crawledBeforeDate"},
			field{c.Subpages != 0, "contents.subpages"},
			field{len(c.SubpageTarget) > 0, "contents.subpageTarget"},
			field{c.Context != nil, "contents.context"},
		)
	}
	for _, f := range fields {
		if f.set {
			return InvalidRequest("%s is not supported with type lite", f.name)
		}
	}
	return nil
}

// Validate checks req against Exa's contract.
func (r *ContentsRequest) Validate() error {
	if (len(r.IDs) > 0) == (len(r.URLs) > 0) {
		return InvalidRequest("exactly one of ids or urls must be set")
	}
	field, targets := "urls", r.URLs
	if len(r.IDs) > 0 {
		field, targets = "ids", r.IDs
	}
	if len(targets) > maxContentsTargets {
		return InvalidRequest("%s must have at most %d items", field, maxContentsTargets)
	}
	for i, t := range targets {
		if t == "" || len(t) > maxTargetChars {
			return InvalidRequest("%s[%d] must be 1 to %d characters", field, i, maxTargetChars)
		}
	}
	if err := r.ContentsOptions.validate("", maxContentsTextChars); err != nil {
		return err
	}
	return validateUserIdentifier(r.UserIdentifier)
}

// Validate checks req against Exa's contract.
func (r *FindSimilarRequest) Validate() error {
	if r.URL == "" || len(r.URL) > maxTargetChars {
		return InvalidRequest("url must be 1 to %d characters", maxTargetChars)
	}
	if err := validateFilters(searchFilters{
		numResults: r.NumResults, category: r.Category,
		includeDomains: r.IncludeDomains, excludeDomains: r.ExcludeDomains,
		startPublished: r.StartPublishedDate != nil, endPublished: r.EndPublishedDate != nil,
		includeText: r.IncludeText, excludeText: r.ExcludeText,
	}); err != nil {
		return err
	}
	if r.Contents != nil {
		if err := r.Contents.validate("contents.", maxSearchTextChars); err != nil {
			return err
		}
	}
	return validateUserIdentifier(r.UserIdentifier)
}

var answerModels = []string{AnswerModelExa, AnswerModelExaPro, AnswerModelExaResearch, AnswerModelExaFast}

// Validate checks req against Exa's contract. outputSchema here is a full
// JSON Schema (draft 7), without /search's size limits.
func (r *AnswerRequest) Validate() error {
	if r.Query == "" {
		return InvalidRequest("query must not be empty")
	}
	if r.Model != "" && !slices.Contains(answerModels, r.Model) {
		return InvalidRequest("model must be one of %v", answerModels)
	}
	if len(r.OutputSchema) > 0 {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(r.OutputSchema, &obj); err != nil {
			return InvalidRequest("outputSchema must be a JSON object")
		}
	}
	return validateUserIdentifier(r.UserIdentifier)
}

type searchFilters struct {
	numResults                   int
	category                     string
	includeDomains               []string
	excludeDomains               []string
	startPublished, endPublished bool
	includeText, excludeText     []string
}

func validateFilters(f searchFilters) error {
	if f.numResults < 0 || f.numResults > maxNumResults {
		return InvalidRequest("numResults must be between 1 and %d", maxNumResults)
	}
	if len(f.includeDomains) > maxDomains {
		return InvalidRequest("includeDomains must have at most %d items", maxDomains)
	}
	if len(f.excludeDomains) > maxDomains {
		return InvalidRequest("excludeDomains must have at most %d items", maxDomains)
	}
	if err := validateTextFilter("includeText", f.includeText); err != nil {
		return err
	}
	if err := validateTextFilter("excludeText", f.excludeText); err != nil {
		return err
	}
	if (f.category == CategoryCompany || f.category == CategoryPeople) && (f.startPublished || f.endPublished) {
		return InvalidRequest("startPublishedDate and endPublishedDate are not supported with category %q", f.category)
	}
	if f.category == CategoryPeople && len(f.excludeDomains) > 0 {
		return InvalidRequest("excludeDomains is not supported with category %q", f.category)
	}
	return nil
}

func validateTextFilter(field string, items []string) error {
	if len(items) > maxTextFilters {
		return InvalidRequest("%s must have at most %d items", field, maxTextFilters)
	}
	for i, s := range items {
		if len(s) > maxTextFilterChars {
			return InvalidRequest("%s[%d] must be at most %d characters", field, i, maxTextFilterChars)
		}
	}
	return nil
}

func (o *ContentsOptions) validate(prefix string, maxTextChars int) error {
	if o.Livecrawl != "" {
		switch o.Livecrawl {
		case LivecrawlNever, LivecrawlFallback, LivecrawlPreferred, LivecrawlAlways, LivecrawlAuto:
		default:
			return InvalidRequest("%slivecrawl must be one of never, fallback, preferred, always, auto", prefix)
		}
		if o.MaxAgeHours != nil {
			return InvalidRequest("%slivecrawl and %smaxAgeHours cannot be used together", prefix, prefix)
		}
	}
	if o.MaxAgeHours != nil && (*o.MaxAgeHours < -1 || *o.MaxAgeHours > maxAgeHoursLimit) {
		return InvalidRequest("%smaxAgeHours must be between -1 and %d", prefix, maxAgeHoursLimit)
	}
	if o.LivecrawlTimeout < 0 || o.LivecrawlTimeout > maxLivecrawlTimeoutMs {
		return InvalidRequest("%slivecrawlTimeout must be between 0 and %d", prefix, maxLivecrawlTimeoutMs)
	}
	if o.Text != nil && (o.Text.MaxCharacters < 0 || o.Text.MaxCharacters > maxTextChars) {
		return InvalidRequest("%stext.maxCharacters must be at most %d", prefix, maxTextChars)
	}
	if h := o.Highlights; h != nil {
		if h.Dynamic && h.MaxCharacters > 0 {
			return InvalidRequest("%shighlights.dynamic and %shighlights.maxCharacters cannot be used together", prefix, prefix)
		}
		if h.MaxCharacters < 0 || h.NumSentences < 0 {
			return InvalidRequest("%shighlights.maxCharacters and numSentences must not be negative", prefix)
		}
	}
	if o.Subpages < 0 || o.Subpages > maxSubpages {
		return InvalidRequest("%ssubpages must be between 0 and %d", prefix, maxSubpages)
	}
	if e := o.Extras; e != nil {
		for name, n := range map[string]int{
			"links": e.Links, "imageLinks": e.ImageLinks, "richImageLinks": e.RichImageLinks,
			"richLinks": e.RichLinks, "codeBlocks": e.CodeBlocks,
		} {
			if n < 0 || n > maxExtras {
				return InvalidRequest("%sextras.%s must be between 0 and %d", prefix, name, maxExtras)
			}
		}
	}
	return nil
}

func validateUserIdentifier(id *string) error {
	if id != nil && !userIdentifierPattern.MatchString(*id) {
		return InvalidRequest("userIdentifier must match ^[A-Za-z0-9_-]{1,%d}$", userIdentifierMaxLength)
	}
	return nil
}

// validateOutputSchema enforces Exa's /search outputSchema limits: type text
// or object; an object has at most 10 properties in total (nested objects and
// array items included), at most 2 levels of nesting, and arrays define items.
func validateOutputSchema(raw json.RawMessage) error {
	var root schemaNode
	if err := json.Unmarshal(raw, &root); err != nil {
		return InvalidRequest("outputSchema must be a JSON object: %v", err)
	}
	switch root.Type {
	case "text":
		return nil
	case "object":
		count := 0
		if err := root.walk("outputSchema", 0, &count); err != nil {
			return err
		}
		if count > maxOutputSchemaProps {
			return InvalidRequest("outputSchema must have at most %d properties in total, has %d", maxOutputSchemaProps, count)
		}
		return nil
	default:
		return InvalidRequest(`outputSchema.type must be "text" or "object"`)
	}
}

type schemaNode struct {
	Type       string                 `json:"type"`
	Properties map[string]*schemaNode `json:"properties"`
	Items      *schemaNode            `json:"items"`
}

// walk counts the properties under n. depth is the nesting level of n's own
// properties minus one: the root's properties are level 1.
func (n *schemaNode) walk(path string, depth int, count *int) error {
	if n.Type == "array" {
		if n.Items == nil {
			return InvalidRequest("%s: arrays must define items", path)
		}
		return n.Items.walk(path+".items", depth, count)
	}
	if len(n.Properties) == 0 {
		return nil
	}
	if depth >= maxOutputSchemaNesting {
		return InvalidRequest("%s: outputSchema allows at most %d levels of nesting", path, maxOutputSchemaNesting)
	}
	for name, p := range n.Properties {
		if p == nil {
			return InvalidRequest("%s.properties.%s must be an object", path, name)
		}
		*count++
		if err := p.walk(fmt.Sprintf("%s.properties.%s", path, name), depth+1, count); err != nil {
			return err
		}
	}
	return nil
}
