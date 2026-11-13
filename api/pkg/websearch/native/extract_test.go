package native

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/websearch"
)

const extractArticleURL = "https://news.example.com/world/2026/storm.html"

func extractFixture(t *testing.T, name, pageURL string) *RawPage {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return &RawPage{URL: pageURL, FinalURL: pageURL, StatusCode: 200, ContentType: "text/html; charset=utf-8", Body: body}
}

func extractPage(t *testing.T, page *RawPage, opts websearch.TextOptions) *Page {
	t.Helper()
	out, err := NewExtractor(nil).Extract(context.Background(), page, opts)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return out
}

func extractArticleText(t *testing.T, opts websearch.TextOptions) string {
	t.Helper()
	return extractPage(t, extractFixture(t, "extract_article.html", extractArticleURL), opts).Text
}

func assertContains(t *testing.T, text string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("text lacks %q:\n%s", w, text)
		}
	}
}

func assertLacks(t *testing.T, text string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(text, u) {
			t.Errorf("text contains %q:\n%s", u, text)
		}
	}
}

// Markers for each part of extract_article.html.
const (
	markBody     = "made landfall on the northern coast"
	markTable    = "Northport"
	markHeadline = "Storm hits the coast"
	markNav      = "Business"
	markBanner   = "We use cookies"
	markSidebar  = "Most read"
	markFooter   = "All rights reserved"
	markScript   = "window.analytics"
)

func TestExtractHTMLMetadata(t *testing.T) {
	page := extractPage(t, extractFixture(t, "extract_article.html", extractArticleURL), websearch.TextOptions{})

	if page.Title != "Storm hits the coast" {
		t.Errorf("Title = %q", page.Title)
	}
	// meta author is absent and article:author is a profile URL, so JSON-LD wins.
	if page.Author != "Jane Doe, John Roe" {
		t.Errorf("Author = %q", page.Author)
	}
	want := time.Date(2026, 11, 12, 7, 30, 0, 0, time.UTC)
	if page.PublishedDate == nil || !page.PublishedDate.Equal(want) || page.PublishedDate.Location() != time.UTC {
		t.Errorf("PublishedDate = %v, want %v", page.PublishedDate, want)
	}
	if page.Image != "https://news.example.com/media/storm-hero.jpg" {
		t.Errorf("Image = %q", page.Image)
	}
	if page.Favicon != "https://news.example.com/static/favicon.png" {
		t.Errorf("Favicon = %q", page.Favicon)
	}
}

