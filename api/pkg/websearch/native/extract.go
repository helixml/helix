package native

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/JohannesKaufmann/html-to-markdown/plugin"
	"github.com/PuerkitoBio/goquery"
	readability "github.com/go-shiori/go-readability"
	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/charset"

	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/websearch"
)

// ErrUnsupportedDocument means the page's content type cannot be extracted.
// Callers report it as websearch.TagUnsupportedDocument.
var ErrUnsupportedDocument = errors.New("unsupported document type")

// Page sections accepted by TextOptions.IncludeSections and ExcludeSections.
const (
	SectionHeader     = "header"
	SectionNavigation = "navigation"
	SectionBanner     = "banner"
	SectionBody       = "body"
	SectionSidebar    = "sidebar"
	SectionFooter     = "footer"
	SectionMetadata   = "metadata"
)

var allSections = []string{
	SectionHeader, SectionNavigation, SectionBanner, SectionBody,
	SectionSidebar, SectionFooter, SectionMetadata,
}

// Text verbosity levels:
//   - compact: readability's main content, found within the body section.
//   - standard: the whole <body> minus header, navigation, banner, sidebar
//     and footer, so it keeps tables, captions and asides-in-article that
//     readability drops.
//   - full: every section of <body> (not the <head> metadata).
const (
	VerbosityCompact  = "compact"
	VerbosityStandard = "standard"
	VerbosityFull     = "full"
)

const (
	LinkStyleFull     = "full"
	LinkStyleCitation = "citation"
)

// PageExtractor implements Extractor: HTML through readability, the DOM and
// html-to-markdown; PDFs and office documents through pkg/extract.
type PageExtractor struct {
	documents extract.Extractor
}

var _ Extractor = (*PageExtractor)(nil)

func NewExtractor(documents extract.Extractor) *PageExtractor {
	return &PageExtractor{documents: documents}
}

type docKind int

const (
	kindHTML docKind = iota + 1
	kindDocument
	kindText
)

func (e *PageExtractor) Extract(ctx context.Context, page *RawPage, opts websearch.TextOptions) (*Page, error) {
	pageURL, err := pageURLOf(page)
	if err != nil {
		return nil, err
	}
	kind, err := classify(page)
	if err != nil {
		return nil, err
	}
	switch kind {
	case kindHTML:
		return extractHTML(page, pageURL, opts)
	case kindDocument:
		return e.extractDocument(ctx, page, pageURL)
	default:
		return extractPlainText(page, pageURL)
	}
}

func pageURLOf(page *RawPage) (*url.URL, error) {
	raw := page.FinalURL
	if raw == "" {
		raw = page.URL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("page has no absolute http(s) URL: %q", raw)
	}
	return u, nil
}

// classify trusts a specific Content-Type and sniffs the body only when the
// type is missing or generic.
func classify(page *RawPage) (docKind, error) {
	mediaType := ""
	if page.ContentType != "" {
		mt, _, err := mime.ParseMediaType(page.ContentType)
		// A malformed parameter (e.g. "charset") still leaves a usable type.
		if err != nil && !errors.Is(err, mime.ErrInvalidMediaParameter) {
			return 0, fmt.Errorf("%w: invalid content type %q", ErrUnsupportedDocument, page.ContentType)
		}
		mediaType = mt
	}
	if mediaType == "" || mediaType == "application/octet-stream" || mediaType == "binary/octet-stream" {
		mediaType = sniffMediaType(page.Body)
	}
	switch {
	case mediaType == "text/html" || mediaType == "application/xhtml+xml":
		return kindHTML, nil
	case isDocumentType(mediaType):
		return kindDocument, nil
	case mediaType == "text/plain" || mediaType == "text/markdown":
		return kindText, nil
	}
	return 0, fmt.Errorf("%w: %s", ErrUnsupportedDocument, mediaType)
}

var documentTypes = []string{
	"application/pdf",
	"application/msword",
	"application/vnd.ms-excel",
	"application/vnd.ms-powerpoint",
	"application/rtf",
	"text/rtf",
	"application/x-ole-storage",
}

