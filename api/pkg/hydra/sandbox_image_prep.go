package hydra

// On-the-fly sandbox runtime image preparation. The user-facing Sandboxes API
// pulls upstream base images (ubuntu:22.04, node:22-bookworm-slim, …) which
// don't ship `tmux`. The terminal handler installs tmux via apt-get on first
// connect, but that has been failing in some hosts (no apt connectivity from
// the container's network namespace) and falling back to plain bash, which
// breaks session persistence.
//
// Solution: when we first see a base image, run a tiny BuildKit overlay
// (FROM <base> + apt-get/apk install tmux) on the sandbox host's docker
// daemon. Tag the result `helix-sandbox-prep:<sha>` and cache it. Every
// subsequent sandbox of the same runtime starts from the prepared image and
// already has tmux on PATH, so the wrapper script's auto-install path is
// only a fallback for unrecognised distros.
//
// The build runs in the sandbox host's network namespace (which has working
// outbound DNS to apt repos) — this is the key reason it succeeds where the
// in-container apt-get install fails.

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
)

// prepBuildInFlight tracks which base-image overlays are currently being
// built in the background. Without this, every parallel sandbox-create for
// the same fresh runtime would kick off its own duplicate build.
//
// Contents: map[preparedTag]struct{} — the tag is in the map iff a build is
// in progress.
var (
	prepBuildInFlight sync.Map
)

// preparedImageTag returns a deterministic helix-sandbox-prep tag for a
// given upstream base image. The hash makes the tag stable across base-image
// updates (e.g. ubuntu:22.04 getting a new digest from upstream produces a
// different tag, so we rebuild). Length-12 sha1 is plenty for collision
// avoidance within a single host.
func preparedImageTag(baseImage string) string {
	h := sha1.Sum([]byte(baseImage))
	return fmt.Sprintf("helix-sandbox-prep:%s", hex.EncodeToString(h[:6]))
}

// dockerfileForBase generates the overlay Dockerfile body. We try apt first
// (ubuntu/debian/most slim variants), then apk for alpine. If the base image
// uses neither, we return ("", false) and the caller skips the overlay step
// — the wrapper script's runtime install is the fallback for those.
//
// Notes:
//   - --no-install-recommends keeps the layer small; tmux pulls in only its
//     direct deps (~3MB) instead of recommends like locales (~30MB).
//   - ca-certificates is a defensive add: even though the base often has it,
//     having it explicit lets users curl https inside the sandbox.
//   - We point sources.list at the canonical canonical mirrors over HTTPS
//     and allow apt to fall through. The default in the upstream ubuntu image
//     is `archive.ubuntu.com`, which is blackholed from some hosts (sat
//     behind 7+ min of TCP retries before any data flowed) — pinning to a
//     reliable CDN cuts our overlay build time from ~8 minutes to seconds.
//   - The retry loop on apt-get update covers transient 503s without failing
//     the whole build.
func dockerfileForBase(baseImage string) (string, bool) {
	lower := strings.ToLower(baseImage)
	// Order matters: check distro-derivative tags (bookworm/bullseye = debian)
	// before generic "node:"/"python:" since those tags determine which apt
	// mirror config is in the base.
	switch {
	case strings.Contains(lower, "alpine"):
		return fmt.Sprintf(`FROM %s
RUN apk add --no-cache tmux ca-certificates
`, baseImage), true
	case strings.Contains(lower, "debian"),
		strings.Contains(lower, "bookworm"),
		strings.Contains(lower, "bullseye"):
		// Debian uses deb.debian.org by default — generally reliable from
		// most networks, no rewrite needed.
		return fmt.Sprintf(`FROM %s
RUN set -eux; \
    for i in 1 2 3; do apt-get update && break || sleep 2; done; \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends tmux ca-certificates; \
    rm -rf /var/lib/apt/lists/*
`, baseImage), true
	case strings.Contains(lower, "ubuntu"):
		// Ubuntu base: archive.ubuntu.com is blackholed in some networks
		// (we observed 7+ minute TCP timeouts before any byte flowed). Point
		// at a known-good UK mirror so the overlay build completes in seconds.
		// If the env var HELIX_SANDBOX_APT_MIRROR is set, prefer that —
		// operators can set it to a corp/regional mirror.
		mirror := os.Getenv("HELIX_SANDBOX_APT_MIRROR")
		if mirror == "" {
			mirror = "http://mirror.ox.ac.uk/sites/archive.ubuntu.com/ubuntu/"
		}
		return fmt.Sprintf(`FROM %s
RUN set -eux; \
    if [ -f /etc/apt/sources.list ]; then \
      sed -i -E 's#https?://(archive|security)\.ubuntu\.com/ubuntu/?#%s#g' /etc/apt/sources.list || true; \
    fi; \
    for i in 1 2 3; do apt-get update && break || sleep 2; done; \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends tmux ca-certificates; \
    rm -rf /var/lib/apt/lists/*
`, baseImage, mirror), true
	case strings.Contains(lower, "node:"),
		strings.Contains(lower, "python:"),
		strings.Contains(lower, "golang:"),
		strings.Contains(lower, "rust:"):
		// Language slim images on debian — uses deb.debian.org. Treat as
		// debian (no mirror rewrite needed).
		return fmt.Sprintf(`FROM %s
RUN set -eux; \
    for i in 1 2 3; do apt-get update && break || sleep 2; done; \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends tmux ca-certificates; \
    rm -rf /var/lib/apt/lists/*
`, baseImage), true
	default:
		return "", false
	}
}

