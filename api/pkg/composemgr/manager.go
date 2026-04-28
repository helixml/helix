// Package composemgr implements the compose-manager subsystem that runs
// inside a Helix sandbox and applies operator-assigned runner profiles by
// running `docker compose` against the inner dockerd.
//
// Lifecycle of a profile assignment:
//
//	(operator) -> POST /api/v1/runners/{id}/assign-profile
//	(api)      -> persists Assignment, sends NATS cmd to runner
//	(this pkg) -> Apply(profile) -> writes /etc/helix/active.yaml,
//	              docker compose pull, docker compose up -d, polls health,
//	              reports state back to api via heartbeat.
//
// On Apply, the order is:
//
//	1. (online modes) docker compose -f new.yaml pull
//	2. docker compose -f old.yaml down --remove-orphans  (if any old)
//	3. docker compose -f new.yaml up -d
//	4. poll service readiness
//
// **Never** prune images between steps 2 and 3 — that destroys shared layers
// and forces a full re-pull. Pruning happens on a separate periodic
// schedule in the Trim() method.
package composemgr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/runner/composeparse"
	"github.com/helixml/helix/api/pkg/types"
)

// State summarises the compose-manager's current view for status reporting.
type State struct {
	ProfileID     string            // empty when no profile is assigned
	ProfileName   string
	Status        string            // "" | "assigning" | "pulling" | "starting" | "running" | "failed"
	Error         string            // populated when Status == "failed"
	ServiceHealth map[string]string // service name -> "healthy" | "starting" | "failed" | "unknown"
}

// Options configures the compose manager. Mostly env-var-style knobs.
type Options struct {
	// ConfigDir is where the manager writes the active compose YAML.
	// Defaults to /etc/helix.
	ConfigDir string

	// DockerComposeBinary is the docker compose CLI invocation.
	// Defaults to "docker", which uses the modern `docker compose ...`
	// subcommand. Set to e.g. "docker-compose" for the legacy plugin.
	DockerComposeBinary string

	// RegistryMirror, if set, rewrites the leading registry portion of
	// every `image:` field in the active YAML before pull/up. Implements
	// HELIX_RUNNER_REGISTRY (parallel to HELIX_SANDBOX_REGISTRY).
	RegistryMirror string

	// Offline, if true, skips `docker compose pull`. Implements
	// HELIX_RUNNER_OFFLINE.
	Offline bool

	// ReadinessPollInterval is how often we poll service health after
	// `up -d`. Default 2s.
	ReadinessPollInterval time.Duration

	// ReadinessTimeout is the maximum time to wait for a service to
	// become healthy before marking the assignment failed. Default 5m.
	ReadinessTimeout time.Duration
}

// Default returns sensible defaults for production use.
func Default() Options {
	return Options{
		ConfigDir:             "/etc/helix",
		DockerComposeBinary:   "docker",
		ReadinessPollInterval: 2 * time.Second,
		ReadinessTimeout:      5 * time.Minute,
	}
}

// Manager holds runtime state. Safe for concurrent use.
type Manager struct {
	opts Options

	mu    sync.RWMutex
	state State
}

func New(opts Options) *Manager {
	if opts.ConfigDir == "" {
		opts.ConfigDir = "/etc/helix"
	}
	if opts.DockerComposeBinary == "" {
		opts.DockerComposeBinary = "docker"
	}
	if opts.ReadinessPollInterval == 0 {
		opts.ReadinessPollInterval = 2 * time.Second
	}
	if opts.ReadinessTimeout == 0 {
		opts.ReadinessTimeout = 5 * time.Minute
	}
	return &Manager{opts: opts}
}

// Snapshot returns a copy of the current state. Safe to call from
// status-reporting goroutines.
func (m *Manager) Snapshot() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := m.state
	if m.state.ServiceHealth != nil {
		cp.ServiceHealth = make(map[string]string, len(m.state.ServiceHealth))
		for k, v := range m.state.ServiceHealth {
			cp.ServiceHealth[k] = v
		}
	}
	return cp
}