func isDocumentType(mediaType string) bool {
	return slices.Contains(documentTypes, mediaType) ||
		strings.HasPrefix(mediaType, "application/vnd.openxmlformats-officedocument.") ||
		strings.HasPrefix(mediaType, "application/vnd.oasis.opendocument.")
}

var oleMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

func sniffMediaType(body []byte) string {
	switch {
	case bytes.HasPrefix(body, []byte("%PDF-")):
		return "application/pdf"
	case bytes.HasPrefix(body, oleMagic):
		return "application/x-ole-storage"
	case bytes.HasPrefix(body, []byte("PK\x03\x04")):
		if isOfficeZip(body) {
			return "application/vnd.openxmlformats-officedocument.unknown"
		}
		return "application/zip"
	}
	mt, _, _ := mime.ParseMediaType(http.DetectContentType(body))
	return mt
}

// isOfficeZip recognises OOXML (docx/xlsx/pptx) and ODF archives.
func isOfficeZip(body []byte) bool {
	r, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return false
	}
	for _, f := range r.File {
		if f.Name == "content.xml" || strings.HasPrefix(f.Name, "word/") ||
			strings.HasPrefix(f.Name, "xl/") || strings.HasPrefix(f.Name, "ppt/") {
			return true
		}
	}
	return false
}

func (e *PageExtractor) extractDocument(ctx context.Context, page *RawPage, pageURL *url.URL) (*Page, error) {
	if e.documents == nil {
		return nil, fmt.Errorf("document extraction is not configured")
	}
	// Content only: passing the URL would make the extraction service fetch
	// it itself, bypassing the guarded fetcher.
	text, err := e.documents.Extract(ctx, &extract.Request{Content: page.Body})
	if err != nil {
		return nil, fmt.Errorf("extract document: %w", err)
	}
	text = strings.TrimSpace(text)
	return &Page{Title: firstLine(text), Text: text, Favicon: defaultFavicon(pageURL)}, nil
}

