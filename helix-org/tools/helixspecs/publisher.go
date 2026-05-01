// Package helixspecs implements `tools.SpecsPublisher` against the
// Helix git API. Each call resolves the target repo from the
// Worker's `HelixRepoID` (set by the spawner at hire time) and
// pushes one file onto the configured branch (`helix-specs` by
// default). State files live under `job/` per the Jobs API
// convention — `job/role.md`, `job/identity.md`, etc.
//
// Workers that haven't been hired against a Helix project yet
// (HelixRepoID == "") are no-ops; callers don't have to gate on
// hire status.
package helixspecs

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
	"github.com/helixml/helix-org/tools/helixclient"
)

// Publisher writes job/* files into a Worker's per-project repo.
type Publisher struct {
	client helixclient.Client
	store  *store.Store
	branch string
	author string
	email  string

	// repoLocks serialises pushes to the same repo. Helix's git
	// write path is not concurrency-safe per repo (it pre-syncs,
	// writes, post-pushes against a single working copy on the
	// Helix host). Two simultaneous PutFile calls against the same
	// repo race on the working copy.
	mu        sync.Mutex
	repoLocks map[string]*sync.Mutex
}

// NewPerWorker constructs a Publisher that resolves repo IDs per
// call from the Worker store. branch is the target branch (typically
// "helix-specs"); author/email are the commit metadata.
func NewPerWorker(client helixclient.Client, st *store.Store, branch, author, email string) *Publisher {
	return &Publisher{
		client:    client,
		store:     st,
		branch:    branch,
		author:    author,
		email:     email,
		repoLocks: map[string]*sync.Mutex{},
	}
}

// PublishFile satisfies tools.SpecsPublisher. Returns nil for
// Workers that aren't yet bound to a Helix project — callers don't
// have to gate on hire status.
func (p *Publisher) PublishFile(ctx context.Context, workerID domain.WorkerID, path, content, message string) error {
	if path == "" {
		return errors.New("helixspecs: path is empty")
	}
	worker, err := p.store.Workers.Get(ctx, workerID)
	if err != nil {
		return fmt.Errorf("helixspecs: get worker %q: %w", workerID, err)
	}
	repoID := worker.HelixRepoID()
	if repoID == "" {
		// Worker not yet bound to a Helix project — silently skip.
		// First activation will populate the project and write the
		// canonical files; this branch is for updates that happen
		// before the first activation completes.
		return nil
	}
	if message == "" {
		message = fmt.Sprintf("update %s", path)
	}
	lock := p.lockFor(repoID)
	lock.Lock()
	defer lock.Unlock()
	return p.client.PutFile(ctx, repoID, helixclient.PutFileRequest{
		Path:    path,
		Branch:  p.branch,
		Message: message,
		Author:  p.author,
		Email:   p.email,
		Content: content,
	})
}

func (p *Publisher) lockFor(repoID string) *sync.Mutex {
	p.mu.Lock()
	defer p.mu.Unlock()
	if l, ok := p.repoLocks[repoID]; ok {
		return l
	}
	l := &sync.Mutex{}
	p.repoLocks[repoID] = l
	return l
}
