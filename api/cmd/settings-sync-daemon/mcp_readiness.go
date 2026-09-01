package main

import (
	"context"
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
// So the address we just wrote into settings.json has to be proven reachable
// BEFORE Zed launches the agent. If it is not, the session is not degraded —
// it is broken, because the same origin serves inference — and it must say so.
//
// See design/2026-09-01-opencode-mcp-tools-unavailable.md.

// Marker paths. Vars, not consts, so tests can redirect them at a tempdir
// instead of writing the real container paths. The same literals appear in
// desktop/shared/start-zed-core.sh, which is the reader.
var (
	// mcpEndpointsReadyMarker is written once every remote context_server
	// origin has answered. start-zed-core.sh waits for it before launching
	// Zed, so the agent's one-shot MCP registration happens in a window where
	// the endpoints are known good.
	mcpEndpointsReadyMarker = "/tmp/helix-mcp-endpoints-ready"

	// mcpEndpointsUnreachableMarker records why the check failed.
	// start-zed-core.sh prints its contents in the fatal message so an
	// operator sees the actual URL rather than "Zed never connected".
	mcpEndpointsUnreachableMarker = "/tmp/helix-mcp-endpoints-unreachable"
)

const (
	// mcpProbePath is an unauthenticated endpoint on the Helix API. Probing it
	// rather than the MCP paths themselves keeps the check free of side
	// effects: a real MCP initialize would create server-side session state
	// (kodit allocates a per-session handler), and a GET on an MCP endpoint
	// either 405s or opens an SSE stream we would have to tear down.
	mcpProbePath = "/api/v1/config"

	mcpProbeTimeout = 5 * time.Second
)

// verifyMCPEndpoints proves every distinct origin behind a remote
// context_server answers, retrying until timeout elapses. It writes the ready
// marker on success and the unreachable marker on failure.
//
// It deliberately reads the URLs back out of the settings we just merged rather
// than trusting HELIX_API_URL. Those two can disagree: a control plane that has
// rolled forward to the sandbox API proxy address emits helix-api.internal:18080
// into context_servers while an older sandbox host still hands the container
// HELIX_API_URL=http://api:8080 and never pins the proxy hostname. Zed's
// websocket then works over the env address, inference works, and only the MCP
// servers are dead — which is exactly the failure this check exists to catch,
// and exactly the one probing HELIX_API_URL would miss.
func (d *SettingsDaemon) verifyMCPEndpoints(ctx context.Context, timeout, retryDelay time.Duration) error {
	if d.helixSettings == nil {
		// The initial sync never completed, so there is no agent config at all
		// — nothing was written for us to verify. Declaring readiness here
		// would be a lie that lets Zed start against a config that does not
		// exist, which is the failure this gate exists to prevent.
		return d.failMCPEndpoints(fmt.Errorf("settings were never synced from Helix, so no agent config was written"))
	}

	origins := remoteContextServerOrigins(d.helixSettings)
	if len(origins) == 0 {
		// Settings synced, but this agent has no URL-addressed context servers
		// (a stdio-only agent, or one with no MCP at all). Nothing to prove.
		log.Printf("MCP readiness: no remote context servers configured; nothing to verify")
		return d.markMCPEndpointsReady()
	}

	log.Printf("MCP readiness: verifying %d context server origin(s): %s",
		len(origins), strings.Join(origins, ", "))

	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		lastErr = nil
		for _, origin := range origins {
			if err := d.probeOrigin(ctx, origin); err != nil {
				lastErr = err
				break
			}
		}
		if lastErr == nil {
			log.Printf("MCP readiness: all %d context server origin(s) reachable", len(origins))
			return d.markMCPEndpointsReady()
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

	return d.failMCPEndpoints(fmt.Errorf("unreachable after %s: %w", timeout, lastErr))
}

// failMCPEndpoints records why the agent must not be started and returns the
// error main() reports to the API.
func (d *SettingsDaemon) failMCPEndpoints(cause error) error {
	err := fmt.Errorf("Helix MCP context servers are not usable from this container (%w). "+
		"The coding agent connects to them exactly once when it starts and never retries, "+
		"so it would run with no Helix tools for the whole session", cause)
	if writeErr := os.WriteFile(mcpEndpointsUnreachableMarker, []byte(err.Error()+"\n"), 0644); writeErr != nil {
		log.Printf("MCP readiness: failed to write %s: %v", mcpEndpointsUnreachableMarker, writeErr)
	}
	return err
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

// markMCPEndpointsReady clears any stale unreachable marker and writes the
// ready marker start-zed-core.sh gates on.
func (d *SettingsDaemon) markMCPEndpointsReady() error {
	if err := os.Remove(mcpEndpointsUnreachableMarker); err != nil && !os.IsNotExist(err) {
		log.Printf("MCP readiness: failed to clear %s: %v", mcpEndpointsUnreachableMarker, err)
	}
	if err := os.WriteFile(mcpEndpointsReadyMarker, []byte("ready\n"), 0644); err != nil {
		return fmt.Errorf("write %s: %w", mcpEndpointsReadyMarker, err)
	}
	return nil
}

// remoteContextServerOrigins returns the distinct scheme://host[:port] of every
// context_server addressed by URL. Stdio servers (chrome-devtools and any
// user-configured command) have no URL and cannot fail this way — they run
// inside the container.
//
// Sorted so the log line and the probe order are stable.
func remoteContextServerOrigins(settings map[string]interface{}) []string {
	contextServers, ok := settings["context_servers"].(map[string]interface{})
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	for name, raw := range contextServers {
		server, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		rawURL, ok := server["url"].(string)
		if !ok || rawURL == "" {
			continue
		}
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Host == "" {
			log.Printf("MCP readiness: skipping context server %q with unparseable url %q", name, rawURL)
			continue
		}
		seen[parsed.Scheme+"://"+parsed.Host] = struct{}{}
	}
	origins := make([]string, 0, len(seen))
	for origin := range seen {
		origins = append(origins, origin)
	}
	sort.Strings(origins)
	return origins
}