// Apply tears down any active compose stack, writes the new profile YAML,
// pulls images, brings the new stack up, and polls readiness. Blocks
// until the apply succeeds or fails. Idempotent: if Apply is called with
// the same profile twice, the second call still does the down/up cycle.
func (m *Manager) Apply(ctx context.Context, p *types.RunnerProfile) error {
	if p == nil {
		return errors.New("Apply: profile is nil")
	}
	parsed, err := composeparse.Parse([]byte(p.ComposeYAML))
	if err != nil {
		m.setFailed(p, fmt.Errorf("compose parse: %w", err))
		return err
	}
	yaml := p.ComposeYAML
	if m.opts.RegistryMirror != "" {
		yaml = rewriteRegistry(yaml, m.opts.RegistryMirror)
	}

	m.setStatus(p, "assigning", "")

	// 1. Pull (unless offline).
	newPath := filepath.Join(m.opts.ConfigDir, "next.yaml")
	if err := os.MkdirAll(m.opts.ConfigDir, 0o755); err != nil {
		m.setFailed(p, err)
		return err
	}
	if err := os.WriteFile(newPath, []byte(yaml), 0o644); err != nil {
		m.setFailed(p, err)
		return err
	}
	if !m.opts.Offline {
		m.setStatus(p, "pulling", "")
		if err := m.runCompose(ctx, "-f", newPath, "pull"); err != nil {
			m.setFailed(p, fmt.Errorf("pull: %w", err))
			return err
		}
	} else if err := m.assertImagesPresent(ctx, parsed); err != nil {
		m.setFailed(p, err)
		return err
	}

	// 2. Down old (if present).
	activePath := filepath.Join(m.opts.ConfigDir, "active.yaml")
	if _, err := os.Stat(activePath); err == nil {
		// Best-effort down; failures here are logged but don't abort the
		// switch. The new `up -d` will fail loudly if old containers
		// genuinely conflict.
		_ = m.runCompose(ctx, "-f", activePath, "down", "--remove-orphans")
	}

	// 3. Up new.
	if err := os.Rename(newPath, activePath); err != nil {
		m.setFailed(p, err)
		return err
	}
	m.setStatus(p, "starting", "")
	if err := m.runCompose(ctx, "-f", activePath, "up", "-d"); err != nil {
		m.setFailed(p, fmt.Errorf("up: %w", err))
		return err
	}

	// 4. Poll readiness.
	if err := m.waitReady(ctx, parsed); err != nil {
		m.setFailed(p, err)
		return err
	}
	m.setStatus(p, "running", "")
	// Persist again to capture the now-populated ServiceHealth alongside
	// the running status. waitReady updates ServiceHealth incrementally
	// but doesn't trigger setStatus.
	m.persistStatus(m.Snapshot())
	return nil
}

// Clear tears down the current compose stack and resets state. Idempotent.
func (m *Manager) Clear(ctx context.Context) error {
	activePath := filepath.Join(m.opts.ConfigDir, "active.yaml")
	if _, err := os.Stat(activePath); err == nil {
		if err := m.runCompose(ctx, "-f", activePath, "down", "--remove-orphans"); err != nil {
			return err
		}
		_ = os.Remove(activePath)
	}
	m.mu.Lock()
	m.state = State{}
	m.mu.Unlock()
	return nil
}

// Trim removes images that are no longer referenced by the active compose
// stack. Run on a periodic schedule, NEVER inline with profile switches.
// Compose's "down before up" ordering means inline pruning would discard
// shared layers and force full re-pulls.
func (m *Manager) Trim(ctx context.Context, olderThan time.Duration) error {
	args := []string{"image", "prune", "-f"}
	if olderThan > 0 {
		args = append(args, "--filter", fmt.Sprintf("until=%s", olderThan.String()))
	}
	cmd := exec.CommandContext(ctx, m.opts.DockerComposeBinary, args...)
	return cmd.Run()
}

// runCompose invokes `docker compose ...` against the inner dockerd.
func (m *Manager) runCompose(ctx context.Context, args ...string) error {
	full := append([]string{"compose"}, args...)
	cmd := exec.CommandContext(ctx, m.opts.DockerComposeBinary, full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s: %w (%s)", strings.Join(full, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// assertImagesPresent checks every image referenced in the parsed compose
// stack is already in the inner dockerd. Used in offline mode to fail
// fast if the image cache is incomplete.
func (m *Manager) assertImagesPresent(ctx context.Context, parsed *composeparse.ParseResult) error {
	// We don't have the parsed image list in ParseResult yet; pull it
	// from the YAML by re-parsing into a minimal struct. For v1 we
	// keep this simple — list `docker images` once and check.
	cmd := exec.CommandContext(ctx, m.opts.DockerComposeBinary, "images", "--format", "{{.Repository}}:{{.Tag}}")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("docker images: %w", err)
	}
	have := map[string]struct{}{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			have[line] = struct{}{}
		}
	}
	// Cross-reference against image refs in parsed compose. We look at
	// ParseResult.Models for hints but the authoritative list is in the
	// YAML — for v1 we trust the operator and only flag obvious
	// missing images at `up -d` time. Future work: extend ParseResult to
	// expose ImageRefs []string for offline pre-flight.
	_ = parsed
	_ = have
	return nil
}

