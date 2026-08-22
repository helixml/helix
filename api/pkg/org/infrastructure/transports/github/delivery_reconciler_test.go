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
	"github.com/helixml/helix/api/pkg/org/domain/aggregate"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
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
	key := deliveryHookKey("helixml", "helix", 42)
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
	key := deliveryHookKey("helixml", "helix", 42)

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

func TestReconcileHookResumesBoundedBacklog(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	var mu sync.Mutex
	listRequests := 0
	attempts := map[int64]int{}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/helixml/helix/hooks/42/deliveries", func(w http.ResponseWriter, req *http.Request) {
		page := 1
		if cursor := req.URL.Query().Get("cursor"); cursor != "" {
			var err error
			page, err = strconv.Atoi(strings.TrimPrefix(cursor, "page-"))
			if err != nil {
				t.Fatal(err)
			}
		}
		mu.Lock()
		listRequests++
		request := listRequests
		mu.Unlock()
		newerID := int64(24 - page*2)
		batch := []githubclient.WebhookDelivery{
			{ID: newerID, GUID: "delivery-" + strconv.FormatInt(newerID, 10), DeliveredAt: now.Add(-time.Duration(page) * time.Minute), StatusCode: 500},
			{ID: newerID - 1, GUID: "delivery-" + strconv.FormatInt(newerID-1, 10), DeliveredAt: now.Add(-time.Duration(page) * time.Minute), StatusCode: 500},
		}
		if page == 2 {
			batch[0].GUID = "page-two-failure"
		}
		if page == 1 && request > deliveryListPageLimit {
			batch = append([]githubclient.WebhookDelivery{{ID: 23, GUID: "page-two-failure", DeliveredAt: now, StatusCode: 204}}, batch...)
		}
		if page < 11 {
			w.Header().Set("Link", `<http://`+req.Host+req.URL.Path+`?cursor=page-`+strconv.Itoa(page+1)+`>; rel="next"`)
		}
		writeDeliveries(t, w, batch)
	})
	mux.HandleFunc("/api/v3/repos/helixml/helix/hooks/42/deliveries/", func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(strings.Split(req.URL.Path, "/")[9], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		attempts[id]++
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})

	server := httptest.NewServer(mux)
	defer server.Close()
	client := newDeliveryTestClient(t, server)
	reconciler := newDeliveryTestReconciler()
	key := deliveryHookKey("helixml", "helix", 42)

	if err := reconciler.reconcileHook(context.Background(), client, "helixml", "helix", 42); err != nil {
		t.Fatal(err)
	}
	if listRequests != deliveryListPageLimit || len(attempts) != 0 || reconciler.checkpointByHook[key] != 0 {
		t.Fatalf("cycle 1: list requests = %d, redeliveries = %d, checkpoint = %d", listRequests, len(attempts), reconciler.checkpointByHook[key])
	}

	if err := reconciler.reconcileHook(context.Background(), client, "helixml", "helix", 42); err != nil {
		t.Fatal(err)
	}
	if listRequests != 12 || len(attempts) != deliveryRedeliveryLimit || attempts[20] != 0 || reconciler.checkpointByHook[key] != 21 {
		t.Fatalf("cycle 2: list requests = %d, redeliveries = %d, stale page-2 attempts = %d, checkpoint = %d", listRequests, len(attempts), attempts[20], reconciler.checkpointByHook[key])
	}

	if err := reconciler.reconcileHook(context.Background(), client, "helixml", "helix", 42); err != nil {
		t.Fatal(err)
	}
	if listRequests != 12 || len(attempts) != 21 || reconciler.checkpointByHook[key] != 23 {
		t.Fatalf("cycle 3: list requests = %d, redeliveries = %d, checkpoint = %d", listRequests, len(attempts), reconciler.checkpointByHook[key])
	}
	for id := int64(1); id <= 22; id++ {
		want := 1
		if id == 20 {
			want = 0
		}
		if attempts[id] != want {
			t.Fatalf("delivery %d attempts = %d, want %d", id, attempts[id], want)
		}
	}
}

