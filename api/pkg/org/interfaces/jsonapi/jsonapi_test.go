package jsonapi

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestNewDocumentComposesComponents(t *testing.T) {
	data := []Resource{{Type: "messages", ID: "e-1"}}
	doc := NewDocument(data,
		TotalMeta{Total: 7},
		Pagination{Number: 1, Size: 5, Total: 7},
	)
	if doc.Meta["total"] != 7 {
		t.Fatalf("meta.total = %v, want 7", doc.Meta["total"])
	}
	if doc.Meta["total_pages"] != 2 {
		t.Fatalf("meta.total_pages = %v, want 2", doc.Meta["total_pages"])
	}
	if doc.Links["self"] == "" || doc.Links["first"] == "" || doc.Links["last"] == "" {
		t.Fatalf("expected self/first/last links, got %v", doc.Links)
	}
}

func TestNewDocumentNoComponentsOmitsMetaAndLinks(t *testing.T) {
	doc := NewDocument([]Resource{})
	if doc.Meta != nil || doc.Links != nil {
		t.Fatalf("expected nil meta/links, got meta=%v links=%v", doc.Meta, doc.Links)
	}
	b, _ := json.Marshal(doc)
	// data must serialise as [] (not null) and meta/links omitted.
	if got := string(b); got != `{"data":[]}` {
		t.Fatalf("marshalled = %s, want {\"data\":[]}", got)
	}
}

func TestTotalMetaStandalone(t *testing.T) {
	doc := NewDocument(nil, TotalMeta{Total: 0})
	if doc.Meta["total"] != 0 {
		t.Fatalf("meta.total = %v, want 0", doc.Meta["total"])
	}
	if doc.Links != nil {
		t.Fatalf("TotalMeta must not add links, got %v", doc.Links)
	}
}

func TestPaginationLinksFirstPage(t *testing.T) {
	doc := NewDocument(nil, Pagination{Number: 1, Size: 2, Total: 5})
	if _, ok := doc.Links["prev"]; ok {
		t.Fatalf("first page must not have prev: %v", doc.Links)
	}
	if doc.Links["next"] == "" {
		t.Fatalf("first page of 3 must have next: %v", doc.Links)
	}
	if doc.Links["last"] != "?page%5Bnumber%5D=3&page%5Bsize%5D=2" {
		t.Fatalf("last = %q", doc.Links["last"])
	}
}

func TestPaginationLinksMiddlePage(t *testing.T) {
	doc := NewDocument(nil, Pagination{Number: 2, Size: 2, Total: 5})
	if doc.Links["prev"] == "" || doc.Links["next"] == "" {
		t.Fatalf("middle page needs prev and next: %v", doc.Links)
	}
}

func TestPaginationLinksLastPage(t *testing.T) {
	doc := NewDocument(nil, Pagination{Number: 3, Size: 2, Total: 5})
	if _, ok := doc.Links["next"]; ok {
		t.Fatalf("last page must not have next: %v", doc.Links)
	}
	if doc.Links["prev"] == "" {
		t.Fatalf("last page needs prev: %v", doc.Links)
	}
}

func TestPaginationOutOfRangePageNoNext(t *testing.T) {
	// Page beyond the last: no next, self still emitted, total_pages correct.
	doc := NewDocument(nil, Pagination{Number: 99, Size: 2, Total: 5})
	if _, ok := doc.Links["next"]; ok {
		t.Fatalf("out-of-range page must not have next: %v", doc.Links)
	}
	if doc.Meta["total_pages"] != 3 {
		t.Fatalf("total_pages = %v, want 3", doc.Meta["total_pages"])
	}
}

func TestPaginationEmptySet(t *testing.T) {
	doc := NewDocument([]Resource{}, Pagination{Number: 1, Size: 50, Total: 0})
	if doc.Meta["total_pages"] != 0 {
		t.Fatalf("total_pages = %v, want 0", doc.Meta["total_pages"])
	}
	// first/last still point at page 1 so clients have a stable anchor.
	if doc.Links["first"] == "" || doc.Links["last"] == "" {
		t.Fatalf("empty set still needs first/last: %v", doc.Links)
	}
	if _, ok := doc.Links["next"]; ok {
		t.Fatalf("empty set must not have next: %v", doc.Links)
	}
}

func TestPaginationPreservesExtraQuery(t *testing.T) {
	q := url.Values{}
	q.Set("filter", "x")
	q.Set(PageNumberParam, "5") // should be overwritten
	doc := NewDocument(nil, Pagination{Number: 1, Size: 2, Total: 4, Query: q})
	self, err := url.Parse(doc.Links["self"])
	if err != nil {
		t.Fatalf("parse self link: %v", err)
	}
	got := self.Query()
	if got.Get("filter") != "x" {
		t.Fatalf("filter not preserved: %v", got)
	}
	if got.Get(PageNumberParam) != "1" {
		t.Fatalf("page[number] = %q, want 1 (overwritten)", got.Get(PageNumberParam))
	}
}

func TestPageParams(t *testing.T) {
	mk := func(raw string) Page {
		r := httptest.NewRequest("GET", "/x"+raw, nil)
		p, err := PageParams(r, 50, 200)
		if err != nil {
			t.Fatalf("PageParams(%q): %v", raw, err)
		}
		return p
	}
	if p := mk(""); p.Number != 1 || p.Size != 50 {
		t.Fatalf("defaults = %+v, want {1 50}", p)
	}
	if p := mk("?page[number]=3&page[size]=10"); p.Number != 3 || p.Size != 10 {
		t.Fatalf("parsed = %+v, want {3 10}", p)
	}
	if p := mk("?page[size]=9999"); p.Size != 200 {
		t.Fatalf("size cap = %d, want 200", p.Size)
	}
	if p := mk("?page[number]=2"); p.Offset() != 50 || p.Limit() != 50 {
		t.Fatalf("offset/limit = %d/%d, want 50/50", p.Offset(), p.Limit())
	}
}

func TestPageParamsInvalid(t *testing.T) {
	for _, raw := range []string{"?page[number]=0", "?page[number]=-1", "?page[number]=abc", "?page[size]=0", "?page[size]=x"} {
		r := httptest.NewRequest("GET", "/x"+raw, nil)
		if _, err := PageParams(r, 50, 200); err == nil {
			t.Fatalf("PageParams(%q) expected error", raw)
		}
	}
}

func TestWriteSetsContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, 200, NewDocument([]Resource{}))
	if ct := rec.Header().Get("Content-Type"); ct != ContentType {
		t.Fatalf("Content-Type = %q, want %q", ct, ContentType)
	}
}
