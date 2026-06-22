package jsonapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// Pagination query-parameter names, per the JSON:API recommended
// page-based profile (https://jsonapi.org/format/#fetching-pagination).
const (
	PageNumberParam = "page[number]"
	PageSizeParam   = "page[size]"
)

// Page is the parsed, validated pagination request: a 1-based page
// number and a page size. It carries the offset/limit translation the
// store layer consumes so handlers never recompute it.
type Page struct {
	Number int
	Size   int
}

// Offset is the number of rows to skip for this page.
func (p Page) Offset() int {
	if p.Number < 1 {
		return 0
	}
	return (p.Number - 1) * p.Size
}

// Limit is the row cap for this page.
func (p Page) Limit() int { return p.Size }

// PageParams parses page[number]/page[size] off the request. number
// defaults to 1, size to defaultSize and is capped at maxSize (when
// maxSize > 0). Non-numeric or out-of-range (< 1) values are a 400-class
// error the handler surfaces — an out-of-range *page* (past the last
// page) is NOT an error here; it simply yields an empty data array.
func PageParams(r *http.Request, defaultSize, maxSize int) (Page, error) {
	q := r.URL.Query()
	number := 1
	if v := q.Get(PageNumberParam); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Page{}, fmt.Errorf("invalid %s %q: must be a positive integer", PageNumberParam, v)
		}
		number = n
	}
	size := defaultSize
	if v := q.Get(PageSizeParam); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Page{}, fmt.Errorf("invalid %s %q: must be a positive integer", PageSizeParam, v)
		}
		size = n
	}
	if maxSize > 0 && size > maxSize {
		size = maxSize
	}
	return Page{Number: number, Size: size}, nil
}

// Pagination is a Component that contributes page-based JSON:API links
// (self/first/prev/next/last) and pagination meta (page/size/total_pages)
// to a Document. It is independent of TotalMeta — compose either or both.
//
// Links are emitted as query-only relative references (e.g.
// "?page[number]=2&page[size]=50"). Per RFC 3986 §5 these resolve
// against the request URL, so they are correct whether the org API is
// served standalone or mounted behind the helix-embedded prefix-
// stripping middleware — neither path needs to know its mount point.
type Pagination struct {
	Number int        // 1-based current page
	Size   int        // page size
	Total  int        // total items across all pages
	Query  url.Values // request query to preserve in links (page[*] overwritten); may be nil
}

// Apply writes the pagination links and meta onto the document.
func (p Pagination) Apply(d *Document) {
	size := p.Size
	if size < 1 {
		size = 1
	}
	totalPages := (p.Total + size - 1) / size

	links := d.ensureLinks()
	links["self"] = p.link(p.Number)
	links["first"] = p.link(1)
	last := totalPages
	if last < 1 {
		last = 1
	}
	links["last"] = p.link(last)
	if p.Number > 1 {
		links["prev"] = p.link(p.Number - 1)
	}
	if p.Number < totalPages {
		links["next"] = p.link(p.Number + 1)
	}

	meta := d.ensureMeta()
	meta["page"] = p.Number
	meta["size"] = p.Size
	meta["total_pages"] = totalPages
}

// link builds the query-only relative reference for the given page,
// preserving any non-pagination query params.
func (p Pagination) link(number int) string {
	q := url.Values{}
	for k, vs := range p.Query {
		if k == PageNumberParam || k == PageSizeParam {
			continue
		}
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	q.Set(PageNumberParam, strconv.Itoa(number))
	q.Set(PageSizeParam, strconv.Itoa(p.Size))
	return "?" + q.Encode()
}
