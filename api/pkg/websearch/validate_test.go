package websearch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type validator interface{ Validate() error }

func decode[T any](t *testing.T, body string) *T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal([]byte(body), &v), body)
	return &v
}

func list(n int, item string) string {
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf("%q", item)
	}
	return "[" + strings.Join(items, ",") + "]"
}

// schemaWithProps returns an object schema with n flat string properties.
func schemaWithProps(n int) string {
	props := make([]string, n)
	for i := range props {
		props[i] = fmt.Sprintf(`"p%d":{"type":"string"}`, i)
	}
	return `{"type":"object","properties":{` + strings.Join(props, ",") + `}}`
}

func TestValidate(t *testing.T) {
	long := strings.Repeat("a", 4097)
	url2049 := "https://x.com/" + strings.Repeat("a", 2035)
	tests := []struct {
		name    string
		req     validator
		wantErr string // "" = valid
	}{
		// /search
		{"search minimal", decode[SearchRequest](t, `{"query":"q"}`), ""},
		{"search empty query", decode[SearchRequest](t, `{"query":""}`), "query must not be empty"},
		{"search numResults 100", decode[SearchRequest](t, `{"query":"q","numResults":100}`), ""},
		{"search numResults 101", decode[SearchRequest](t, `{"query":"q","numResults":101}`), "numResults"},
		{"search numResults negative", decode[SearchRequest](t, `{"query":"q","numResults":-1}`), "numResults"},
		{"additionalQueries with deep", decode[SearchRequest](t, `{"query":"q","type":"deep","additionalQueries":["a"]}`), ""},
		{"additionalQueries with deep-reasoning", decode[SearchRequest](t, `{"query":"q","type":"deep-reasoning","additionalQueries":["a"]}`), ""},
		{"additionalQueries with deep-lite", decode[SearchRequest](t, `{"query":"q","type":"deep-lite","additionalQueries":["a"]}`), ""},
		{"additionalQueries with auto", decode[SearchRequest](t, `{"query":"q","additionalQueries":["a"]}`), "only supported with type deep"},
		{"additionalQueries 11", decode[SearchRequest](t, `{"query":"q","type":"deep","additionalQueries":`+list(11, "a")+`}`), "at most 10"},
		{"includeDomains 1200", decode[SearchRequest](t, `{"query":"q","includeDomains":`+list(1200, "a.com")+`}`), ""},
		{"includeDomains 1201", decode[SearchRequest](t, `{"query":"q","includeDomains":`+list(1201, "a.com")+`}`), "includeDomains"},
		{"excludeDomains 1201", decode[SearchRequest](t, `{"query":"q","excludeDomains":`+list(1201, "a.com")+`}`), "excludeDomains"},
		{"includeText 51", decode[SearchRequest](t, `{"query":"q","includeText":`+list(51, "a")+`}`), "includeText"},
		{"excludeText too long", decode[SearchRequest](t, `{"query":"q","excludeText":["`+long+`"]}`), "excludeText[0]"},
		{"company with published date", decode[SearchRequest](t, `{"query":"q","category":"company","startPublishedDate":"2025-01-01T00:00:00Z"}`), "not supported with category"},
		{"people with end date", decode[SearchRequest](t, `{"query":"q","category":"people","endPublishedDate":"2025-01-01T00:00:00Z"}`), "not supported with category"},
		{"news with dates", decode[SearchRequest](t, `{"query":"q","category":"news","startPublishedDate":"2025-01-01T00:00:00Z"}`), ""},
		{"people excludeDomains", decode[SearchRequest](t, `{"query":"q","category":"people","excludeDomains":["a.com"]}`), "excludeDomains is not supported"},
		{"company excludeDomains", decode[SearchRequest](t, `{"query":"q","category":"company","excludeDomains":["a.com"]}`), ""},
		{"stream without outputSchema", decode[SearchRequest](t, `{"query":"q","stream":true}`), ""},
		{"userIdentifier ok", decode[SearchRequest](t, `{"query":"q","userIdentifier":"user_1-A"}`), ""},
		{"userIdentifier bad chars", decode[SearchRequest](t, `{"query":"q","userIdentifier":"a b"}`), "userIdentifier"},
		{"userIdentifier empty", decode[SearchRequest](t, `{"query":"q","userIdentifier":""}`), "userIdentifier"},
		{"userIdentifier too long", decode[SearchRequest](t, `{"query":"q","userIdentifier":"`+strings.Repeat("a", 65)+`"}`), "userIdentifier"},

		// outputSchema
		{"schema text", decode[SearchRequest](t, `{"query":"q","outputSchema":{"type":"text","description":"x"}}`), ""},
		{"schema 10 props", decode[SearchRequest](t, `{"query":"q","outputSchema":`+schemaWithProps(10)+`}`), ""},
		{"schema 11 props", decode[SearchRequest](t, `{"query":"q","outputSchema":`+schemaWithProps(11)+`}`), "at most 10 properties"},
		{"schema nested counts", decode[SearchRequest](t, `{"query":"q","outputSchema":{"type":"object","properties":{
			"a":{"type":"object","properties":`+strings.TrimSuffix(strings.TrimPrefix(schemaWithProps(9), `{"type":"object","properties":`), "}")+`},
			"b":{"type":"string"}}}}`), "at most 10 properties"},
		{"schema array items count", decode[SearchRequest](t, `{"query":"q","outputSchema":{"type":"object","properties":{
			"list":{"type":"array","items":`+schemaWithProps(10)+`}}}}`), "at most 10 properties"},
		{"schema two levels", decode[SearchRequest](t, `{"query":"q","outputSchema":{"type":"object","properties":{
			"a":{"type":"object","properties":{"b":{"type":"string"}}}}}}`), ""},
		{"schema three levels", decode[SearchRequest](t, `{"query":"q","outputSchema":{"type":"object","properties":{
			"a":{"type":"object","properties":{"b":{"type":"object","properties":{"c":{"type":"string"}}}}}}}}`), "levels of nesting"},
		{"schema array of objects is a level", decode[SearchRequest](t, `{"query":"q","outputSchema":{"type":"object","properties":{
			"a":{"type":"array","items":{"type":"object","properties":{"b":{"type":"object","properties":{"c":{"type":"string"}}}}}}}}}`), "levels of nesting"},
		{"schema array without items", decode[SearchRequest](t, `{"query":"q","outputSchema":{"type":"object","properties":{"a":{"type":"array"}}}}`), "arrays must define items"},
		{"schema bad type", decode[SearchRequest](t, `{"query":"q","outputSchema":{"type":"string"}}`), "outputSchema.type"},
		{"schema not an object", decode[SearchRequest](t, `{"query":"q","outputSchema":[1]}`), "outputSchema must be a JSON object"},

		// lite
		{"lite allowed fields", decode[SearchRequest](t, `{"query":"q","type":"lite","objective":"o","userIdentifier":"u","category":"news","numResults":5,
			"contents":{"highlights":true,"text":true,"snapshotAsOf":"2025-01-01"},"userLocation":"US","includeDomains":["a.com"],"excludeDomains":["b.com"],
			"startPublishedDate":"2025-01-01T00:00:00Z","endPublishedDate":"2025-02-01T00:00:00Z","moderation":true,"compliance":"hipaa"}`), ""},
		{"lite outputSchema", decode[SearchRequest](t, `{"query":"q","type":"lite","outputSchema":{"type":"text"}}`), "outputSchema is not supported with type lite"},
		{"lite systemPrompt", decode[SearchRequest](t, `{"query":"q","type":"lite","systemPrompt":"s"}`), "systemPrompt"},
		{"lite summary", decode[SearchRequest](t, `{"query":"q","type":"lite","contents":{"summary":{}}}`), "contents.summary"},
		{"lite summary true", decode[SearchRequest](t, `{"query":"q","type":"lite","contents":{"summary":true}}`), "contents.summary"},
		{"lite summary false", decode[SearchRequest](t, `{"query":"q","type":"lite","contents":{"summary":false}}`), ""},
		{"lite context false", decode[SearchRequest](t, `{"query":"q","type":"lite","context":false,"contents":{"context":false}}`), ""},
		{"lite maxAgeHours", decode[SearchRequest](t, `{"query":"q","type":"lite","contents":{"maxAgeHours":1}}`), "contents.maxAgeHours"},
		{"lite crawl date", decode[SearchRequest](t, `{"query":"q","type":"lite","startCrawlDate":"2025-01-01T00:00:00Z"}`), "startCrawlDate"},

		// contents inside /search
		{"search text 1M", decode[SearchRequest](t, `{"query":"q","contents":{"text":{"maxCharacters":1000000}}}`), ""},
		{"search text over 1M", decode[SearchRequest](t, `{"query":"q","contents":{"text":{"maxCharacters":1000001}}}`), "contents.text.maxCharacters"},
		{"search livecrawl with maxAgeHours", decode[SearchRequest](t, `{"query":"q","contents":{"livecrawl":"always","maxAgeHours":1}}`), "cannot be used together"},

		// /contents
		{"contents urls", decode[ContentsRequest](t, `{"urls":["https://a.com"]}`), ""},
		{"contents ids", decode[ContentsRequest](t, `{"ids":["doc_1"]}`), ""},
		{"contents neither", decode[ContentsRequest](t, `{}`), "exactly one of ids or urls"},
		{"contents both", decode[ContentsRequest](t, `{"ids":["doc_1"],"urls":["https://a.com"]}`), "exactly one of ids or urls"},
		{"contents 100 urls", decode[ContentsRequest](t, `{"urls":`+list(100, "https://a.com")+`}`), ""},
		{"contents 101 urls", decode[ContentsRequest](t, `{"urls":`+list(101, "https://a.com")+`}`), "at most 100"},
		{"contents url too long", decode[ContentsRequest](t, `{"urls":["`+url2049+`"]}`), "urls[0]"},
		{"contents empty id", decode[ContentsRequest](t, `{"ids":[""]}`), "ids[0]"},
		{"contents livecrawl+maxAge", decode[ContentsRequest](t, `{"urls":["https://a.com"],"livecrawl":"never","maxAgeHours":0}`), "cannot be used together"},
		{"contents livecrawl unknown", decode[ContentsRequest](t, `{"urls":["https://a.com"],"livecrawl":"sometimes"}`), "livecrawl must be one of"},
		{"contents maxAgeHours -1", decode[ContentsRequest](t, `{"urls":["https://a.com"],"maxAgeHours":-1}`), ""},
		{"contents text options", decode[ContentsRequest](t, `{"urls":["https://a.com"],"text":{"verbosity":"full","linkStyle":"citation","includeSections":["body"],"excludeSections":["footer"]}}`), ""},
		{"contents text verbosity unknown", decode[ContentsRequest](t, `{"urls":["https://a.com"],"text":{"verbosity":"verbose"}}`), "text.verbosity must be one of"},
		{"contents text linkStyle unknown", decode[ContentsRequest](t, `{"urls":["https://a.com"],"text":{"linkStyle":"footnote"}}`), "text.linkStyle must be one of"},
		{"contents text includeSections unknown", decode[ContentsRequest](t, `{"urls":["https://a.com"],"text":{"includeSections":["body","main"]}}`), "text.includeSections[1]"},
		{"contents text excludeSections unknown", decode[ContentsRequest](t, `{"urls":["https://a.com"],"text":{"excludeSections":["ads"]}}`), "text.excludeSections[0]"},
		{"search text verbosity unknown", decode[SearchRequest](t, `{"query":"q","contents":{"text":{"verbosity":"verbose"}}}`), "contents.text.verbosity"},
		{"contents maxAgeHours 720", decode[ContentsRequest](t, `{"urls":["https://a.com"],"maxAgeHours":720}`), ""},
		{"contents maxAgeHours -2", decode[ContentsRequest](t, `{"urls":["https://a.com"],"maxAgeHours":-2}`), "maxAgeHours"},
		{"contents maxAgeHours 721", decode[ContentsRequest](t, `{"urls":["https://a.com"],"maxAgeHours":721}`), "maxAgeHours"},
		{"contents livecrawlTimeout 90000", decode[ContentsRequest](t, `{"urls":["https://a.com"],"livecrawlTimeout":90000}`), ""},
		{"contents livecrawlTimeout 90001", decode[ContentsRequest](t, `{"urls":["https://a.com"],"livecrawlTimeout":90001}`), "livecrawlTimeout"},
		{"contents text 5M", decode[ContentsRequest](t, `{"urls":["https://a.com"],"text":{"maxCharacters":5000000}}`), ""},
		{"contents text over 5M", decode[ContentsRequest](t, `{"urls":["https://a.com"],"text":{"maxCharacters":5000001}}`), "text.maxCharacters"},
		{"contents dynamic+maxCharacters", decode[ContentsRequest](t, `{"urls":["https://a.com"],"highlights":{"dynamic":true,"maxCharacters":100}}`), "cannot be used together"},
		{"contents subpages 100", decode[ContentsRequest](t, `{"urls":["https://a.com"],"subpages":100}`), ""},
		{"contents subpages 101", decode[ContentsRequest](t, `{"urls":["https://a.com"],"subpages":101}`), "subpages"},
		{"contents extras 1000", decode[ContentsRequest](t, `{"urls":["https://a.com"],"extras":{"links":1000,"imageLinks":1000}}`), ""},
		{"contents extras 1001", decode[ContentsRequest](t, `{"urls":["https://a.com"],"extras":{"richImageLinks":1001}}`), "extras.richImageLinks"},
		{"contents userIdentifier", decode[ContentsRequest](t, `{"urls":["https://a.com"],"userIdentifier":"no!"}`), "userIdentifier"},

		// /findSimilar
		{"findSimilar ok", decode[FindSimilarRequest](t, `{"url":"https://a.com","numResults":5}`), ""},
		{"findSimilar no url", decode[FindSimilarRequest](t, `{}`), "url must be"},
		{"findSimilar numResults", decode[FindSimilarRequest](t, `{"url":"https://a.com","numResults":101}`), "numResults"},
		{"findSimilar people excludeDomains", decode[FindSimilarRequest](t, `{"url":"https://a.com","category":"people","excludeDomains":["x.com"]}`), "excludeDomains"},
		{"findSimilar contents text limit", decode[FindSimilarRequest](t, `{"url":"https://a.com","contents":{"text":{"maxCharacters":1000001}}}`), "contents.text.maxCharacters"},

		// /answer
		{"answer ok", decode[AnswerRequest](t, `{"query":"q","model":"exa-pro"}`), ""},
		{"answer default model", decode[AnswerRequest](t, `{"query":"q"}`), ""},
		{"answer empty query", decode[AnswerRequest](t, `{"query":""}`), "query"},
		{"answer bad model", decode[AnswerRequest](t, `{"query":"q","model":"gpt-4"}`), "model must be one of"},
		{"answer schema object", decode[AnswerRequest](t, `{"query":"q","outputSchema":{"type":"object","properties":{"a":{"type":"string"}}}}`), ""},
		{"answer schema not object", decode[AnswerRequest](t, `{"query":"q","outputSchema":"x"}`), "outputSchema"},
		{"answer userIdentifier", decode[AnswerRequest](t, `{"query":"q","userIdentifier":"a/b"}`), "userIdentifier"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			e := AsError(err)
			assert.Equal(t, http.StatusBadRequest, e.Status)
			assert.Equal(t, TagInvalidRequest, e.Tag)
			assert.Contains(t, e.Message, tc.wantErr)
		})
	}
}

func TestNormalizeSearchType(t *testing.T) {
	for in, want := range map[string]string{
		"": "auto", "auto": "auto", "instant": "auto", "fast": "auto",
		"deep-reasoning": "deep", "deep": "deep", "deep-lite": "deep-lite", "lite": "lite",
		"neural": "neural",
	} {
		assert.Equal(t, want, NormalizeSearchType(in), in)
	}
}