func extractPlainText(page *RawPage, pageURL *url.URL) (*Page, error) {
	r, err := charset.NewReader(bytes.NewReader(page.Body), page.ContentType)
	if err != nil {
		return nil, fmt.Errorf("decode text: %w", err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		return nil, fmt.Errorf("decode text: %w", err)
	}
	text := strings.TrimSpace(buf.String())
	return &Page{Title: firstLine(text), Text: text, Favicon: defaultFavicon(pageURL)}, nil
}

func firstLine(text string) string {
	for line := range strings.SplitSeq(text, "\n") {
		if line = strings.TrimSpace(strings.TrimLeft(line, "# ")); line != "" {
			return line
		}
	}
	return ""
}

// htmlPage is a parsed HTML document with every a[href] and img[src]
// already resolved against the document base URL.
type htmlPage struct {
	doc     *goquery.Document
	pageURL *url.URL
	base    *url.URL
	meta    pageMetadata
	article *readability.Article
}

func extractHTML(page *RawPage, pageURL *url.URL, opts websearch.TextOptions) (*Page, error) {
	r, err := charset.NewReader(bytes.NewReader(page.Body), page.ContentType)
	if err != nil {
		return nil, fmt.Errorf("decode html: %w", err)
	}
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	h := &htmlPage{doc: doc, pageURL: pageURL, base: documentBase(doc, pageURL)}
	h.absolutize()
	if h.meta, err = h.metadata(); err != nil {
		return nil, err
	}
	text, err := h.text(opts)
	if err != nil {
		return nil, err
	}
	return &Page{
		Title:         h.meta.title,
		Author:        h.meta.author,
		PublishedDate: h.meta.published,
		Image:         h.meta.image,
		Favicon:       h.meta.favicon,
		Text:          text,
		Links:         h.links(),
		ImageLinks:    h.imageLinks(),
	}, nil
}

func documentBase(doc *goquery.Document, pageURL *url.URL) *url.URL {
	if href, ok := doc.Find("base[href]").First().Attr("href"); ok {
		if u, err := pageURL.Parse(strings.TrimSpace(href)); err == nil {
			return u
		}
	}
	return pageURL
}

// resolve returns ref as an absolute URL, or "" when it is not http(s).
func (h *htmlPage) resolve(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	u, err := h.base.Parse(ref)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.String()
}

func (h *htmlPage) absolutize() {
	for _, attr := range []struct{ sel, name string }{
		{"a[href]", "href"}, {"img[src]", "src"}, {"link[href]", "href"},
	} {
		h.doc.Find(attr.sel).Each(func(_ int, s *goquery.Selection) {
			if abs := h.resolve(s.AttrOr(attr.name, "")); abs != "" {
				s.SetAttr(attr.name, abs)
			}
		})
	}
}

// readable runs readability once per page.
func (h *htmlPage) readable() (*readability.Article, error) {
	if h.article != nil {
		return h.article, nil
	}
	// Readability runs on the body section alone so it cannot pick a cookie
	// notice or a link list as the main content.
	doc := h.doc.Selection.Clone()
	if body := doc.Find("body"); body.Length() > 0 {
		prune(body.Get(0), true, sectionFilter([]string{SectionBody}, nil))
	}
	article, err := readability.FromDocument(doc.Get(0), h.pageURL)
	if err != nil {
		return nil, fmt.Errorf("readability: %w", err)
	}
	h.article = &article
	return h.article, nil
}

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

// links is every http(s) link on the page, deduplicated by URL without its
// fragment, in document order.
func (h *htmlPage) links() []Link {
	var links []Link
	index := map[string]int{}
	h.doc.Find("body a[href]").Each(func(_ int, s *goquery.Selection) {
		u, err := url.Parse(s.AttrOr("href", ""))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return
		}
		u.Fragment = ""
		key := u.String()
		anchor := firstNonEmpty(
			collapseSpace(s.Text()),
			strings.TrimSpace(s.AttrOr("aria-label", "")),
			strings.TrimSpace(s.AttrOr("title", "")),
			strings.TrimSpace(s.Find("img[alt]").First().AttrOr("alt", "")),
		)
		if i, seen := index[key]; seen {
			if links[i].Anchor == "" {
				links[i].Anchor = anchor
			}
			return
		}
		index[key] = len(links)
		links = append(links, Link{URL: key, Anchor: anchor})
	})
	return links
}

func (h *htmlPage) imageLinks() []websearch.ImageLink {
	var images []websearch.ImageLink
	seen := map[string]bool{}
	h.doc.Find("body img[src]").Each(func(_ int, s *goquery.Selection) {
		src := h.resolve(s.AttrOr("src", ""))
		if src == "" || seen[src] {
			return
		}
		seen[src] = true
		images = append(images, websearch.ImageLink{URL: src, Alt: collapseSpace(s.AttrOr("alt", ""))})
	})
	return images
}

// text renders the page text per opts. Sections, when given, select parts of
// the full document and override verbosity.
func (h *htmlPage) text(opts websearch.TextOptions) (string, error) {
	if opts.LinkStyle != "" && opts.LinkStyle != LinkStyleFull && opts.LinkStyle != LinkStyleCitation {
		return "", fmt.Errorf("unknown link style %q", opts.LinkStyle)
	}
	for _, name := range slices.Concat(opts.IncludeSections, opts.ExcludeSections) {
		if !slices.Contains(allSections, name) {
			return "", fmt.Errorf("unknown section %q", name)
		}
	}

	var content *goquery.Selection
	var err error
	switch {
	case len(opts.IncludeSections) > 0 || len(opts.ExcludeSections) > 0:
		include := opts.IncludeSections
		if len(include) == 0 {
			include = allSections
		}
		content = h.sections(include, opts.ExcludeSections)
	case opts.Verbosity == "" || opts.Verbosity == VerbosityCompact:
		content, err = h.readableContent()
	case opts.Verbosity == VerbosityStandard:
		content = h.sections([]string{SectionBody}, nil)
	case opts.Verbosity == VerbosityFull:
		content = h.sections(allSections, []string{SectionMetadata})
	default:
		return "", fmt.Errorf("unknown verbosity %q", opts.Verbosity)
	}
	if err != nil {
		return "", err
	}

	clean(content)
	if opts.IncludeHTMLTags {
		stripAttributes(content)
		out, err := content.Html()
		if err != nil {
			return "", fmt.Errorf("render html: %w", err)
		}
		return strings.TrimSpace(out), nil
	}
	return toMarkdown(content, opts.LinkStyle), nil
}

