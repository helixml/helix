package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/types"
)

func newTestClient(t *testing.T, mux *http.ServeMux) (*HelixClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(mux)
	c, err := NewClient(srv.URL, "test-key", false)
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func TestSpecTaskLifecycle(t *testing.T) {
	var created types.CreateTaskRequest
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/spec-tasks/from-prompt", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer auth")
		}
		if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(types.SpecTask{ID: "task_1", ProjectID: created.ProjectID, Name: created.Name, Status: types.TaskStatusQueuedImplementation})
	})
	mux.HandleFunc("/api/v1/spec-tasks", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("project_id") != "prj_1" || q.Get("include_archived") != "true" || q.Get("sort") != "created" {
			t.Errorf("unexpected list query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]types.SpecTask{{ID: "task_1", ProjectID: "prj_1", Name: "retest abc", Status: types.TaskStatusImplementation}})
	})
	mux.HandleFunc("/api/v1/spec-tasks/task_1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(types.SpecTask{ID: "task_1", ProjectID: "prj_1", Name: "retest abc", Status: types.TaskStatusDone})
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()

	task, err := c.CreateSpecTaskFromPrompt(context.Background(), &types.CreateTaskRequest{
		ProjectID: "prj_1", Prompt: "bounded prompt", Name: "retest abc",
		JustDoItMode: true, AutoStart: true, SandboxRuntime: types.SandboxRuntimeHeadlessUbuntu,
	})
	if err != nil || task.ID != "task_1" {
		t.Fatalf("create: task=%+v err=%v", task, err)
	}
	if !created.JustDoItMode || !created.AutoStart || created.SandboxRuntime != types.SandboxRuntimeHeadlessUbuntu {
		t.Fatalf("request not forwarded: %+v", created)
	}
	found, err := c.FindSpecTaskByName(context.Background(), "prj_1", "retest abc")
	if err != nil || found == nil || found.ID != "task_1" {
		t.Fatalf("find: task=%+v err=%v", found, err)
	}
	if missing, err := c.FindSpecTaskByName(context.Background(), "prj_1", "nope"); err != nil || missing != nil {
		t.Fatalf("find missing: task=%+v err=%v", missing, err)
	}
	got, err := c.GetSpecTask(context.Background(), "task_1")
	if err != nil || got.Status != types.TaskStatusDone {
		t.Fatalf("get: task=%+v err=%v", got, err)
	}
}

func TestFindSpecTaskByNamePaginates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/spec-tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks := make([]types.SpecTask, 0, 200)
		if r.URL.Query().Get("offset") == "" {
			for i := 0; i < 200; i++ {
				tasks = append(tasks, types.SpecTask{ID: "old", ProjectID: "p", Name: "other"})
			}
		} else {
			tasks = append(tasks, types.SpecTask{ID: "wanted", ProjectID: "p", Name: "exact"})
		}
		_ = json.NewEncoder(w).Encode(tasks)
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	task, err := c.FindSpecTaskByName(context.Background(), "p", "exact")
	if err != nil || task == nil || task.ID != "wanted" {
		t.Fatalf("paginated find: task=%+v err=%v", task, err)
	}
}

func TestListSpecTasksRequiresScope(t *testing.T) {
	c, err := NewClient("http://127.0.0.1:1", "k", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListSpecTasks(context.Background(), &SpecTaskFilter{}); err == nil {
		t.Fatal("expected scope error")
	}
}

func TestFilestoreGetAndRead(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/filestore/get", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("path") != "engagements/p/findings.json" {
			t.Errorf("unexpected path: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(filestore.Item{Path: "dev/users/u1/engagements/p/findings.json", Name: "findings.json"})
	})
	mux.HandleFunc("/api/v1/filestore/viewer/dev/users/u1/engagements/p/findings.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer auth")
		}
		_, _ = w.Write([]byte(`[{"id":"F-1"}]`))
	})
	mux.HandleFunc("/api/v1/filestore/viewer/dev/users/u1/missing", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()

	item, err := c.FilestoreGet(context.Background(), "engagements/p/findings.json")
	if err != nil || item.Path != "dev/users/u1/engagements/p/findings.json" {
		t.Fatalf("get: item=%+v err=%v", item, err)
	}
	data, err := c.FilestoreRead(context.Background(), item.Path)
	if err != nil || string(data) != `[{"id":"F-1"}]` {
		t.Fatalf("read: data=%q err=%v", data, err)
	}
	if _, err := c.FilestoreRead(context.Background(), "dev/users/u1/missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
