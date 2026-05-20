package services

import (
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// defaultPRListCacheTTL is the default TTL for cached external PR list results.
// 60s absorbs a 30s orchestrator poll cycle plus coincident user-driven requests
// while keeping PR state visibly fresh in the UI.
const defaultPRListCacheTTL = 60 * time.Second

// prListCacheEntry holds either a successful PR list result or a cached error
// (used to back off on rate-limit responses without re-hitting the upstream).
type prListCacheEntry struct {
	prs       []*types.PullRequest
	err       error
	expiresAt time.Time
}

// prListCache is an in-process TTL cache for ListPullRequests results, keyed by
// repository ID. Successful results are cached for prListCache.ttl; rate-limit
// errors are cached until their reset time (set explicitly via setError).
type prListCache struct {
	mu      sync.Mutex
	entries map[string]prListCacheEntry
	ttl     time.Duration
}

func newPRListCache(ttl time.Duration) *prListCache {
	if ttl <= 0 {
		ttl = defaultPRListCacheTTL
	}
	return &prListCache{
		entries: make(map[string]prListCacheEntry),
		ttl:     ttl,
	}
}

// get returns the cached entry for repoID and true if it is still fresh,
// otherwise the zero entry and false.
func (c *prListCache) get(repoID string) ([]*types.PullRequest, error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[repoID]
	if !ok {
		return nil, nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.entries, repoID)
		return nil, nil, false
	}
	return entry.prs, entry.err, true
}

// set caches a successful PR list result for the default TTL.
func (c *prListCache) set(repoID string, prs []*types.PullRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[repoID] = prListCacheEntry{
		prs:       prs,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// setError caches an error result until expiresAt. Used to back off on
// rate-limit errors so we don't re-hit the upstream until the limit resets.
func (c *prListCache) setError(repoID string, err error, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[repoID] = prListCacheEntry{
		err:       err,
		expiresAt: expiresAt,
	}
}

// invalidate drops any cached entry for repoID. Call after a mutating
// operation (e.g. CreatePullRequest) so the next read sees fresh state.
func (c *prListCache) invalidate(repoID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, repoID)
}
