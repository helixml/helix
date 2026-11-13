package native

import (
	"fmt"
	"html"
	"net/url"
	"slices"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/JohannesKaufmann/html-to-markdown/plugin"
	"github.com/PuerkitoBio/goquery"
	nethtml "golang.org/x/net/html"

	"github.com/helixml/helix/api/pkg/websearch"
)

// text renders the page text per opts, which the request has validated.
// Sections, when given, select parts of the full document and override
// verbosity. Verbosity levels:
//   - compact (default): readability's main content, found within the body
//     section.
//   - standard: the whole <body> minus header, navigation, banner, sidebar
//     and footer, so it keeps tables, captions and asides-in-article that
//     readability drops.
//   - full: every section of <body> (not the <head> metadata).
func (h *htmlPage) text(opts websearch.TextOptions) (string, error) {
	var content *goquery.Selection
	var err error
	switch {
	case len(opts.IncludeSections) > 0 || len(opts.ExcludeSections) > 0:
		include := opts.IncludeSections
		if len(include) == 0 {
			include = websearch.Sections
		}
		content = h.sections(include, opts.ExcludeSections)
	case opts.Verbosity == websearch.VerbosityStandard:
		content = h.sections([]string{websearch.SectionBody}, nil)
	case opts.Verbosity == websearch.VerbosityFull:
		content = h.sections(websearch.Sections, []string{websearch.SectionMetadata})
	default:
		content, err = h.readableContent()
	}
	if err != nil {
		return "", err
	}

	clean(content)
	if opts.IncludeHTMLTags {
		stripAttributes(content)
		out, err := content.Html()
		if err != nil {
			return "", unextractable("render html", err)
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
		return nil, unextractable("parse readability content", err)
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
		prune(body.Get(0), keep([]string{websearch.SectionBody}), keep)
	}
	if keep([]string{websearch.SectionMetadata}) {
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
		return []string{websearch.SectionHeader, websearch.SectionBanner}
	case "navigation":
		return []string{websearch.SectionNavigation}
	case "complementary":
		return []string{websearch.SectionSidebar}
	case "contentinfo":
		return []string{websearch.SectionFooter}
	case "main", "article":
		return []string{websearch.SectionBody}
	case "alert", "alertdialog", "dialog":
		return []string{websearch.SectionBanner}
	}
	switch n.Data {
	case "nav":
		return []string{websearch.SectionNavigation}
	case "aside":
		return []string{websearch.SectionSidebar}
	case "main", "article":
		return []string{websearch.SectionBody}
	case "header":
		if isTopLevel(n) {
			return []string{websearch.SectionHeader, websearch.SectionBanner}
		}
	case "footer":
		if isTopLevel(n) {
			return []string{websearch.SectionFooter}
		}
	}
	if isNoticeBanner(n) {
		return []string{websearch.SectionBanner}
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
			if linkStyle != websearch.LinkStyleCitation {
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