func TestExtractHTMLMetadataFallbacks(t *testing.T) {
	html := `<html><head>
<meta name="AUTHOR" content="Meta Author">
<meta itemprop="datePublished" content="March 5, 2025">
<meta name="twitter:image" content="//img.example.com/t.png">
<link rel="apple-touch-icon" href="/touch.png">
<script type="application/ld+json">{"@type":"Article","author":"LD Author","datePublished":"2020-01-01"}</script>
<script type="application/ld+json">{not json</script>
</head><body><p>Short page.</p></body></html>`
	page := extractPage(t, &RawPage{FinalURL: "https://blog.example.com/p/1", ContentType: "text/html", Body: []byte(html)}, websearch.TextOptions{})

	if page.Author != "Meta Author" {
		t.Errorf("Author = %q, want the meta author ahead of JSON-LD", page.Author)
	}
	if page.PublishedDate == nil || !page.PublishedDate.Equal(time.Date(2025, 3, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PublishedDate = %v", page.PublishedDate)
	}
	if page.Image != "https://img.example.com/t.png" {
		t.Errorf("Image = %q", page.Image)
	}
	if page.Favicon != "https://blog.example.com/touch.png" {
		t.Errorf("Favicon = %q", page.Favicon)
	}
}

func TestExtractHTMLWithoutMetadata(t *testing.T) {
	page := extractPage(t, extractFixture(t, "extract_bare.html", "http://docs.example.net/guide/index.html"), websearch.TextOptions{})

	if page.Title != "Getting started" {
		t.Errorf("Title = %q, want the first <h1>", page.Title)
	}
	if page.Author != "" || page.PublishedDate != nil || page.Image != "" {
		t.Errorf("unexpected metadata: author=%q date=%v image=%q", page.Author, page.PublishedDate, page.Image)
	}
	if page.Favicon != "http://docs.example.net/favicon.ico" {
		t.Errorf("Favicon = %q, want the derived /favicon.ico", page.Favicon)
	}
	wantLinks := []Link{
		{URL: "http://docs.example.net/guide/docs/start.html", Anchor: "quick start guide"},
		{URL: "http://docs.example.net/forum", Anchor: "community forum"},
	}
	if len(page.Links) != len(wantLinks) {
		t.Fatalf("Links = %+v", page.Links)
	}
	for i, l := range wantLinks {
		if page.Links[i] != l {
			t.Errorf("Links[%d] = %+v, want %+v", i, page.Links[i], l)
		}
	}
	if len(page.ImageLinks) != 1 || page.ImageLinks[0] != (websearch.ImageLink{URL: "http://docs.example.net/guide/diagram.png"}) {
		t.Errorf("ImageLinks = %+v", page.ImageLinks)
	}
	assertContains(t, page.Text, "[quick start guide](http://docs.example.net/guide/docs/start.html)")
}

func TestExtractHTMLLinksAndImages(t *testing.T) {
	page := extractPage(t, extractFixture(t, "extract_article.html", extractArticleURL), websearch.TextOptions{})

	links := map[string]string{}
	for _, l := range page.Links {
		if _, dup := links[l.URL]; dup {
			t.Errorf("duplicate link %s", l.URL)
		}
		links[l.URL] = l.Anchor
	}
	for u, anchor := range map[string]string{
		"https://news.example.com/":                                   "Example News logo",
		"https://news.example.com/world":                              "World",
		"https://news.example.com/about":                              "About us",
		"https://news.example.com/world/2026/evacuation-centres.html": "twelve towns",
		"https://weather.example.org/forecast?region=north":           "full forecast",
		"https://news.example.com/world/2026/storm.html":              "Jump to comments",
		"https://news.example.com/privacy":                            "Privacy policy",
		"https://news.example.com/world/2026/heatwave.html":           "Heatwave records broken",
	} {
		if got, ok := links[u]; !ok || got != anchor {
			t.Errorf("link %s: anchor %q (present=%v), want %q", u, got, ok, anchor)
		}
	}
	for u := range links {
		if !strings.HasPrefix(u, "https://") {
			t.Errorf("non-http(s) link %s", u)
		}
	}

	want := []websearch.ImageLink{
		{URL: "https://news.example.com/static/logo.svg", Alt: "Example News logo"},
		{URL: "https://news.example.com/world/2026/images/flooded-street.jpg", Alt: "A flooded street in the harbour district"},
	}
	if len(page.ImageLinks) != len(want) {
		t.Fatalf("ImageLinks = %+v", page.ImageLinks)
	}
	for i := range want {
		if page.ImageLinks[i] != want[i] {
			t.Errorf("ImageLinks[%d] = %+v, want %+v", i, page.ImageLinks[i], want[i])
		}
	}
}

func TestExtractVerbosity(t *testing.T) {
	compact := extractArticleText(t, websearch.TextOptions{})
	if explicit := extractArticleText(t, websearch.TextOptions{Verbosity: VerbosityCompact}); explicit != compact {
		t.Error("default verbosity differs from compact")
	}
	assertContains(t, compact, markBody, "![A flooded street in the harbour district](https://news.example.com/world/2026/images/flooded-street.jpg)\n\nFlooding in the harbour district")
	assertLacks(t, compact, markNav, markBanner, markSidebar, markFooter, markScript, "data:image")

	standard := extractArticleText(t, websearch.TextOptions{Verbosity: VerbosityStandard})
	assertContains(t, standard, markHeadline, markBody, markTable, "Additional reporting")
	assertLacks(t, standard, markNav, markBanner, markSidebar, markFooter, markScript, "tracking comment", "Accept")

	full := extractArticleText(t, websearch.TextOptions{Verbosity: VerbosityFull})
	assertContains(t, full, markBody, markNav, markBanner, markSidebar, markFooter)
	assertLacks(t, full, markScript, "font-family", "tracking comment", "- title:")
}

func TestExtractSections(t *testing.T) {
	cases := []struct {
		name    string
		opts    websearch.TextOptions
		want    []string
		unwants []string
	}{
		{
			name:    "include navigation",
			opts:    websearch.TextOptions{IncludeSections: []string{SectionNavigation}},
			want:    []string{"[World](https://news.example.com/world)", markNav},
			unwants: []string{markBody, markBanner, "Example News logo", markFooter},
		},
		{
			name:    "a nav inside the header is navigation, not header",
			opts:    websearch.TextOptions{IncludeSections: []string{SectionHeader}},
			want:    []string{"Example News logo"},
			unwants: []string{markNav, markBody, markBanner, markFooter},
		},
		{
			name:    "header and navigation",
			opts:    websearch.TextOptions{IncludeSections: []string{SectionHeader, SectionNavigation}},
			want:    []string{"Example News logo", markNav},
			unwants: []string{markBody},
		},
		{
			name:    "include banner",
			opts:    websearch.TextOptions{IncludeSections: []string{SectionBanner}},
			want:    []string{markBanner},
			unwants: []string{markBody, markFooter},
		},
		{
			name:    "include body keeps the article header and footer",
			opts:    websearch.TextOptions{IncludeSections: []string{SectionBody}},
			want:    []string{markHeadline, markBody, markTable, "Additional reporting"},
			unwants: []string{markNav, markBanner, markSidebar, markFooter},
		},
		{
			name:    "include sidebar and footer",
			opts:    websearch.TextOptions{IncludeSections: []string{SectionSidebar, SectionFooter}},
			want:    []string{markSidebar, markFooter},
			unwants: []string{markBody, markNav},
		},
		{
			name:    "exclude only",
			opts:    websearch.TextOptions{ExcludeSections: []string{SectionFooter, SectionSidebar, SectionBanner}},
			want:    []string{markBody, markNav, "- title: Storm hits the coast"},
			unwants: []string{markFooter, markSidebar, markBanner},
		},
		{
			name: "include metadata",
			opts: websearch.TextOptions{IncludeSections: []string{SectionMetadata}},
			want: []string{
				"- title: Storm hits the coast",
				"- description: A powerful storm made landfall overnight.",
				"- author: Jane Doe, John Roe",
				"- published: 2026-11-12T07:30:00Z",
				"- site: Example News",
			},
			unwants: []string{markBody, markNav},
		},
		{
			name:    "sections override verbosity",
			opts:    websearch.TextOptions{Verbosity: VerbosityFull, IncludeSections: []string{SectionFooter}},
			want:    []string{markFooter},
			unwants: []string{markBody},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := extractArticleText(t, tc.opts)
			assertContains(t, text, tc.want...)
			assertLacks(t, text, tc.unwants...)
		})
	}
}

