// Package opencode resolves pinned opencode releases into downloadable,
// integrity-checked artifacts for the desktop container.
//
// The desktop image bakes a known-good opencode build (BakedVersion). An admin
// can pin a newer release from system settings without rebuilding the image;
// this package turns that version string into concrete per-architecture
// archive URLs plus their SHA256 digests, so the container never has to know
// the release URL scheme and never runs an unverified download.
package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"golang.org/x/sync/singleflight"
)

// BakedVersion is the opencode release compiled into the desktop image.
//
// It MUST be kept in sync with OPENCODE_VERSION in sandbox-versions.txt and
// the ARG OPENCODE_VERSION default in Dockerfile.ubuntu-helix. It is used only
// to validate admin input ("the override must be newer than what ships"); the
// settings-sync-daemon treats /opt/helix/opencode.version — written at image
// build time — as the authoritative floor, so a drift between this constant
// and a deployed image cannot cause the wrong binary to run.
const BakedVersion = "1.18.18"

// DefaultReleasesURL is the GitHub releases API base for opencode. Operators
// running air-gapped can point this at an internal mirror that serves the same
// JSON shape (see config.Sandboxes.OpenCodeReleasesURL).
const DefaultReleasesURL = "https://api.github.com/repos/anomalyco/opencode/releases"

// assetsByArch maps GOARCH to the release asset name we want. opencode
// publishes glibc, musl and "baseline" variants; the desktop image is
// glibc on modern hardware, so we take the plain build.
var assetsByArch = map[string]string{
	"amd64": "opencode-linux-x64.tar.gz",
	"arm64": "opencode-linux-arm64.tar.gz",
}

// versionPattern is deliberately strict: the version becomes part of a release
// tag in an outbound URL, so anything but a bare semver is rejected rather
// than escaped.
var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// ValidateVersion checks that v is a bare semver newer than the baked release.
// An empty string is valid and means "use the version baked into the image".
//
// Downgrades are rejected on purpose: this setting exists to roll forward
// between image builds. Pinning an older build belongs in the image, where it
// gets the same CI coverage as everything else.
func ValidateVersion(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if !versionPattern.MatchString(v) {
		return fmt.Errorf("opencode version %q must be a bare semver like %q", v, BakedVersion)
	}
	newer, err := isNewer(v, BakedVersion)
	if err != nil {
		return err
	}
	if !newer {
		return fmt.Errorf("opencode version %q is not newer than the bundled version %s; leave the override blank to use the bundled build", v, BakedVersion)
	}
	return nil
}

// isNewer reports whether version a is strictly greater than b. Both must
// already have passed versionPattern.
func isNewer(a, b string) (bool, error) {
	aParts, err := parseVersion(a)
	if err != nil {
		return false, err
	}
	bParts, err := parseVersion(b)
	if err != nil {
		return false, err
	}
	for i := range aParts {
		if aParts[i] != bParts[i] {
			return aParts[i] > bParts[i], nil
		}
	}
	return false, nil
}

func parseVersion(v string) ([3]int, error) {
	var out [3]int
	fields := strings.Split(v, ".")
	if len(fields) != 3 {
		return out, fmt.Errorf("invalid opencode version %q", v)
	}
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return out, fmt.Errorf("invalid opencode version %q", v)
		}
		out[i] = n
	}
	return out, nil
}

// githubRelease is the subset of the releases API we consume. The digest field
// is authoritative — it is the same value the ACP registry publishes.
type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
	} `json:"assets"`
}

type cacheEntry struct {
	binary  *types.CodeAgentBinary
	fetched time.Time
}

// Resolver turns a pinned version into per-architecture artifacts. Results are
// cached because a release is immutable once published; without the cache
// every session start would hit the releases API.
type Resolver struct {
	httpClient  *http.Client
	releasesURL string
	ttl         time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry
	fetch singleflight.Group
}

// NewResolver builds a Resolver. Pass an empty releasesURL to use the public
// GitHub API.
func NewResolver(httpClient *http.Client, releasesURL string) *Resolver {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if releasesURL == "" {
		releasesURL = DefaultReleasesURL
	}
	return &Resolver{
		httpClient:  httpClient,
		releasesURL: strings.TrimSuffix(releasesURL, "/"),
		ttl:         time.Hour,
		cache:       map[string]cacheEntry{},
	}
}

// Resolve looks up the release for version and returns its linux artifacts.
// It fails rather than returning a partial result when an architecture we
// support is missing: a half-resolved release would start sessions on one
// sandbox host and break them on another.
func (r *Resolver) Resolve(ctx context.Context, version string) (*types.CodeAgentBinary, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil, nil
	}
	if !versionPattern.MatchString(version) {
		return nil, fmt.Errorf("opencode version %q must be a bare semver", version)
	}

	r.mu.Lock()
	if entry, ok := r.cache[version]; ok && time.Since(entry.fetched) < r.ttl {
		r.mu.Unlock()
		return entry.binary, nil
	}
	r.mu.Unlock()

	result, err, _ := r.fetch.Do(version, func() (interface{}, error) {
		r.mu.Lock()
		if entry, ok := r.cache[version]; ok && time.Since(entry.fetched) < r.ttl {
			r.mu.Unlock()
			return entry.binary, nil
		}
		r.mu.Unlock()

		binary, err := r.fetchRelease(ctx, version)
		if err != nil {
			return nil, err
		}

		r.mu.Lock()
		r.cache[version] = cacheEntry{binary: binary, fetched: time.Now()}
		r.mu.Unlock()
		return binary, nil
	})
	if err != nil {
		return nil, err
	}
	binary, ok := result.(*types.CodeAgentBinary)
	if !ok {
		return nil, fmt.Errorf("opencode resolver returned unexpected result type %T", result)
	}
	return binary, nil
}

func (r *Resolver) fetchRelease(ctx context.Context, version string) (*types.CodeAgentBinary, error) {
	url := fmt.Sprintf("%s/tags/v%s", r.releasesURL, version)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build opencode release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach the opencode release index at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("opencode version %s does not exist upstream", version)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("opencode release index returned status %d for version %s", resp.StatusCode, version)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse the opencode release index: %w", err)
	}

	artifacts := map[string]types.CodeAgentBinaryArtifact{}
	for arch, assetName := range assetsByArch {
		for _, asset := range release.Assets {
			if asset.Name != assetName {
				continue
			}
			digest := strings.TrimPrefix(asset.Digest, "sha256:")
			if digest == "" || digest == asset.Digest {
				return nil, fmt.Errorf("opencode %s asset %s has no sha256 digest; refusing to ship an unverifiable binary", version, assetName)
			}
			if asset.URL == "" {
				return nil, fmt.Errorf("opencode %s asset %s has no download URL", version, assetName)
			}
			artifacts[arch] = types.CodeAgentBinaryArtifact{URL: asset.URL, SHA256: digest}
			break
		}
		if _, ok := artifacts[arch]; !ok {
			return nil, fmt.Errorf("opencode %s publishes no %s build (expected asset %s)", version, arch, assetName)
		}
	}

	return &types.CodeAgentBinary{Version: version, Artifacts: artifacts}, nil
}