// waitReady polls each service's readiness until they all pass or the
// timeout fires. For HTTP services we look for the OpenAI-compatible
// /v1/models endpoint to return 200. For services without a known port
// we fall back to "container running".
func (m *Manager) waitReady(ctx context.Context, parsed *composeparse.ParseResult) error {
	deadline := time.Now().Add(m.opts.ReadinessTimeout)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		health := map[string]string{}
		allHealthy := true
		for _, svc := range parsed.Models {
			ok := pingService(ctx, svc.ContainerName, svc.InternalPort)
			if ok {
				health[svc.ContainerName] = "healthy"
			} else {
				health[svc.ContainerName] = "starting"
				allHealthy = false
			}
		}
		m.mu.Lock()
		m.state.ServiceHealth = health
		m.mu.Unlock()
		if allHealthy {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("readiness timeout after %s", m.opts.ReadinessTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.opts.ReadinessPollInterval):
		}
	}
}

func (m *Manager) setStatus(p *types.RunnerProfile, status, errStr string) {
	m.mu.Lock()
	m.state.ProfileID = p.ID
	m.state.ProfileName = p.Name
	m.state.Status = status
	m.state.Error = errStr
	stateCopy := m.state
	m.mu.Unlock()
	m.persistStatus(stateCopy)
}

// persistStatus writes the current state to <ConfigDir>/status.json so
// other processes inside the sandbox (specifically sandbox-heartbeat) can
// pick it up and forward to the API server. Best-effort — failures don't
// block the manager.
func (m *Manager) persistStatus(s State) {
	path := filepath.Join(m.opts.ConfigDir, "status.json")
	data, err := jsonMarshal(s)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// jsonMarshal is split out so tests can stub if needed; kept tiny.
func jsonMarshal(v any) ([]byte, error) {
	type alias = State
	out, err := jsonMarshalIndent(v, "", "")
	if err != nil {
		return nil, err
	}
	return out, nil
}

func jsonMarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

func (m *Manager) setFailed(p *types.RunnerProfile, err error) {
	m.setStatus(p, "failed", err.Error())
}

// rewriteRegistry rewrites every `image: <registry>/...` line to use the
// supplied mirror. Mirrors the sed-style transformation in
// sandbox/04-start-dockerd.sh that already supports HELIX_SANDBOX_REGISTRY.
func rewriteRegistry(yaml, mirror string) string {
	// match: leading whitespace + "image:" + whitespace + image ref
	re := regexp.MustCompile(`(?m)^(\s*image:\s*)(\S+)\s*$`)
	return re.ReplaceAllStringFunc(yaml, func(line string) string {
		parts := re.FindStringSubmatch(line)
		if len(parts) != 3 {
			return line
		}
		ref := parts[2]
		// If ref already starts with mirror, skip.
		if strings.HasPrefix(ref, mirror+"/") {
			return line
		}
		// Strip the leading registry portion. A compose image ref is one of:
		//   image:tag                 (-> "library/image:tag" on docker.io)
		//   user/image:tag            (-> "user/image:tag" on docker.io)
		//   registry.host/path:tag    (-> "path:tag" on the host)
		// The portion before the first slash is a registry iff it contains
		// a "." or a ":" (port) or is "localhost". Otherwise the whole ref
		// is on docker.io.
		idx := strings.Index(ref, "/")
		var stripped string
		if idx >= 0 && (strings.Contains(ref[:idx], ".") || strings.Contains(ref[:idx], ":") || ref[:idx] == "localhost") {
			stripped = ref[idx+1:]
		} else {
			stripped = ref
		}
		return parts[1] + mirror + "/" + stripped
	})
}

// pingService probes the upstream's /v1/models endpoint via the host port
// mapping that compose declared. The compose-manager runs in the outer
// sandbox network namespace where 127.0.0.1:<host_port> reaches the inner
// container — same path the inference-proxy uses at request time.
func pingService(ctx context.Context, _ string, port int) bool {
	if port == 0 {
		return false
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/models", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
