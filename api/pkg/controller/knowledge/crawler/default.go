package crawler

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/gocolly/colly/v2"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/controller/knowledge/readability"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	defaultMaxDepth    = 10  // How deep to crawl the website
	defaultMaxPages    = 500 // How many pages to crawl before stopping
	defaultParallelism = 20  // How many pages to crawl in parallel
	defaultUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

// Default crawler for web sources, uses colly to crawl the website
// and convert the content to markdown
type Default struct {
	knowledge *types.Knowledge

	converter *md.Converter
	parser    readability.Parser
}

func NewDefault(k *types.Knowledge) (*Default, error) {
	return &Default{
		knowledge: k,
		converter: md.NewConverter("", true, nil),
		parser:    readability.NewParser(),
	}, nil
}

func (d *Default) Crawl(ctx context.Context) ([]*types.CrawledDocument, error) {
	var domains []string
	for _, u := range d.knowledge.Source.Web.URLs {
		parsedURL, err := url.Parse(u)
		if err != nil {
			return nil, err
		}
		domains = append(domains, parsedURL.Host)
	}

	var (
		maxPages    int32
		maxDepth    int
		userAgent   string
		pageCounter atomic.Int32
	)

	if d.knowledge.Source.Web.Crawler.MaxDepth == 0 {
		maxDepth = defaultMaxDepth
	} else {
		maxDepth = d.knowledge.Source.Web.Crawler.MaxDepth
	}

	if d.knowledge.Source.Web.Crawler.UserAgent == "" {
		userAgent = defaultUserAgent
	} else {
		userAgent = d.knowledge.Source.Web.Crawler.UserAgent
	}

	if !d.knowledge.Source.Web.Crawler.Enabled {
		maxPages = 1
	} else {
		if d.knowledge.Source.Web.Crawler.MaxPages > 500 {
			maxPages = 500
		}

		if d.knowledge.Source.Web.Crawler.MaxPages == 0 {
			maxPages = defaultMaxPages
		} else {
			maxPages = int32(d.knowledge.Source.Web.Crawler.MaxPages)
		}
	}

	pageCounter.Store(0)

	collyOptions := []colly.CollectorOption{
		colly.AllowedDomains(domains...),
		colly.UserAgent(userAgent),
		colly.MaxDepth(maxDepth), // Limit crawl depth to avoid infinite crawling
		colly.IgnoreRobotsTxt(),
	}

	if len(d.knowledge.Source.Web.Excludes) > 0 {
		// Create the regex for the excludes
		excludesRegex := regexp.MustCompile(strings.Join(d.knowledge.Source.Web.Excludes, "|"))
		collyOptions = append(collyOptions, colly.DisallowedURLFilters(excludesRegex))
	}

	collector := colly.NewCollector(collyOptions...)

	for _, domain := range domains {
		collector.Limit(&colly.LimitRule{
			DomainGlob:  fmt.Sprintf("*%s*", domain),
			Parallelism: defaultParallelism,
		})
	}

	var crawledDocs []*types.CrawledDocument

	collector.OnHTML("html", func(e *colly.HTMLElement) {
		log.Trace().
			Str("knowledge_id", d.knowledge.ID).
			Str("url", e.Request.URL.String()).Msg("Visiting link")

		doc := &types.CrawledDocument{
			SourceURL: e.Request.URL.String(),
		}

		// Extract title
		doc.Title = e.ChildText("title")

		// Extract description
		doc.Description = e.ChildAttr("meta[name=description]", "content")

		// Extract and convert content to markdown
		content, err := e.DOM.Find("body").Html()
		if err != nil {
			log.Warn().Err(err).Str("url", e.Request.URL.String()).Msg("Error getting body HTML")
			return
		}

		doc, err = d.convertHTMLToMarkdown(content, doc)
		if err != nil {
			log.Warn().Err(err).Str("url", e.Request.URL.String()).Msg("Error converting HTML to markdown")
			return
		}

		crawledDocs = append(crawledDocs, doc)

		pageCounter.Add(1)
	})

	// Add this new OnHTML callback to find and visit links
	collector.OnHTML("a[href]", func(e *colly.HTMLElement) {
		if pageCounter.Load() >= maxPages {
			log.Warn().
				Str("knowledge_id", d.knowledge.ID).
				Msg("Max pages reached")
			return
		}

		link := e.Attr("href")
		collector.Visit(e.Request.AbsoluteURL(link))
	})

	collector.OnRequest(func(r *colly.Request) {
		r.Ctx.Put("url", r.URL.String())
	})

	log.Info().
		Str("knowledge_id", d.knowledge.ID).
		Str("knowledge_name", d.knowledge.Name).
		Str("url", d.knowledge.Source.Web.URLs[0]).
		Str("domains", strings.Join(domains, ",")).
		Msg("starting to crawl the website")

	for _, url := range d.knowledge.Source.Web.URLs {
		err := collector.Visit(url)
		if err != nil {
			log.Warn().Err(err).Str("url", url).Msg("Error visiting URL")
			// Continue with the next URL instead of returning
			continue
		}
	}

	log.Info().
		Str("knowledge_id", d.knowledge.ID).
		Str("knowledge_name", d.knowledge.Name).
		Str("url", d.knowledge.Source.Web.URLs[0]).
		Str("domains", strings.Join(domains, ",")).
		Int32("pages_crawled", pageCounter.Load()).
		Msg("finished crawling the website")

	return crawledDocs, nil
}

func (d *Default) convertHTMLToMarkdown(content string, doc *types.CrawledDocument) (*types.CrawledDocument, error) {
	if !d.knowledge.Source.Web.Crawler.Readability {
		// If readability is turned off, try to convert HTML directly
		markdown, err := d.converter.ConvertString(content)
		if err != nil {
			return nil, err
		}

		doc.Content = strings.TrimSpace(markdown)

		return doc, nil
	}

	ctx := context.Background()

	// If we have readability enabled, use the readability parser
	article, err := d.parser.Parse(ctx, content, doc.SourceURL)
	if err != nil {
		return nil, err
	}

	markdown, err := d.converter.ConvertString(article.Content)
	if err != nil {
		return nil, err
	}

	doc.Content = strings.TrimSpace(markdown)
	doc.Title = article.Title
	doc.Description = article.Excerpt

	return doc, nil
}
