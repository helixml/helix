package main

import "fmt"

// Paths for the DeepSeek Harness (dsh) runtime. Vars, not consts, so tests can
// redirect them at a tempdir.
var (
	// DeepSeekHarnessCommand is the wrapper Dockerfile.ubuntu-helix installs.
	// It selects the private Node 24 under /opt/helix/dsh and loads the ACP
	// composition. Upstream's product CLI has no acp subcommand, so there is
	// no "dsh acp" to call instead.
	DeepSeekHarnessCommand = "/usr/local/bin/dsh-acp"
	// DeepSeekHarnessHome is $DSH_HOME: skill discovery and the credential
	// store. Isolated under a helix- prefix so it cannot collide with a
	// harness a user installs themselves.
	DeepSeekHarnessHome = "/home/retro/.config/helix-dsh"
	// DeepSeekHarnessSessionsDir holds the JSONL session log and its derived
	// SQLite query index. Under the workspace so sessions survive a container
	// restart, matching QWEN_DATA_DIR and OpenCodeDataHome.
	DeepSeekHarnessSessionsDir = "/home/retro/work/.dsh-sessions"
)

// buildDeepSeekHarnessEnv renders the environment the ACP composition reads.
//
// Unlike opencode (one JSON blob) or goose (a config file on disk), dsh takes
// its whole configuration from desktop/shared/dsh/cordis.yml, which resolves
// these four variables at boot. Keeping the composition in the image and only
// the values here means a config change is a reviewable repo diff rather than
// a Go string builder.
//
// baseURL must already have been through rewriteLocalhostURL: the agent runs
// in a different network namespace from the API, so a localhost URL from the
// session config would resolve to the container itself.
func buildDeepSeekHarnessEnv(baseURL, model, apiKey string) (map[string]interface{}, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("no base URL configured")
	}
	if model == "" {
		return nil, fmt.Errorf("no model configured")
	}
	// pi-ai resolves apiKeyEnv per request and fails the request with
	// MISSING_CREDENTIAL when the reference resolves to nothing. Catching it
	// here turns a mid-turn provider error into a startup log line naming the
	// cause.
	if apiKey == "" {
		return nil, fmt.Errorf("no API key available yet")
	}
	return map[string]interface{}{
		"HELIX_BASE_URL":    baseURL,
		"HELIX_MODEL":       model,
		"HELIX_API_KEY":     apiKey,
		"DSH_HOME":          DeepSeekHarnessHome,
		"DSH_SESSIONS_ROOT": DeepSeekHarnessSessionsDir,
	}, nil
}