func TestReconcileHookProgressesAfterOverlapAtListLimit(t *testing.T) {
	now := time.Now().UTC()
	cycleRequests := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/helixml/helix/hooks/42/deliveries", func(w http.ResponseWriter, req *http.Request) {
		cycleRequests++
		cursor := req.URL.Query().Get("cursor")
		if cursor == "old-page" {
			writeDeliveries(t, w, []githubclient.WebhookDelivery{{ID: 9, GUID: "older", DeliveredAt: now.Add(-time.Hour), StatusCode: 204}})
			return
		}
		page := 1
		if cursor != "" {
			var err error
			page, err = strconv.Atoi(strings.TrimPrefix(cursor, "page-"))
			if err != nil {
				t.Fatal(err)
			}
		}
		id := int64(20 - page)
		w.Header().Set("Link", `<http://`+req.Host+req.URL.Path+`?cursor=page-`+strconv.Itoa(page+1)+`>; rel="next"`)
		writeDeliveries(t, w, []githubclient.WebhookDelivery{{ID: id, GUID: "delivery-" + strconv.FormatInt(id, 10), DeliveredAt: now, StatusCode: 204}})
	})

	server := httptest.NewServer(mux)
	defer server.Close()
	client := newDeliveryTestClient(t, server)
	reconciler := newDeliveryTestReconciler()
	key := deliveryHookKey("helixml", "helix", 42)
	reconciler.scanByHook[key] = &deliveryScan{
		cursor:     "old-page",
		deliveries: []githubclient.WebhookDelivery{{ID: 10, GUID: "overlap", DeliveredAt: now.Add(-time.Minute), StatusCode: 204}},
	}

	if err := reconciler.reconcileHook(context.Background(), client, "helixml", "helix", 42); err != nil {
		t.Fatal(err)
	}
	if cycleRequests != deliveryListPageLimit || reconciler.checkpointByHook[key] != 0 {
		t.Fatalf("cycle 1: list requests = %d, checkpoint = %d", cycleRequests, reconciler.checkpointByHook[key])
	}

	cycleRequests = 0
	if err := reconciler.reconcileHook(context.Background(), client, "helixml", "helix", 42); err != nil {
		t.Fatal(err)
	}
	if cycleRequests != 2 || reconciler.checkpointByHook[key] != 19 {
		t.Fatalf("cycle 2: list requests = %d, checkpoint = %d", cycleRequests, reconciler.checkpointByHook[key])
	}
	if _, ok := reconciler.scanByHook[key]; ok {
		t.Fatal("completed scan was not removed")
	}
}

func TestReconcileCanonicalHookAndCleansStaleState(t *testing.T) {
	var lists, redeliveries int
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/HelixML/Helix/hooks/42/deliveries", func(w http.ResponseWriter, _ *http.Request) {
		lists++
		writeDeliveries(t, w, []githubclient.WebhookDelivery{{ID: 1, GUID: "failed", DeliveredAt: time.Now(), StatusCode: 500}})
	})
	mux.HandleFunc("/api/v3/repos/HelixML/Helix/hooks/42/deliveries/1/attempts", func(w http.ResponseWriter, _ *http.Request) {
		redeliveries++
		w.WriteHeader(http.StatusAccepted)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	st := memory.New()
	for i, repo := range []string{"HelixML/Helix", "helixml/helix"} {
		cfg, err := json.Marshal(transport.GitHubConfig{Repo: repo, Events: []string{"*"}, WebhookID: 42})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Triggers.Create(context.Background(), trigger.Trigger{
			Aggregate: aggregate.Aggregate{
				ID:             "trigger-" + strconv.Itoa(i),
				OrganizationID: "org-test",
				CreatedAt:      time.Now(),
			},
			Name:   "trigger-" + strconv.Itoa(i),
			Kind:   transport.KindGitHub,
			Config: cfg,
		}); err != nil {
			t.Fatal(err)
		}
	}
	tokenFails := false
	reconciler := NewDeliveryReconciler(st, func(context.Context, string) (string, error) {
		if tokenFails {
			return "", io.ErrUnexpectedEOF
		}
		return "token", nil
	}, server.URL+"/", slog.New(slog.NewTextHandler(io.Discard, nil)))
	staleKey := deliveryHookKey("old", "repo", 7)
	reconciler.checkpointByHook[staleKey] = 9
	reconciler.scanByHook[staleKey] = &deliveryScan{cursor: "old"}

	reconciler.reconcile(context.Background())
	if lists != 1 || redeliveries != 1 {
		t.Fatalf("lists = %d, redeliveries = %d, want 1 each", lists, redeliveries)
	}
	if _, ok := reconciler.checkpointByHook[staleKey]; ok {
		t.Fatal("stale checkpoint was not removed")
	}
	if _, ok := reconciler.scanByHook[staleKey]; ok {
		t.Fatal("stale scan was not removed")
	}
	if len(reconciler.checkpointByHook) != 1 {
		t.Fatalf("checkpoints = %v, want one canonical hook", reconciler.checkpointByHook)
	}
	liveKey := deliveryHookKey("HELIXML", "HELIX", 42)
	reconciler.scanByHook[liveKey] = &deliveryScan{cursor: "resume"}
	tokenFails = true
	reconciler.reconcile(context.Background())
	if _, ok := reconciler.scanByHook[liveKey]; !ok {
		t.Fatal("live scan was removed after credential resolution failed")
	}

	for i := range 2 {
		if err := st.Triggers.Delete(context.Background(), "org-test", "trigger-"+strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	tokenFails = false
	reconciler.reconcile(context.Background())
	if len(reconciler.checkpointByHook) != 0 || len(reconciler.scanByHook) != 0 {
		t.Fatalf("orphan state remains: checkpoints = %v, scans = %v", reconciler.checkpointByHook, reconciler.scanByHook)
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
	return &DeliveryReconciler{
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		checkpointByHook: map[string]int64{},
		scanByHook:       map[string]*deliveryScan{},
	}
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
