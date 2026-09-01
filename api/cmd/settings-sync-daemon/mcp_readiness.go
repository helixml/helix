package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// Why this exists.
//
// The remote context_servers we write into settings.json (helix-tasks,
// helix-session, helix-desktop, kodit) do not reach the coding agent through
// Zed at runtime. Zed forwards them once, on the ACP session/new|load|resume
// request, and the agent connects to each exactly once. opencode records a
// connect failure as a permanently "failed" server, logs a single warning, and
// never retries; Zed never re-pushes them either.
//
// The consequence is the worst kind of failure: invisible and total. Every
// <server>_<tool> call the model makes for the rest of the session dies with
// "Model tried to call unavailable tool", while the LLM path (which retries per
// turn) keeps working, so the session looks healthy. Meanwhile our own prompts
// keep instructing the agent to call list_secrets / get_secret / ask_human. A
// session in this state burns its whole budget calling tools that were never
// registered.
//
// So the addresses in settings.json have to be proven reachable before EVERY
// Zed launch, not once at boot: run_zed_restart_loop respawns Zed for the life
// of the container, and restartZed() deliberately uses that path to give the
// agent a fresh ACP session on an agent switch. Each of those launches is
// another one-shot MCP registration, so each one needs its own verdict.
// start-zed-core.sh asks /mcp-readiness for a fresh one before every launch.
//
// See design/2026-09-01-opencode-mcp-tools-unavailable.md.

const (
	// mcpProbePath is an unauthenticated endpoint on the Helix API. Probing it
	// rather than the MCP paths themselves keeps the check free of side
	// effects: a real MCP initialize would create server-side session state
	// (kodit allocates a per-session handler), and a GET on an MCP endpoint
	// either 405s or opens an SSE stream we would have to tear down.
	mcpProbePath = "/api/v1/config"

	mcpProbeTimeout = 5 * time.Second
)

// probeMCPEndpoints makes one pass over the context server origins and returns
// nil when the agent can safely be started. It is the whole readiness verdict:
// callers decide whether to retry.
func (d *SettingsDaemon) probeMCPEndpoints(ctx context.Context, settingsPath string) error {
	origins, err := contextServerOrigins(settingsPath)
	if err != nil {
		return err
	}
	if len(origins) == 0 {
		// Settings parsed, but this agent has no URL-addressed context servers
		// (a stdio-only agent, or one with no MCP at all). Nothing to prove.
		return nil
	}
	for _, origin := range origins {
		if err := d.probeOrigin(ctx, origin); err != nil {
			return err
		}
	}
	return nil
}

// waitForMCPEndpoints retries probeMCPEndpoints until timeout elapses. Used on
// the boot path, where a failure is reported to the API so the operator sees it
// in the session rather than having to read container logs.
func (d *SettingsDaemon) waitForMCPEndpoints(ctx context.Context, settingsPath string, timeout, retryDelay time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		lastErr = d.probeMCPEndpoints(ctx, settingsPath)
		if lastErr == nil {
			log.Printf("MCP readiness: context servers reachable")
			return nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			break
		}
		log.Printf("MCP readiness: %v (retrying in %s)", lastErr, retryDelay)
		select {
		case <-ctx.Done():
		case <-time.After(retryDelay):
		}
	}
	return fmt.Errorf("Helix MCP context servers are not usable from this container "+
		"(unreachable after %s: %w). The coding agent connects to them exactly once when it "+
		"starts and never retries, so it would run with no Helix tools for the whole session",
		timeout, lastErr)
}

// mcpReadinessHandler answers the per-launch gate in start-zed-core.sh with a
// freshly measured verdict. 200 means "safe to launch Zed"; 503 carries the
// reason, which the gate prints.
//
// It is deliberately a single pass with no internal retry: the waiting policy
// lives in the shell loop that polls this, so there is exactly one place that
// decides how long a launch may be held back.
func (d *SettingsDaemon) mcpReadinessHandler(w http.ResponseWriter, r *http.Request) {
	if err := d.probeMCPEndpoints(r.Context(), SettingsPath); err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ready")
}

// probeOrigin reports whether origin is reachable. Any HTTP response counts: a
// 401/404/405 proves the transport works, which is the only thing that can
// break the agent's MCP registration. Only a transport error (DNS failure,
// connection refused, timeout) is a failure.
func (d *SettingsDaemon) probeOrigin(ctx context.Context, origin string) error {
	probeCtx, cancel := context.WithTimeout(ctx, mcpProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, origin+mcpProbePath, nil)
	if err != nil {
		return fmt.Errorf("probe %s: %w", origin, err)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("probe %s: %w", origin, err)
	}
	defer resp.Body.Close()
	return nil
}

// contextServerOrigins returns the distinct scheme://host[:port] of every
// context_server addressed by URL, read from the settings file on disk.
//
// Reading the file rather than d.helixSettings is deliberate on two counts.
// It is the exact set Zed loads and forwards to the agent, including any user
// overrides merged in; and the file is written atomically (tempfile + rename),
// so the HTTP handler can read it concurrently with the poll loop rewriting it
// without sharing mutable state with the rest of the daemon.
//
// A context server with no "url" is a stdio server: it runs inside the
// container and cannot fail this way, so it is ignored. A context server that
// declares a "url" we cannot turn into an origin is a configuration error and
// fails readiness — Zed will hand that broken URL to the agent regardless, and
// the agent's one-shot registration against it will fail permanently. Silently
// skipping it is how an all-malformed config would otherwise report "ready".
//
// Origins are sorted so the log line and the probe order are stable.
func contextServerOrigins(settingsPath string) ([]string, error) {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		// No settings file means the initial sync never completed and no agent
		// config was written. Reporting readiness here would let Zed start
		// against a config that does not exist.
		return nil, fmt.Errorf("read %s: %w", settingsPath, err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", settingsPath, err)
	}

	contextServers, ok := settings["context_servers"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	seen := map[string]struct{}{}
	var invalid []string
	for name, entry := range contextServers {
		server, ok := entry.(map[string]interface{})
		if !ok {
			invalid = append(invalid, fmt.Sprintf("%s (not an object)", name))
			continue
		}
		value, present := server["url"]
		if !present {
			// stdio server (command/args) — nothing to reach over the network.
			continue
		}
		rawURL, ok := value.(string)
		if !ok || strings.TrimSpace(rawURL) == "" {
			invalid = append(invalid, fmt.Sprintf("%s (empty url)", name))
			continue
		}
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			invalid = append(invalid, fmt.Sprintf("%s (unusable url %q)", name, rawURL))
			continue
		}
		seen[parsed.Scheme+"://"+parsed.Host] = struct{}{}
	}
	if len(invalid) > 0 {
		sort.Strings(invalid)
		return nil, fmt.Errorf("context servers with unusable urls: %s", strings.Join(invalid, ", "))
	}

	origins := make([]string, 0, len(seen))
	for origin := range seen {
		origins = append(origins, origin)
	}
	sort.Strings(origins)
	return origins, nil
}
