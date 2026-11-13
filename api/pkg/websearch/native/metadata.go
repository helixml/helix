package native

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type pageMetadata struct {
	title       string
	description string
	author      string
	published   *time.Time
	image       string
	favicon     string
	siteName    string
	keywords    string
}

// metadata reads the whole document; readability's byline is the last
// resort for the author.
func (h *htmlPage) metadata() (pageMetadata, error) {
	meta := h.metaTags()
	ld := h.jsonLD()

	m := pageMetadata{
		title: firstNonEmpty(
			meta["og:title"],
			collapseSpace(h.doc.Find("title").First().Text()),
			meta["twitter:title"],
			ld.str("headline"),
			collapseSpace(h.doc.Find("h1").First().Text()),
		),
		description: firstNonEmpty(meta["description"], meta["og:description"], ld.str("description")),
		author: firstNonEmpty(
			meta["author"],
			notURL(meta["article:author"]),
			ld.author(),
			notURL(meta["twitter:creator"]),
		),
		siteName: meta["og:site_name"],
		keywords: meta["keywords"],
		image: firstNonEmpty(
			h.resolve(meta["og:image"]),
			h.resolve(meta["og:image:url"]),
			h.resolve(meta["og:image:secure_url"]),
			h.resolve(meta["twitter:image"]),
			h.resolve(meta["twitter:image:src"]),
			h.resolve(ld.image()),
		),
		favicon: h.favicon(),
	}

	for _, raw := range []string{
		meta["article:published_time"],
		meta["datepublished"],
		ld.str("datePublished"),
		meta["date"], meta["pubdate"], meta["publishdate"], meta["publish-date"],
		meta["dc.date"], meta["dc.date.issued"], meta["dcterms.created"],
		meta["parsely-pub-date"], meta["sailthru.date"],
		h.articleTime(),
	} {
		if t := parseDate(raw); t != nil {
			m.published = t
			break
		}
	}
	if m.author == "" {
		article, err := h.readable()
		if err != nil {
			return pageMetadata{}, err
		}
		m.author = strings.TrimSpace(article.Byline)
	}
	return m, nil
}

// metaTags maps lowercased name / property / itemprop to the first content.
func (h *htmlPage) metaTags() map[string]string {
	tags := map[string]string{}
	h.doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		content := strings.TrimSpace(s.AttrOr("content", s.AttrOr("datetime", "")))
		if content == "" {
			return
		}
		for _, attr := range []string{"name", "property", "itemprop"} {
			key := strings.ToLower(strings.TrimSpace(s.AttrOr(attr, "")))
			if _, seen := tags[key]; key != "" && !seen {
				tags[key] = content
			}
		}
	})
	return tags
}

// articleTime is the first <time datetime> inside the article, preferring
// one marked as the publication date.
func (h *htmlPage) articleTime() string {
	scope := h.doc.Find("article, main, [role=main]")
	if scope.Length() == 0 {
		return ""
	}
	marked := scope.Find("time[datetime][pubdate], time[datetime][itemprop=datePublished]").First()
	if marked.Length() > 0 {
		return marked.AttrOr("datetime", "")
	}
	return scope.Find("time[datetime]").First().AttrOr("datetime", "")
}

func (h *htmlPage) favicon() string {
	var icon, touchIcon string
	h.doc.Find("link[rel][href]").Each(func(_ int, s *goquery.Selection) {
		href := h.resolve(s.AttrOr("href", ""))
		if href == "" {
			return
		}
		for rel := range strings.FieldsSeq(strings.ToLower(s.AttrOr("rel", ""))) {
			switch {
			case rel == "icon" && icon == "":
				icon = href
			case strings.HasPrefix(rel, "apple-touch-icon") && touchIcon == "":
				touchIcon = href
			}
		}
	})
	return firstNonEmpty(icon, touchIcon, defaultFavicon(h.pageURL))
}

func defaultFavicon(pageURL *url.URL) string {
	return pageURL.Scheme + "://" + pageURL.Host + "/favicon.ico"
}

// jsonLDObjects is every JSON-LD object on the page, article-typed first.
type jsonLDObjects []map[string]any

var articleTypes = []string{"Article", "NewsArticle", "BlogPosting", "Report", "ScholarlyArticle", "TechArticle", "WebPage"}

func (h *htmlPage) jsonLD() jsonLDObjects {
	var objects []map[string]any
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case []any:
			for _, item := range v {
				walk(item)
			}
		case map[string]any:
			objects = append(objects, v)
			walk(v["@graph"])
		}
	}
	h.doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, s *goquery.Selection) {
		var v any
		// Malformed JSON-LD is common and only costs us metadata.
		if json.Unmarshal([]byte(s.Text()), &v) == nil {
			walk(v)
		}
	})
	isArticle := func(o map[string]any) bool {
		for _, t := range stringsOf(o["@type"]) {
			if slices.Contains(articleTypes, t) {
				return true
			}
		}
		return false
	}
	slices.SortStableFunc(objects, func(a, b map[string]any) int {
		switch aa, ba := isArticle(a), isArticle(b); {
		case aa && !ba:
			return -1
		case ba && !aa:
			return 1
		}
		return 0
	})
	return objects
}

func (ld jsonLDObjects) str(key string) string {
	for _, o := range ld {
		if s, ok := o[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func (ld jsonLDObjects) author() string {
	for _, o := range ld {
		var names []string
		for _, a := range listOf(o["author"]) {
			switch a := a.(type) {
			case string:
				names = append(names, notURL(a))
			case map[string]any:
				name, _ := a["name"].(string)
				names = append(names, strings.TrimSpace(name))
			}
		}
		names = slices.DeleteFunc(names, func(s string) bool { return s == "" })
		if len(names) > 0 {
			return strings.Join(names, ", ")
		}
	}
	return ""
}

func (ld jsonLDObjects) image() string {
	for _, o := range ld {
		for _, img := range listOf(o["image"]) {
			switch img := img.(type) {
			case string:
				return img
			case map[string]any:
				if u, ok := img["url"].(string); ok {
					return u
				}
			}
		}
	}
	return ""
}

func listOf(v any) []any {
	switch v := v.(type) {
	case nil:
		return nil
	case []any:
		return v
	default:
		return []any{v}
	}
}

func stringsOf(v any) []string {
	var out []string
	for _, item := range listOf(v) {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z0700",
	"2006-01-02T15:04Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"2006/01/02",
	"20060102",
	time.RFC1123Z,
	time.RFC1123,
	time.RFC850,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"January 2, 2006",
	"Jan 2, 2006",
	"2 January 2006",
	"2 Jan 2006",
}

// parseDate parses the date formats found in page metadata; nil when none fit.
func parseDate(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}

// notURL drops values that are profile links rather than names.
func notURL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return ""
	}
	return s
}