func TestExtractIncludeHTMLTags(t *testing.T) {
	for _, verbosity := range []string{VerbosityCompact, VerbosityStandard} {
		text := extractArticleText(t, websearch.TextOptions{IncludeHTMLTags: true, Verbosity: verbosity})
		assertContains(t, text, "<p>", markBody,
			`<a href="https://news.example.com/world/2026/evacuation-centres.html">twelve towns</a>`,
			`<img src="https://news.example.com/world/2026/images/flooded-street.jpg" alt="A flooded street in the harbour district"/>`)
		assertLacks(t, text, "<script", "<style", "class=", "<!--", "data:image", "[twelve towns]")
	}
}

func TestExtractLinkStyles(t *testing.T) {
	full := extractArticleText(t, websearch.TextOptions{Verbosity: VerbosityStandard, LinkStyle: LinkStyleFull})
	assertContains(t, full,
		"[twelve towns](https://news.example.com/world/2026/evacuation-centres.html)",
		"[full forecast](https://weather.example.org/forecast?region=north#today)")
	assertLacks(t, full, "mailto:", "javascript:")
	if !strings.Contains(full, "news@example.com") || !strings.Contains(full, "share this story") {
		t.Errorf("non-http links should keep their text:\n%s", full)
	}

	cited := extractArticleText(t, websearch.TextOptions{Verbosity: VerbosityStandard, LinkStyle: LinkStyleCitation})
	assertContains(t, cited,
		"[twelve towns][1]",
		"[full forecast][2]",
		"[more world news][3]",
		"\n\n[1]: https://news.example.com/world/2026/evacuation-centres.html\n[2]: https://weather.example.org/forecast?region=north#today\n[3]: https://news.example.com/world\n[4]: https://news.example.com/world/2026/storm.html#comments")
	assertLacks(t, cited, "](https://news.example.com/world)", "](https://news.example.com/world/2026/evac")

	// A repeated URL keeps its first number.
	repeated := extractPage(t, &RawPage{
		FinalURL:    "https://a.example.com/",
		ContentType: "text/html",
		Body:        []byte(`<body><main><p><a href="/x">one</a> <a href="/y">two</a> <a href="/x">again</a></p></main></body>`),
	}, websearch.TextOptions{Verbosity: VerbosityFull, LinkStyle: LinkStyleCitation}).Text
	want := "[one][1] [two][2] [again][1]\n\n[1]: https://a.example.com/x\n[2]: https://a.example.com/y"
	if repeated != want {
		t.Errorf("citation text = %q, want %q", repeated, want)
	}
}

