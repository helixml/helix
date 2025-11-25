package crawler

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/gocolly/colly/v2"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/controller/knowledge/readability"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	defaultDepth       = 10
	defaultMaxDepth    = 100 // How many pages to crawl before stopping
	defaultParallelism = 5   // How many pages to crawl in parallel
	defaultUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

// Default crawler for web sources, uses colly to crawl the website
// and convert the content to markdown
type Default struct {
	knowledge *types.Knowledge

	converter *md.Converter
	parser    readability.Parser

	browser *browser.Browser

	pageTimeout time.Duration

	updateProgress func(progress types.KnowledgeProgress)

	// disableDomainCheck disables colly's AllowedDomains restriction
	// Useful for testing with localhost where AllowedDomains doesn't work properly
	disableDomainCheck bool
}

func NewDefault(browser *browser.Browser, k *types.Knowledge, updateProgress func(progress types.KnowledgeProgress)) (*Default, error) {
	crawler := &Default{
		knowledge:      k,
		converter:      md.NewConverter("", true, nil),
		parser:         readability.NewParser(),
		browser:        browser,
		pageTimeout:    15 * time.Second,
		updateProgress: updateProgress,
	}

	return crawler, nil
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

	started := time.Now()

	var (
		maxPages  int32
		userAgent string
		// Number of pages queued to be crawled, we increment this
		// number as we add pages to the queue on "a[href]" elements
		pageQueueCounter atomic.Int32
	)

	if d.knowledge.Source.Web.Crawler.UserAgent == "" {
		userAgent = defaultUserAgent
	} else {
		userAgent = d.knowledge.Source.Web.Crawler.UserAgent
	}

	if !d.knowledge.Source.Web.Crawler.Enabled {
		maxPages = 1
	} else {
		if d.knowledge.Source.Web.Crawler.MaxDepth > defaultMaxDepth {
			maxPages = defaultMaxDepth
		}

		// If not specified, use the default depth
		if d.knowledge.Source.Web.Crawler.MaxDepth == 0 {
			maxPages = defaultDepth
		} else {
			maxPages = int32(d.knowledge.Source.Web.Crawler.MaxDepth)
		}
	}

	pageQueueCounter.Store(0)

	collyOptions := []colly.CollectorOption{
		// Some websites block requests with the same user agent
		colly.UserAgent(userAgent),
		// If you have this as "2" then it will crawl the current
		// supplied page and the pages it links to
		colly.MaxDepth(int(maxPages)),
	}

	// Only add AllowedDomains if domain checking is enabled
	// Note: colly's AllowedDomains doesn't work well with localhost/IP addresses with ports
	// For tests using localhost, set disableDomainCheck to true
	if !d.disableDomainCheck && len(domains) > 0 {
		collyOptions = append(collyOptions, colly.AllowedDomains(domains...))
	}

	if d.knowledge.Source.Web.Crawler.IgnoreRobotsTxt {
		collyOptions = append(collyOptions, colly.IgnoreRobotsTxt())
	}

	if len(d.knowledge.Source.Web.Excludes) > 0 {
		// Create the regex for the excludes
		excludesRegex := regexp.MustCompile(strings.Join(d.knowledge.Source.Web.Excludes, "|"))
		collyOptions = append(collyOptions, colly.DisallowedURLFilters(excludesRegex))
	}

	collector := colly.NewCollector(collyOptions...)

	b, err := d.browser.GetBrowser()
	if err != nil {
		return nil, fmt.Errorf("error getting browser: %w", err)
	}

	defer func() {
		if err := d.browser.PutBrowser(b); err != nil {
			log.Warn().Err(err).Msg("error putting browser")
		}
	}()

	for _, domain := range domains {
		if err := collector.Limit(&colly.LimitRule{
			DomainGlob:  fmt.Sprintf("*%s*", domain),
			Parallelism: defaultParallelism,
			// RandomDelay: 1 * time.Second,
		}); err != nil {
			log.Warn().
				Str("domain_glob", fmt.Sprintf("*%s*", domain)).
				Msg("failed setting collector limit")
		}
	}

	var (
		crawledMu   sync.Mutex
		crawledDocs []*types.CrawledDocument
	)

	crawledURLs := make(map[string]bool)

	collector.OnHTML("html", func(e *colly.HTMLElement) {
		visited := pageQueueCounter.Load()

		log.Info().
			Str("knowledge_id", d.knowledge.ID).
			Int32("visited_pages", visited).
			Str("url", e.Request.URL.String()).Msg("visiting link")

		// TODO: get more URLs from the browser too, it can render
		doc, err := d.crawlWithBrowser(ctx, b, e.Request.URL.String())
		if err != nil {
			log.Warn().
				Err(err).
				Str("url", e.Request.URL.String()).
				Msg("error crawling URL")

			// Errored pages still count as visited
			crawledMu.Lock()
			crawledDocs = append(crawledDocs, &types.CrawledDocument{
				SourceURL:  e.Request.URL.String(),
				StatusCode: 0,
				DurationMs: 0,
				Message:    err.Error(),
			})
			crawledMu.Unlock()

			return
		}

		log.Info().
			Str("knowledge_id", d.knowledge.ID).
			Str("url", e.Request.URL.String()).
			Int64("duration_ms", doc.DurationMs).
			Int("status_code", doc.StatusCode).
			Msg("crawled page")

		crawledMu.Lock()
		crawledDocs = append(crawledDocs, doc)
		crawledMu.Unlock()

		d.updateProgress(types.KnowledgeProgress{
			Step:           "Crawling",
			Progress:       0, // We don't know the progress here
			ElapsedSeconds: int(time.Since(started).Seconds()),
			Message:        fmt.Sprintf("Visited %d pages", visited),
			StartedAt:      started,
		})
	})

	// Add this new OnHTML callback to find and visit links
	collector.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")

		if pageQueueCounter.Load() > maxPages {
			log.Trace().
				Str("knowledge_id", d.knowledge.ID).
				Str("url", e.Request.URL.String()).
				Str("absolute_url", e.Request.AbsoluteURL(link)).
				Int32("max_pages", maxPages).
				Msg("Max pages reached")
			return
		}

		crawledMu.Lock()
		alreadyCrawled := crawledURLs[link]
		crawledMu.Unlock()

		// This link has been seen, nothing to do
		if alreadyCrawled {
			return
		}

		// Increment the page queue counter
		pageQueueCounter.Add(1)

		// Add link to the seen list
		crawledMu.Lock()
		crawledURLs[link] = true
		crawledMu.Unlock()

		// Schedule the link to be crawled
		err := e.Request.Visit(e.Request.AbsoluteURL(link))
		if err != nil {
			if strings.Contains(err.Error(), "URL already visited") {
				// Common error if duplicate URLs are visited
				return
			}
			if strings.Contains(err.Error(), "Forbidden domain") {
				// Common error when domain links to another domain (we can ignore these)
				return
			}
			log.Warn().
				Err(err).
				Str("url", e.Request.URL.String()).
				Str("absolute_url", e.Request.AbsoluteURL(link)).
				Msg("error visiting link")
		}
	})

	collector.OnRequest(func(r *colly.Request) {
		r.Ctx.Put("url", r.URL.String())
	})

	log.Info().
		Str("knowledge_id", d.knowledge.ID).
		Str("knowledge_name", d.knowledge.Name).
		Str("url", d.knowledge.Source.Web.URLs[0]).
		Str("domains", strings.Join(domains, ",")).
		Int("max_pages", int(maxPages)).
		Msg("starting to crawl the website")

	for _, url := range d.knowledge.Source.Web.URLs {
		err := collector.Visit(url)
		if err != nil {
			log.Warn().Err(err).Str("url", url).Msg("Error visiting URL")

			// Add an error document for this failed URL
			crawledMu.Lock()
			crawledDocs = append(crawledDocs, &types.CrawledDocument{
				SourceURL:  url,
				StatusCode: 0,
				DurationMs: 0,
				Message:    err.Error(),
			})
			crawledMu.Unlock()

			// Continue with the next URL instead of returning
			continue
		}
	}

	log.Info().
		Str("knowledge_id", d.knowledge.ID).
		Str("knowledge_name", d.knowledge.Name).
		Str("url", d.knowledge.Source.Web.URLs[0]).
		Str("domains", strings.Join(domains, ",")).
		Int32("pages_queued", pageQueueCounter.Load()).
		Msg("finished crawling the website")

	return crawledDocs, nil
}

