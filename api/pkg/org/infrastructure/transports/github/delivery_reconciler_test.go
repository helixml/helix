package github

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	githubclient "github.com/helixml/helix/api/pkg/github"
)

func TestReconcileHookDoesNotAdvanceAfterIncompleteScan(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	var mu sync.Mutex
	pageTwoFails := true
	var redeliveries []int64

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/helixml/helix/hooks/42/deliveries", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("cursor") == "page-2" {
			mu.Lock()
			fails := pageTwoFails
			mu.Unlock()
			if fails {
				http.Error(w, "temporary failure", http.StatusInternalServerError)
				return
			}
			writeDeliveries(t, w, []githubclient.WebhookDelivery{
				{ID: 50, GUID: "page-two-success", DeliveredAt: now.Add(-100 * time.Second), StatusCode: 204},
				{ID: 49, GUID: "page-two-failure", DeliveredAt: now.Add(-101 * time.Second), StatusCode: 502},
				{ID: 10000, GUID: "checkpoint", DeliveredAt: now.Add(-102 * time.Second), StatusCode: 204},
			})
			return
		}

		batch := make([]githubclient.WebhookDelivery, 0, 100)
		for id := int64(150); id > 50; id-- {
			status := http.StatusNoContent
			batch = append(batch, githubclient.WebhookDelivery{ID: id, GUID: "delivery-" + strconv.FormatInt(id, 10), DeliveredAt: now.Add(-time.Duration(150-id) * time.Second), StatusCode: status})
		}
		w.Header().Set("Link", `<http://`+req.Host+req.URL.Path+`?cursor=page-2>; rel="next"`)
		writeDeliveries(t, w, batch)
	})
	mux.HandleFunc("/api/v3/repos/helixml/helix/hooks/42/deliveries/", func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(strings.Split(req.URL.Path, "/")[9], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		redeliveries = append(redeliveries, id)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})

	server := httptest.NewServer(mux)
	defer server.Close()
	client := newDeliveryTestClient(t, server)
	reconciler := newDeliveryTestReconciler()
	key := "helixml/helix/42"
	reconciler.checkpointByHook[key] = 10000

	if err := reconciler.reconcileHook(context.Background(), client, "helixml", "helix", 42); err == nil {
		t.Fatal("reconcileHook error = nil, want page 2 error")
	}
	if got := reconciler.checkpointByHook[key]; got != 10000 {
		t.Fatalf("checkpoint = %d, want 10000", got)
	}
	mu.Lock()
	if len(redeliveries) != 0 {
		t.Fatalf("redeliveries after incomplete scan = %v, want none", redeliveries)
	}
	pageTwoFails = false
	mu.Unlock()

	if err := reconciler.reconcileHook(context.Background(), client, "helixml", "helix", 42); err != nil {
		t.Fatal(err)
	}
	if got := reconciler.checkpointByHook[key]; got != 150 {
		t.Fatalf("checkpoint = %d, want 150", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(redeliveries) != 1 || redeliveries[0] != 49 {
		t.Fatalf("redeliveries = %v, want [49]", redeliveries)
	}
}

func TestReconcileHookCheckpointsAcceptedRedeliveries(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	var mu sync.Mutex
	attempts := map[int64]int{}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/helixml/helix/hooks/42/deliveries", func(w http.ResponseWriter, _ *http.Request) {
		writeDeliveries(t, w, []githubclient.WebhookDelivery{
			{ID: 3, GUID: "third", DeliveredAt: now, StatusCode: 500},
			{ID: 2, GUID: "second", DeliveredAt: now.Add(-time.Minute), StatusCode: 500},
			{ID: 1, GUID: "first", DeliveredAt: now.Add(-2 * time.Minute), StatusCode: 500},
		})
	})
	mux.HandleFunc("/api/v3/repos/helixml/helix/hooks/42/deliveries/", func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(strings.Split(req.URL.Path, "/")[9], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		attempts[id]++
		attempt := attempts[id]
		mu.Unlock()
		if id == 2 && attempt == 1 {
			http.Error(w, "temporary failure", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	server := httptest.NewServer(mux)
	defer server.Close()
	client := newDeliveryTestClient(t, server)
	reconciler := newDeliveryTestReconciler()
	key := "helixml/helix/42"

	if err := reconciler.reconcileHook(context.Background(), client, "helixml", "helix", 42); err == nil {
		t.Fatal("reconcileHook error = nil, want redelivery error")
	}
	if got := reconciler.checkpointByHook[key]; got != 1 {
		t.Fatalf("checkpoint after partial failure = %d, want 1", got)
	}
	if err := reconciler.reconcileHook(context.Background(), client, "helixml", "helix", 42); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if attempts[1] != 1 || attempts[2] != 2 || attempts[3] != 1 {
		t.Fatalf("redelivery attempts = %v, want map[1:1 2:2 3:1]", attempts)
	}
	if got := reconciler.checkpointByHook[key]; got != 3 {
		t.Fatalf("checkpoint = %d, want 3", got)
	}
}

func newDeliveryTestClient(t *testing.T, server *httptest.Server) *githubclient.Client {
	t.Helper()
	client, err := githubclient.NewGithubClient(githubclient.ClientOptions{Ctx: context.Background(), Token: "test-token", BaseURL: server.URL + "/"})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func newDeliveryTestReconciler() *DeliveryReconciler {
	return &DeliveryReconciler{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), checkpointByHook: map[string]int64{}}
}

func writeDeliveries(t *testing.T, w http.ResponseWriter, deliveries []githubclient.WebhookDelivery) {
	t.Helper()
	type response struct {
		ID          int64     `json:"id"`
		GUID        string    `json:"guid"`
		DeliveredAt time.Time `json:"delivered_at"`
		StatusCode  int       `json:"status_code"`
	}
	body := make([]response, len(deliveries))
	for i, delivery := range deliveries {
		body[i] = response(delivery)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatal(err)
	}
}
