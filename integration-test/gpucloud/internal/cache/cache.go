// Package cache stores the (entry-id, profile-yaml-sha) -> green-result
// mapping so re-runs of the harness can skip unchanged matrix entries.
//
// On-disk format: one JSON file per cache key under cacheDir. Lifetime
// is bounded by the file system (no eviction policy yet — manual prune
// or rm the directory). Plenty for the current matrix size.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

// Entry is the on-disk record for one (key, result) pair.
type Entry struct {
	Passed     bool      `json:"passed"`
	RecordedAt time.Time `json:"recorded_at"`
	HarnessSHA string    `json:"harness_sha"`
}

// Cache is a tiny file-system cache.
type Cache struct {
	dir string
}

// New returns a cache rooted at dir. Creates dir if it doesn't exist.
func New(dir string) *Cache {
	_ = os.MkdirAll(dir, 0o755)
	return &Cache{dir: dir}
}

// Key derives a stable cache key from the entry ID + profile YAML SHA +
// the harness's own build SHA (so re-runs after harness code changes
// invalidate the cache automatically).
func (c *Cache) Key(entryID, profilePath string) (string, error) {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return "", fmt.Errorf("read profile: %w", err)
	}
	h := sha256.New()
	h.Write([]byte(entryID))
	h.Write([]byte{0})
	h.Write(data)
	h.Write([]byte{0})
	h.Write([]byte(harnessSHA()))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Lookup returns the cached entry if present + still considered fresh
// (≤ 7 days). Stale entries are reported as missing so the harness re-runs
// them eventually even on stable hardware.
func (c *Cache) Lookup(key string) (Entry, bool) {
	path := filepath.Join(c.dir, key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, false
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return Entry{}, false
	}
	if time.Since(e.RecordedAt) > 7*24*time.Hour {
		return Entry{}, false
	}
	return e, true
}

// Record persists a fresh entry, overwriting any prior record.
func (c *Cache) Record(key string, e Entry) {
	if e.HarnessSHA == "" {
		e.HarnessSHA = harnessSHA()
	}
	data, _ := json.MarshalIndent(e, "", "  ")
	_ = os.WriteFile(filepath.Join(c.dir, key+".json"), data, 0o644)
}

// harnessSHA returns the harness's own build SHA — pulled from the Go
// build info embedded VCS revision. Falls back to "unknown" when the
// binary wasn't built from a git checkout.
func harnessSHA() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return "unknown"
}