func TestExtractBaseHrefAndCharset(t *testing.T) {
	// "café" in windows-1252, with a <base> that relative links resolve against.
	body := []byte("<html><head><base href=\"https://cdn.example.com/docs/\"></head><body><main><p>caf\xe9 <a href=\"intro\">intro</a></p></main></body></html>")
	page := extractPage(t, &RawPage{FinalURL: "https://www.example.com/x", ContentType: "text/html; charset=windows-1252", Body: body},
		websearch.TextOptions{Verbosity: VerbosityStandard})
	assertContains(t, page.Text, "café", "[intro](https://cdn.example.com/docs/intro)")
	if page.Favicon != "https://www.example.com/favicon.ico" {
		t.Errorf("Favicon = %q, want it derived from the page URL", page.Favicon)
	}
}

func TestExtractInvalidOptions(t *testing.T) {
	for _, opts := range []websearch.TextOptions{
		{Verbosity: "verbose"},
		{LinkStyle: "footnote"},
		{IncludeSections: []string{"main"}},
		{ExcludeSections: []string{"ads"}},
	} {
		page := extractFixture(t, "extract_article.html", extractArticleURL)
		if _, err := NewExtractor(nil).Extract(context.Background(), page, opts); err == nil {
			t.Errorf("opts %+v: want error", opts)
		}
	}
}

func TestExtractDocuments(t *testing.T) {
	pdf := []byte("%PDF-1.7\n...binary...")
	var docx bytes.Buffer
	zw := zip.NewWriter(&docx)
	if _, err := zw.Create("word/document.xml"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name        string
		contentType string
		body        []byte
	}{
		{"pdf by content type", "application/pdf", []byte("not really a pdf")},
		{"pdf by magic bytes", "application/octet-stream", pdf},
		{"pdf without content type", "", pdf},
		{"docx by content type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", docx.Bytes()},
		{"docx by zip contents", "binary/octet-stream", docx.Bytes()},
		{"legacy office by magic bytes", "", append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, 0, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			documents := extract.NewMockExtractor(ctrl)
			documents.EXPECT().
				Extract(gomock.Any(), &extract.Request{Content: tc.body}).
				Return("\n\n  Annual Report 2026  \nRevenue grew.\n", nil)

			page, err := NewExtractor(documents).Extract(context.Background(), &RawPage{
				FinalURL: "https://files.example.com/reports/annual.pdf", ContentType: tc.contentType, Body: tc.body,
			}, websearch.TextOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if page.Title != "Annual Report 2026" || page.Text != "Annual Report 2026  \nRevenue grew." {
				t.Errorf("page = %+v", page)
			}
			if page.Favicon != "https://files.example.com/favicon.ico" {
				t.Errorf("Favicon = %q", page.Favicon)
			}
		})
	}
}

func TestExtractDocumentErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	documents := extract.NewMockExtractor(ctrl)
	failure := errors.New("extractor down")
	documents.EXPECT().Extract(gomock.Any(), gomock.Any()).Return("", failure)

	page := &RawPage{FinalURL: "https://x.example.com/a.pdf", ContentType: "application/pdf", Body: []byte("%PDF-")}
	if _, err := NewExtractor(documents).Extract(context.Background(), page, websearch.TextOptions{}); !errors.Is(err, failure) {
		t.Errorf("err = %v, want the extractor error", err)
	}
	if _, err := NewExtractor(nil).Extract(context.Background(), page, websearch.TextOptions{}); err == nil {
		t.Error("want an error when document extraction is not configured")
	}
}

func TestExtractUnsupported(t *testing.T) {
	for _, tc := range []struct {
		contentType string
		body        []byte
	}{
		{"image/png", []byte("\x89PNG\r\n\x1a\n")},
		{"application/json", []byte(`{"a":1}`)},
		{"application/octet-stream", []byte{0x00, 0x01, 0x02, 0xff}},
		{"", []byte("PK\x03\x04 not an office archive")},
		{"/", []byte("<html></html>")},
	} {
		_, err := NewExtractor(nil).Extract(context.Background(), &RawPage{FinalURL: "https://x.example.com/f", ContentType: tc.contentType, Body: tc.body}, websearch.TextOptions{})
		if !errors.Is(err, ErrUnsupportedDocument) {
			t.Errorf("%q: err = %v, want ErrUnsupportedDocument", tc.contentType, err)
		}
	}
}

func TestExtractMalformedContentTypeParameter(t *testing.T) {
	page := extractPage(t, &RawPage{FinalURL: "https://x.example.com/", ContentType: "text/html; charset", Body: []byte("<main><p>hello</p></main>")},
		websearch.TextOptions{Verbosity: VerbosityStandard})
	if page.Text != "hello" {
		t.Errorf("Text = %q", page.Text)
	}
}

func TestExtractPlainText(t *testing.T) {
	page := extractPage(t, &RawPage{FinalURL: "https://raw.example.com/README.md", ContentType: "text/markdown", Body: []byte("# Project\n\nSome text.\n")}, websearch.TextOptions{})
	if page.Title != "Project" || page.Text != "# Project\n\nSome text." {
		t.Errorf("page = %+v", page)
	}
}

func TestExtractRequiresAbsoluteURL(t *testing.T) {
	for _, u := range []string{"", "/relative", "ftp://x.example.com/a"} {
		if _, err := NewExtractor(nil).Extract(context.Background(), &RawPage{FinalURL: u, ContentType: "text/html", Body: []byte("<p>x</p>")}, websearch.TextOptions{}); err == nil {
			t.Errorf("url %q: want error", u)
		}
	}
}

func TestExtractParseDate(t *testing.T) {
	for raw, want := range map[string]time.Time{
		"2026-11-12T08:30:00+01:00":       time.Date(2026, 11, 12, 7, 30, 0, 0, time.UTC),
		"2026-11-12T08:30:00.123Z":        time.Date(2026, 11, 12, 8, 30, 0, 123000000, time.UTC),
		"2026-11-12T08:30:00+0100":        time.Date(2026, 11, 12, 7, 30, 0, 0, time.UTC),
		"2026-11-12T08:30":                time.Date(2026, 11, 12, 8, 30, 0, 0, time.UTC),
		"2026-11-12 08:30:00":             time.Date(2026, 11, 12, 8, 30, 0, 0, time.UTC),
		"2026-11-12":                      time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC),
		"Thu, 12 Nov 2026 08:30:00 +0000": time.Date(2026, 11, 12, 8, 30, 0, 0, time.UTC),
		"November 12, 2026":               time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC),
		"12 Nov 2026":                     time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC),
	} {
		got := parseDate(raw)
		if got == nil || !got.Equal(want) {
			t.Errorf("parseDate(%q) = %v, want %v", raw, got, want)
		}
	}
	for _, raw := range []string{"", "yesterday", "12/11/2026"} {
		if got := parseDate(raw); got != nil {
			t.Errorf("parseDate(%q) = %v, want nil", raw, got)
		}
	}
}