func (d *Default) crawlWithBrowser(ctx context.Context, b *rod.Browser, url string) (*types.CrawledDocument, error) {

	log.Info().Str("url", url).Msg("crawling with browser")

	start := time.Now()

	page, err := d.browser.GetPage(b, proto.TargetCreateTarget{URL: url})
	if err != nil {
		return nil, fmt.Errorf("error getting page for %s: %w", url, err)
	}
	defer d.browser.PutPage(page)

	if d.knowledge.Source.Web.Crawler.UserAgent != "" {
		if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
			UserAgent: d.knowledge.Source.Web.Crawler.UserAgent,
		}); err != nil {
			return nil, fmt.Errorf("failed setting user agent %v: %v", d.knowledge.Source.Web.Crawler.UserAgent, err)
		}
	}

	log.Trace().Str("url", url).Msg("waiting for page to load")

	e := proto.NetworkResponseReceived{}
	wait := page.WaitEvent(&e)

	err = page.Timeout(d.pageTimeout).WaitLoad()
	if err != nil {
		return nil, fmt.Errorf("error waiting for page to load for %s: %w", url, err)
	}

	wait()

	log.Trace().Str("url", url).Msg("getting page HTML")

	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("error getting HTML for %s: %w", url, err)
	}

	duration := time.Since(start).Milliseconds()

	article, err := d.parser.Parse(ctx, html, url)
	if err != nil {
		return nil, fmt.Errorf("error parsing HTML for %s: %w", url, err)
	}

	doc := &types.CrawledDocument{
		SourceURL:   url,
		Content:     strings.TrimSpace(article.Content),
		Title:       article.Title,
		Description: article.Excerpt,
		StatusCode:  e.Response.Status,
		DurationMs:  duration,
	}

	doc, err = d.convertToMarkdown(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("error converting HTML to markdown for %s: %w", url, err)
	}

	return doc, nil
}

func (d *Default) convertToMarkdown(ctx context.Context, doc *types.CrawledDocument) (*types.CrawledDocument, error) {
	if !d.knowledge.Source.Web.Crawler.Readability {
		markdown, err := d.converter.ConvertString(doc.Content)
		if err != nil {
			return nil, fmt.Errorf("error converting HTML to markdown for %s: %w", doc.SourceURL, err)
		}

		doc.Content = strings.TrimSpace(markdown)

		return doc, nil
	}

	// Pass through readability
	article, err := d.parser.Parse(ctx, doc.Content, doc.SourceURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing HTML for %s: %w", doc.SourceURL, err)
	}

	markdown, err := d.converter.ConvertString(doc.Content)
	if err != nil {
		return nil, fmt.Errorf("error converting HTML to markdown for %s: %w", doc.SourceURL, err)
	}

	doc.Content = strings.TrimSpace(markdown)
	doc.Title = article.Title
	doc.Description = article.Excerpt

	return doc, nil
}