func (h *htmlPage) readableContent() (*goquery.Selection, error) {
	article, err := h.readable()
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(article.Content))
	if err != nil {
		return nil, fmt.Errorf("parse readability content: %w", err)
	}
	return doc.Find("body"), nil
}

// sections returns a detached copy of <body> pruned to the content whose
// section is included and not excluded, with the <head> metadata prepended
// when that section is selected.
func (h *htmlPage) sections(include, exclude []string) *goquery.Selection {
	keep := sectionFilter(include, exclude)
	body := h.doc.Find("body").First().Clone()
	if body.Length() > 0 {
		prune(body.Get(0), keep([]string{SectionBody}), keep)
	}
	if keep([]string{SectionMetadata}) {
		body.PrependHtml(h.metadataHTML())
	}
	return body
}

// sectionFilter reports whether content with the given section labels is
// selected: some label included and none excluded.
func sectionFilter(include, exclude []string) func(labels []string) bool {
	return func(labels []string) bool {
		return slices.ContainsFunc(labels, func(l string) bool { return slices.Contains(include, l) }) &&
			!slices.ContainsFunc(labels, func(l string) bool { return slices.Contains(exclude, l) })
	}
}

// prune removes the children of n that fall outside the selected sections.
// An element's section is that of its nearest section root; content outside
// any root is body. A dropped root survives as a bare container when a
// nested root inside it is kept. Returns whether anything under n was kept.
func prune(n *nethtml.Node, keepParent bool, keep func([]string) bool) bool {
	kept := false
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		switch c.Type {
		case nethtml.ElementNode:
			keepChild := keepParent
			if labels := sectionLabels(c); labels != nil {
				keepChild = keep(labels)
			}
			if prune(c, keepChild, keep) || keepChild {
				kept = true
			} else {
				n.RemoveChild(c)
			}
		case nethtml.TextNode:
			if keepParent {
				kept = kept || strings.TrimSpace(c.Data) != ""
			} else {
				n.RemoveChild(c)
			}
		default:
			n.RemoveChild(c)
		}
		c = next
	}
	return kept
}

// sectionLabels names the sections an element is the root of, from HTML
// landmarks and ARIA roles; nil when it is not a section root. <header> and
// <footer> count only outside sectioning content, as in ARIA's mapping.
func sectionLabels(n *nethtml.Node) []string {
	switch firstToken(nodeAttr(n, "role")) {
	case "banner":
		return []string{SectionHeader, SectionBanner}
	case "navigation":
		return []string{SectionNavigation}
	case "complementary":
		return []string{SectionSidebar}
	case "contentinfo":
		return []string{SectionFooter}
	case "main", "article":
		return []string{SectionBody}
	case "alert", "alertdialog", "dialog":
		return []string{SectionBanner}
	}
	switch n.Data {
	case "nav":
		return []string{SectionNavigation}
	case "aside":
		return []string{SectionSidebar}
	case "main", "article":
		return []string{SectionBody}
	case "header":
		if isTopLevel(n) {
			return []string{SectionHeader, SectionBanner}
		}
	case "footer":
		if isTopLevel(n) {
			return []string{SectionFooter}
		}
	}
	if isNoticeBanner(n) {
		return []string{SectionBanner}
	}
	return nil
}

func isTopLevel(n *nethtml.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == nethtml.ElementNode && slices.Contains([]string{"article", "aside", "main", "nav", "section"}, p.Data) {
			return false
		}
	}
	return true
}

var noticeBannerTokens = []string{"cookie", "consent", "gdpr"}

