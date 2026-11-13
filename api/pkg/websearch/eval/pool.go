package eval

import (
	"sort"
	"strings"

	"github.com/helixml/helix/api/pkg/websearch"
)

// Doc is one pooled page as the judge sees it: no provider, no rank.
type Doc struct {
	Key   string
	URL   string
	Title string
	Text  string
}

// PageKey identifies a page across providers: websearch.NormalizeURL, then
// without scheme, "www." or a trailing slash, which providers spell
// differently for the same page. Unparseable URLs are keyed as written.
func PageKey(rawURL string) string {
	u, err := websearch.NormalizeURL(rawURL)
	if err != nil {
		return strings.TrimSpace(rawURL)
	}
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	u = strings.TrimPrefix(u, "www.")
	return strings.TrimSuffix(u, "/")
}

// Pool merges the providers' top results for one query into the unique pages
// to judge, ordered by key so the pool does not reveal who returned what.
// When providers return the same page, the judge gets the longest text.
func Pool(runs ...QueryRun) []Doc {
	byKey := map[string]Doc{}
	for _, qr := range runs {
		for _, h := range qr.Hits {
			key := PageKey(h.URL)
			if d, ok := byKey[key]; ok && len(d.Text) >= len(h.Text) {
				continue
			}
			byKey[key] = Doc{Key: key, URL: h.URL, Title: h.Title, Text: h.Text}
		}
	}
	docs := make([]Doc, 0, len(byKey))
	for _, d := range byKey {
		docs = append(docs, d)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Key < docs[j].Key })
	return docs
}
