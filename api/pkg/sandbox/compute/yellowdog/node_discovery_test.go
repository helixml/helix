package yellowdog

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestDiscoverNodePools(t *testing.T) {
	cases := []struct {
		name     string
		response string
		want     []NodePool
	}{
		{
			name:     "no nodes",
			response: `{"items":[]}`,
			want:     []NodePool{},
		},
		{
			name: "two pools of different instance types grouped and counted",
			response: `{"items":[
				{"id":"n1","status":"RUNNING","details":{"workerTag":"worker-nvidia","instanceType":"g5.xlarge"}},
				{"id":"n2","status":"RUNNING","details":{"workerTag":"worker-nvidia","instanceType":"g5.xlarge"}},
				{"id":"n3","status":"RUNNING","details":{"workerTag":"worker-inf2","instanceType":"inf2.8xlarge"}}
			]}`,
			want: []NodePool{
				{WorkerTag: "worker-inf2", InstanceType: "inf2.8xlarge", NodeCount: 1},
				{WorkerTag: "worker-nvidia", InstanceType: "g5.xlarge", NodeCount: 2},
			},
		},
		{
			name: "same tag different instance types are distinct pools",
			response: `{"items":[
				{"id":"n1","status":"RUNNING","details":{"workerTag":"worker-mix","instanceType":"g5.xlarge"}},
				{"id":"n2","status":"RUNNING","details":{"workerTag":"worker-mix","instanceType":"inf2.8xlarge"}}
			]}`,
			want: []NodePool{
				{WorkerTag: "worker-mix", InstanceType: "g5.xlarge", NodeCount: 1},
				{WorkerTag: "worker-mix", InstanceType: "inf2.8xlarge", NodeCount: 1},
			},
		},
		{
			name: "nodes without a worker tag are dropped",
			response: `{"items":[
				{"id":"n1","status":"RUNNING","details":{"workerTag":"","instanceType":"g5.xlarge"}},
				{"id":"n2","status":"RUNNING","details":{"workerTag":"worker-real","instanceType":"g5.xlarge"}}
			]}`,
			want: []NodePool{
				{WorkerTag: "worker-real", InstanceType: "g5.xlarge", NodeCount: 1},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeServer(t)
			f.handler = func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/workerPools/nodes" {
					t.Errorf("path = %q, want /workerPools/nodes", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			got, err := DiscoverNodePools(ctx, Config{
				APIKeyID:             "k",
				APISecret:            "s",
				BaseURL:              f.srv.URL,
				HTTPClient:           f.srv.Client(),
				AllowInsecureBaseURL: true,
			})
			if err != nil {
				t.Fatalf("DiscoverNodePools: %v", err)
			}
			if len(got) == 0 {
				got = []NodePool{}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDiscoverNodePoolsRequiresCreds(t *testing.T) {
	_, err := DiscoverNodePools(context.Background(), Config{
		BaseURL: "https://portal.yellowdog.co/api",
	})
	if err == nil {
		t.Fatal("expected error for missing APIKeyID/APISecret")
	}
}

func TestDiscoverNodePoolsRefusesPlaintextURL(t *testing.T) {
	_, err := DiscoverNodePools(context.Background(), Config{
		APIKeyID:  "k",
		APISecret: "s",
		BaseURL:   "http://example.com/api",
	})
	if err == nil {
		t.Fatal("expected error for non-https BaseURL without AllowInsecureBaseURL")
	}
}

func TestDiscoverNodePoolsPropagatesServerError(t *testing.T) {
	f := newFakeServer(t)
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := DiscoverNodePools(ctx, Config{
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
