package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/client"
)

func TestLookupProjectResolvesOrganizationProjectName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/projects" && r.URL.Query().Get("organization_id") == "":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/api/v1/organizations":
			_, _ = w.Write([]byte(`[{"id":"org_test","name":"Acme"}]`))
		case r.URL.Path == "/api/v1/projects" && r.URL.Query().Get("organization_id") == "org_test":
			_, _ = w.Write([]byte(`[{"id":"prj_test","name":"CLI project","organization_id":"org_test"}]`))
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	apiClient, err := client.NewClient(server.URL, "test-token", false)
	if err != nil {
		t.Fatal(err)
	}

	project, err := LookupProject(t.Context(), apiClient, "CLI project", "")
	if err != nil {
		t.Fatalf("LookupProject returned an error: %v", err)
	}
	if project.ID != "prj_test" {
		t.Fatalf("project ID = %q, want prj_test", project.ID)
	}
}

func TestLookupProjectRejectsAmbiguousNames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/projects" && r.URL.Query().Get("organization_id") == "":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/api/v1/organizations":
			_, _ = w.Write([]byte(`[{"id":"org_a","name":"A"},{"id":"org_b","name":"B"}]`))
		case r.URL.Path == "/api/v1/projects" && r.URL.Query().Get("organization_id") == "org_a":
			_, _ = w.Write([]byte(`[{"id":"prj_a","name":"Duplicate"}]`))
		case r.URL.Path == "/api/v1/projects" && r.URL.Query().Get("organization_id") == "org_b":
			_, _ = w.Write([]byte(`[{"id":"prj_b","name":"Duplicate"}]`))
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	apiClient, err := client.NewClient(server.URL, "test-token", false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LookupProject(t.Context(), apiClient, "Duplicate", "")
	if err == nil || !containsAll(err.Error(), "ambiguous", "prj_a", "prj_b") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
