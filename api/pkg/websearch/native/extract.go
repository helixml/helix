package native

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/PuerkitoBio/goquery"
	readability "github.com/go-shiori/go-readability"
	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/charset"

	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/websearch"
)

// PageExtractor implements Extractor: HTML through readability, the DOM and
// html-to-markdown; PDFs and office documents through pkg/extract.
type PageExtractor struct {
	documents extract.Extractor
}

var _ Extractor = (*PageExtractor)(nil)

// NewExtractor extracts PDFs and office documents with documents, which is
// required.
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
	if err != nil || !isWebURL(u) {
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
	case isHTMLMediaType(mediaType):
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
	// Content only: passing the URL would make the extraction service fetch
	// it itself, bypassing the guarded fetcher.
	text, err := e.documents.Extract(ctx, &extract.Request{Content: page.Body})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %w", ErrExtractionFailed, err)
	}
	text = strings.TrimSpace(text)
	return &Page{Title: firstLine(text), Text: text, Favicon: defaultFavicon(pageURL)}, nil
}

func extractPlainText(page *RawPage, pageURL *url.URL) (*Page, error) {
	r, err := charset.NewReader(bytes.NewReader(page.Body), page.ContentType)
	if err != nil {
		return nil, unextractable("decode text", err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		return nil, unextractable("decode text", err)
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
		return nil, unextractable("decode html", err)
	}
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, unextractable("parse html", err)
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
	if err != nil || !isWebURL(u) {
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
		prune(body.Get(0), true, sectionFilter([]string{websearch.SectionBody}, nil))
	}
	article, err := readability.FromDocument(doc.Get(0), h.pageURL)
	if err != nil {
		return nil, unextractable("readability", err)
	}
	h.article = &article
	return h.article, nil
}

// links is every http(s) link on the page, deduplicated by URL without its
// fragment, in document order.
func (h *htmlPage) links() []Link {
	var links []Link
	index := map[string]int{}
	h.doc.Find("body a[href]").Each(func(_ int, s *goquery.Selection) {
		u, err := url.Parse(s.AttrOr("href", ""))
		if err != nil || !isWebURL(u) {
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

// unextractable marks a failure caused by the page's content.
func unextractable(step string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrUnsupportedDocument, step, err)
}

// isWebURL reports whether u is an absolute http(s) URL.
func isWebURL(u *url.URL) bool {
	return u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
}

func isHTMLMediaType(mediaType string) bool {
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
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
