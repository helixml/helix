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
	"sync"
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
// So the context servers have to be proven reachable before EVERY Zed launch,
// not once at boot: run_zed_restart_loop respawns Zed for the life of the
// container, and restartZed() deliberately uses that path to give the agent a
// fresh ACP session on an agent switch. Each of those launches is another
// one-shot MCP registration, so each one needs its own verdict.
// start-zed-core.sh asks /mcp-readiness for a fresh one before every launch.
//
// See design/2026-09-01-opencode-mcp-tools-unavailable.md.

const (
	// mcpProbeTimeout bounds one endpoint probe. Endpoints are probed
	// concurrently, so this is also roughly the bound on a whole pass, which is
	// what keeps /mcp-readiness prompt enough for the shell gate to poll it.
	mcpProbeTimeout = 5 * time.Second
)

// contextServerEndpoint is one URL-addressed MCP server exactly as Zed will
// hand it to the agent.
type contextServerEndpoint struct {
	name    string
	url     string
	headers map[string]string
}

// probeMCPEndpoints makes one pass over the configured context servers and
// returns nil when the agent can safely be started. It is the whole readiness
// verdict: callers decide whether to retry.
func (d *SettingsDaemon) probeMCPEndpoints(ctx context.Context, settingsPath string) error {
	endpoints, err := contextServerEndpoints(settingsPath)
	if err != nil {
		return err
	}
	if len(endpoints) == 0 {
		// Settings parsed, but this agent has no URL-addressed context servers
		// (a stdio-only agent, or one with no MCP at all). Nothing to prove.
		return nil
	}

	failures := make([]string, len(endpoints))
	var wg sync.WaitGroup
	for i, ep := range endpoints {
		wg.Add(1)
		go func(i int, ep contextServerEndpoint) {
			defer wg.Done()
			if err := d.probeContextServer(ctx, ep); err != nil {
				failures[i] = err.Error()
			}
		}(i, ep)
	}
	wg.Wait()

	var reasons []string
	for _, f := range failures {
		if f != "" {
			reasons = append(reasons, f)
		}
	}
	if len(reasons) > 0 {
		return fmt.Errorf("%s", strings.Join(reasons, "; "))
	}
	return nil
}

// probeContextServer reports whether the agent will be able to reach this
// context server.
//
// It requests the configured URL verbatim — path, query and headers — because
// that is the only thing whose reachability we are entitled to assert. An
// earlier version probed a synthetic `origin + "/api/v1/config"`, which is a
// different route: a reverse proxy can serve that while rejecting or
// misrouting /api/v1/mcp/..., and a user-configured third-party MCP server
// need not serve Helix's config endpoint at all.
//
// Method: OPTIONS. The MCP gateway routes register GET, POST and OPTIONS
// (api/pkg/server/server.go), and the choice among them is forced:
//
//   - POST is the real MCP initialize and allocates server-side session state.
//   - GET is the Streamable HTTP notification stream. Measured against a live
//     session it hangs until timeout on helix-desktop and returns an open SSE
//     stream on helix-session and kodit.
//   - HEAD is not a registered method, so gorilla/mux never matches the route
//     and the request never reaches the auth middleware. Measured, HEAD
//     returns 404 for a valid token, an invalid token and no token alike — it
//     cannot see authentication at all.
//
// OPTIONS is the one method that is prompt (0.00s on all four servers),
// allocates nothing (24 probes produced no additional kodit handler
// creations), and actually traverses authentication.
//
// Failure is a transport error, a 5xx, or a 401/403. Every other status is
// success, and that is measured rather than lazy: with a valid token, OPTIONS
// on the correct MCP URLs returns 404 (helix-desktop, helix-session, kodit) or
// 400 (helix-tasks), while OPTIONS on a deliberately wrong path returns 204
// from the frontend catch-all. Status is anti-correlated with routing
// correctness, so anything stricter would reject exactly the endpoints that
// work. This probe therefore cannot detect path-level misrouting; it detects
// transport, upstream health and authentication.
//
// The three failure classes each correspond to a way the agent's one-shot
// registration dies:
//
//   - transport error: the address does not resolve or refuses connections.
//   - 5xx: with the Helix API down but the sandbox proxy up, the proxy's
//     ErrorHandler answers every MCP URL with 502 "Helix API unavailable".
//   - 401/403: the session token in the header is expired or wrong. Measured,
//     an invalid token turns every one of these endpoints from 404/400 into
//     401, and the agent sending that same header would fail identically.
//
// A third-party MCP server that rejects even an authenticated OPTIONS would be
// a false negative here. That is the right way to be wrong: a false negative
// is a loud error naming the URL and status, while a false positive is the
// silent session-long tool outage this gate exists to prevent.
func (d *SettingsDaemon) probeContextServer(ctx context.Context, ep contextServerEndpoint) error {
	probeCtx, cancel := context.WithTimeout(ctx, mcpProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodOptions, ep.url, nil)
	if err != nil {
		return fmt.Errorf("probe %s (%s): %w", ep.name, ep.url, err)
	}
	for k, v := range ep.headers {
		req.Header.Set(k, v)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("probe %s (%s): %w", ep.name, ep.url, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 500:
		return fmt.Errorf("probe %s (%s): %s (the Helix API behind the sandbox proxy is unavailable)",
			ep.name, ep.url, resp.Status)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("probe %s (%s): %s (the configured Authorization header is not accepted; "+
			"the agent sends the same header and its MCP registration would fail)",
			ep.name, ep.url, resp.Status)
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
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := d.probeMCPEndpoints(r.Context(), SettingsPath); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ready")
}

// contextServerEndpoints reads the settings file on disk and returns every
// URL-addressed context server, sorted by name so logs and probe order are
// stable.
//
// Reading the file rather than d.helixSettings is deliberate on two counts.
// It is the exact set Zed loads and forwards to the agent, including any user
// overrides merged in; and the file is written atomically (tempfile + rename),
// so the HTTP handler can read it concurrently with the poll loop rewriting it
// without sharing mutable state with the rest of the daemon.
//
// A context server with no "url" is a stdio server: it runs inside the
// container and cannot fail this way, so it is ignored. A context server that
// declares a "url" we cannot use is a configuration error and fails readiness —
// Zed will hand that broken URL to the agent regardless, and the agent's
// one-shot registration against it will fail permanently. Silently skipping it
// is how an all-malformed config would otherwise report "ready".
func contextServerEndpoints(settingsPath string) ([]contextServerEndpoint, error) {
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

	var endpoints []contextServerEndpoint
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
		endpoints = append(endpoints, contextServerEndpoint{
			name:    name,
			url:     rawURL,
			headers: stringHeaders(server["headers"]),
		})
	}
	if len(invalid) > 0 {
		sort.Strings(invalid)
		return nil, fmt.Errorf("context servers with unusable urls: %s", strings.Join(invalid, ", "))
	}

	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].name < endpoints[j].name })
	return endpoints, nil
}

// stringHeaders pulls the string-valued headers out of a context server entry.
// They are sent with the probe so it traverses the same auth edge the agent
// will.
func stringHeaders(raw interface{}) map[string]string {
	entries, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	headers := make(map[string]string, len(entries))
	for k, v := range entries {
		if s, ok := v.(string); ok {
			headers[k] = s
		}
	}
	return headers
}
