package services

import (
	"testing"

	"github.com/helixml/kodit/domain/enrichment"
)

// Compile-time interface check.
var _ KoditServicer = (*KoditService)(nil)

func TestNewKoditService_NilClient(t *testing.T) {
	if NewKoditService(nil).IsEnabled() {
		t.Error("expected disabled when client is nil")
	}
	var nilSvc *KoditService
	if nilSvc.IsEnabled() {
		t.Error("nil receiver must return false")
	}
}

func TestDisabledServiceMethods(t *testing.T) {
	svc := NewKoditService(nil)
	ctx := t.Context()

	// RegisterRepository returns an error when disabled.
	id, isNew, err := svc.RegisterRepository(ctx, &RegisterRepositoryParams{CloneURL: "https://example.com/repo.git"})
	if err == nil || id != 0 || isNew {
		t.Errorf("RegisterRepository: want (0, false, error), got (%d, %v, %v)", id, isNew, err)
	}

	// All other methods error when disabled.
	for _, tc := range []struct {
		name string
		fn   func() error
	}{
		{"GetRepositoryEnrichments", func() error { _, e := svc.GetRepositoryEnrichments(ctx, 1, "", ""); return e }},
		{"GetEnrichment", func() error { _, e := svc.GetEnrichment(ctx, "1"); return e }},
		{"GetRepositoryCommits", func() error { _, e := svc.GetRepositoryCommits(ctx, 1, 10); return e }},
		{"SearchSnippets", func() error { _, e := svc.SearchSnippets(ctx, 1, "test", 10); return e }},
		{"GetRepositoryStatus", func() error { _, e := svc.GetRepositoryStatus(ctx, 1); return e }},
		{"RescanCommit", func() error { return svc.RescanCommit(ctx, 1, "abc123") }},
		{"DeleteRepository", func() error { return svc.DeleteRepository(ctx, 1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.fn() == nil {
				t.Error("expected error from disabled service")
			}
		})
	}
}

// Input-validation tests that previously forced enabled=true without a real
// client are gone: the service now derives enabled state from the presence of
// the underlying *kodit.Client, so exercising validation paths requires a
// live client (covered by integration tests).

func TestEnrichmentFiltering(t *testing.T) {
	all := []enrichment.Enrichment{
		enrichment.NewSnippetEnrichment("code"),
		enrichment.NewSnippetSummary("summary"),
		enrichment.NewExampleSummary("example"),
		enrichment.NewCookbook("cookbook"),
		enrichment.NewPhysicalArchitecture("arch"),
	}
	var kept int
	for _, e := range all {
		if e.Subtype() != enrichment.SubtypeSnippetSummary && e.Subtype() != enrichment.SubtypeExampleSummary {
			kept++
		}
	}
	if kept != 3 {
		t.Errorf("expected 3 after filtering, got %d", kept)
	}
}