// isNoticeBanner spots cookie/consent notices by id or class.
func isNoticeBanner(n *nethtml.Node) bool {
	names := strings.ToLower(nodeAttr(n, "id") + " " + nodeAttr(n, "class"))
	return slices.ContainsFunc(noticeBannerTokens, func(t string) bool { return strings.Contains(names, t) })
}

func (h *htmlPage) metadataHTML() string {
	m := h.meta
	published := ""
	if m.published != nil {
		published = m.published.Format(time.RFC3339)
	}
	var b strings.Builder
	b.WriteString("<ul>")
	for _, field := range []struct{ name, value string }{
		{"title", m.title},
		{"description", m.description},
		{"author", m.author},
		{"published", published},
		{"site", m.siteName},
		{"keywords", m.keywords},
	} {
		if field.value != "" {
			fmt.Fprintf(&b, "<li>%s: %s</li>", field.name, html.EscapeString(field.value))
		}
	}
	b.WriteString("</ul>")
	return b.String()
}

var nonContentTags = "script, style, noscript, template, svg, canvas, iframe, object, embed, link, meta, button, input, select, textarea"

// clean drops non-content elements, comments and inline data-URI images.
func clean(s *goquery.Selection) {
	s.Find(nonContentTags).Remove()
	s.Find(`img[src^="data:"]`).Remove()
	for _, n := range s.Nodes {
		removeComments(n)
	}
}

func removeComments(n *nethtml.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == nethtml.CommentNode {
			n.RemoveChild(c)
		} else {
			removeComments(c)
		}
		c = next
	}
}

var keptAttributes = []string{"href", "src", "alt", "title", "datetime", "colspan", "rowspan"}

func stripAttributes(s *goquery.Selection) {
	s.Find("*").Each(func(_ int, el *goquery.Selection) {
		n := el.Get(0)
		n.Attr = slices.DeleteFunc(n.Attr, func(a nethtml.Attribute) bool {
			return !slices.Contains(keptAttributes, a.Key)
		})
	})
}

// toMarkdown converts content; citation style numbers each distinct URL and
// appends a `[n]: url` reference list.
func toMarkdown(content *goquery.Selection, linkStyle string) string {
	var refs []string
	numbers := map[string]int{}
	conv := md.NewConverter("", true, nil)
	conv.Use(plugin.GitHubFlavored())
	conv.AddRules(md.Rule{
		Filter: []string{"figcaption"},
		Replacement: func(text string, _ *goquery.Selection, _ *md.Options) *string {
			return md.String("\n\n" + strings.TrimSpace(text) + "\n\n")
		},
	}, md.Rule{
		Filter: []string{"a"},
		AdvancedReplacement: func(text string, s *goquery.Selection, _ *md.Options) (md.AdvancedResult, bool) {
			href := s.AttrOr("href", "")
			u, err := url.Parse(href)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
				return md.AdvancedResult{Markdown: text}, false
			}
			text = strings.TrimSpace(md.EscapeMultiLine(text))
			if text == "" {
				text = firstNonEmpty(s.AttrOr("aria-label", ""), s.AttrOr("title", ""))
			}
			if text == "" {
				return md.AdvancedResult{}, false
			}
			if linkStyle != LinkStyleCitation {
				return md.AdvancedResult{Markdown: md.AddSpaceIfNessesary(s, fmt.Sprintf("[%s](%s)", text, href))}, false
			}
			n, ok := numbers[href]
			if !ok {
				refs = append(refs, href)
				n = len(refs)
				numbers[href] = n
			}
			return md.AdvancedResult{Markdown: md.AddSpaceIfNessesary(s, fmt.Sprintf("[%s][%d]", text, n))}, false
		},
	})
	out := conv.Convert(content)
	if len(refs) == 0 {
		return out
	}
	var b strings.Builder
	b.WriteString(out)
	b.WriteString("\n\n")
	for i, ref := range refs {
		fmt.Fprintf(&b, "[%d]: %s\n", i+1, ref)
	}
	return strings.TrimSpace(b.String())
}

func nodeAttr(n *nethtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func firstToken(s string) string {
	fields := strings.Fields(strings.ToLower(s))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// notURL drops values that are profile links rather than names.
func notURL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return ""
	}
	return s
}