// buildContextTar packs a single-file build context (just the Dockerfile)
// into a tar buffer. dockerClient.ImageBuild expects a tar reader for the
// build context.
func buildContextTar(dockerfile string) (io.Reader, error) {
	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)
	body := []byte(dockerfile)
	hdr := &tar.Header{
		Name:    "Dockerfile",
		Mode:    0o644,
		Size:    int64(len(body)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := tw.Write(body); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

// prepBuildTimeout caps how long an overlay build may run. apt-get update +
// install tmux on a typical container can take 4–5 minutes (slow apt
// mirrors, package extraction, ca-certificates post-install hooks); 15
// minutes gives plenty of headroom for pathological cases while still
// bounding hangs.
const prepBuildTimeout = 15 * time.Minute

// EnsureSandboxRuntimeImage returns an image tag for the sandbox container.
// On a cache hit it synchronously returns the prepared tag (one with tmux
// pre-installed). On a cache miss it kicks off a *background* build and
// returns the base image immediately — the sandbox-create RPC has a tight
// deadline (~2 min) but the overlay build of ubuntu+tmux takes ~5 min, so
// blocking the caller is not viable. This sandbox falls back to the
// in-container apt-get install path in the terminal wrapper script; the
// next sandbox of the same runtime, started after the build completes, gets
// the prepared tag and skips the install entirely.
//
// On any error (build failure, unsupported package manager, build timeout)
// the caller transparently uses the base image. Logs surface the reason.
func EnsureSandboxRuntimeImage(ctx context.Context, dockerClient *client.Client, baseImage string) string {
	dockerfile, ok := dockerfileForBase(baseImage)
	if !ok {
		log.Debug().Str("base_image", baseImage).Msg("no overlay recipe for base image; using as-is")
		return baseImage
	}

	prepared := preparedImageTag(baseImage)

	// Cache hit — most calls after the first one for each base land here.
	if _, _, err := dockerClient.ImageInspectWithRaw(ctx, prepared); err == nil {
		log.Debug().Str("base", baseImage).Str("prepared", prepared).Msg("using cached prepared sandbox image")
		return prepared
	}

	// Cache miss — kick off a background build if one isn't already running
	// for this tag. LoadOrStore atomically returns whether we just inserted;
	// if loaded==true another goroutine owns the build, we just return the
	// base image and let that other build finish.
	if _, loaded := prepBuildInFlight.LoadOrStore(prepared, struct{}{}); !loaded {
		go runBackgroundOverlayBuild(dockerClient, baseImage, prepared, dockerfile)
	} else {
		log.Debug().Str("base", baseImage).Msg("overlay build already in progress; using base image for this sandbox")
	}
	return baseImage
}

// runBackgroundOverlayBuild executes the actual ImageBuild on its own
// timeout. Always clears the in-flight marker so a failed build can be
// retried by the next sandbox-create.
func runBackgroundOverlayBuild(dockerClient *client.Client, baseImage, prepared, dockerfile string) {
	defer prepBuildInFlight.Delete(prepared)

	log.Info().
		Str("base", baseImage).
		Str("prepared", prepared).
		Msg("starting background sandbox runtime overlay build (adds tmux + ca-certificates)")
	start := time.Now()

	tarReader, err := buildContextTar(dockerfile)
	if err != nil {
		log.Warn().Err(err).Str("base", baseImage).Msg("failed to pack build context; sandbox will keep using base image")
		return
	}

	buildCtx, cancel := context.WithTimeout(context.Background(), prepBuildTimeout)
	defer cancel()

	resp, err := dockerClient.ImageBuild(buildCtx, tarReader, dockertypes.ImageBuildOptions{
		Tags:        []string{prepared},
		Remove:      true,
		ForceRemove: true,
		PullParent:  false, // base was already pulled by the caller
		Dockerfile:  "Dockerfile",
		BuildArgs:   map[string]*string{},
	})
	if err != nil {
		log.Warn().Err(err).Str("base", baseImage).Msg("ImageBuild RPC failed; sandbox will keep using base image")
		return
	}
	defer resp.Body.Close()

	// Drain to completion — ImageBuild only finishes when its streaming body
	// is fully consumed.
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		log.Warn().Err(err).Str("base", baseImage).Msg("error draining build output; sandbox will keep using base image")
		return
	}

	if _, _, err := dockerClient.ImageInspectWithRaw(buildCtx, prepared); err != nil {
		log.Warn().Err(err).Str("base", baseImage).Msg("prepared image not found after build; sandbox will keep using base image")
		return
	}

	log.Info().
		Str("base", baseImage).
		Str("prepared", prepared).
		Dur("duration", time.Since(start)).
		Msg("prepared sandbox runtime image ready (subsequent sandboxes will use it)")
}
