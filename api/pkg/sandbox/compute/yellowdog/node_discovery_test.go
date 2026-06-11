package yellowdog

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDiscoverOnlineWorkerTags(t *testing.T) {
	cases := []struct {
		name     string
		response string
		want     []string
	}{
		{
			name:     "no nodes",
			response: `{"items":[]}`,
			want:     []string{},
		},
		{
			name: "single tag across multiple nodes is deduplicated",
			response: `{"items":[
				{"id":"n1","status":"RUNNING","details":{"workerTag":"worker-psamuel"}},
				{"id":"n2","status":"RUNNING","details":{"workerTag":"worker-psamuel"}}
			]}`,
			want: []string{"worker-psamuel"},
		},
		{
			name: "multiple distinct tags returned sorted",
			response: `{"items":[
				{"id":"n1","status":"RUNNING","details":{"workerTag":"worker-staging"}},
				{"id":"n2","status":"RUNNING","details":{"workerTag":"worker-prod"}}
			]}`,
			want: []string{"worker-prod", "worker-staging"},
		},
		{
			name: "empty workerTag fields are dropped",
			response: `{"items":[
				{"id":"n1","status":"RUNNING","details":{"workerTag":""}},
				{"id":"n2","status":"RUNNING","details":{"workerTag":"worker-real"}}
			]}`,
			want: []string{"worker-real"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeServer(t)
			f.handler = func(w http.ResponseWriter, r *http.Request) {
				// Must hit the right endpoint with the right query
				// shape. If this assertion ever fails it means we
				// regressed away from the public OpenAPI contract.
				if r.URL.Path != "/workerPools/nodes" {
					t.Errorf("path = %q, want /workerPools/nodes", r.URL.Path)
				}
				if r.Method != http.MethodGet {
					t.Errorf("method = %q, want GET", r.Method)
				}
				q := r.URL.Query()
				if q.Get("nodeSearch") == "" {
					t.Error("nodeSearch query param missing - YD will 400")
				}
				if q.Get("sliceReference") == "" {
					t.Error("sliceReference query param missing - YD will 400")
				}
				// Only RUNNING nodes should be requested.
				if !strings.Contains(q.Get("nodeSearch"), "RUNNING") {
					t.Errorf("nodeSearch should filter to RUNNING, got %q", q.Get("nodeSearch"))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			got, err := DiscoverOnlineWorkerTags(ctx, Config{
				APIKeyID:             "k",
				APISecret:            "s",
				BaseURL:              f.srv.URL,
				HTTPClient:           f.srv.Client(),
				AllowInsecureBaseURL: true,
			})
			if err != nil {
				t.Fatalf("DiscoverOnlineWorkerTags: %v", err)
			}
			// Treat nil and empty slice as equivalent for this comparison.
			if got == nil {
				got = []string{}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDiscoverOnlineWorkerTagsRequiresCreds(t *testing.T) {
	_, err := DiscoverOnlineWorkerTags(context.Background(), Config{
		BaseURL: "https://portal.yellowdog.co/api",
	})
	if err == nil {
		t.Fatal("expected error for missing APIKeyID/APISecret")
	}
}

func TestDiscoverOnlineWorkerTagsRefusesPlaintextURL(t *testing.T) {
	_, err := DiscoverOnlineWorkerTags(context.Background(), Config{
		APIKeyID:  "k",
		APISecret: "s",
		BaseURL:   "http://example.com/api",
	})
	if err == nil {
		t.Fatal("expected error for non-https BaseURL without AllowInsecureBaseURL")
	}
}

func TestDiscoverOnlineWorkerTagsPropagatesServerError(t *testing.T) {
	f := newFakeServer(t)
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := DiscoverOnlineWorkerTags(ctx, Config{
		APIKeyID:             "k",
		APISecret:            "s",
		BaseURL:              f.srv.URL,
		HTTPClient:           f.srv.Client(),
		AllowInsecureBaseURL: true,
	})
	if err == nil {
		t.Fatal("expected error when YD returns 500")
	}
}

