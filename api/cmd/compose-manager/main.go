// Command compose-manager is a long-running process that lives inside the
// Helix sandbox and applies operator-assigned runner profiles by running
// `docker compose` against the inner dockerd.
//
// It connects to the API server (over RevDial / NATS as the rest of the
// sandbox already does), polls for its assigned profile, and applies it.
// It also reports its state (active profile, service health) back via
// the existing sandbox heartbeat.
//
// In offline mode (HELIX_RUNNER_OFFLINE=true) it skips `docker compose
// pull` and refuses to apply a profile if any referenced image is absent
// from the local cache.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/composemgr"
	"github.com/helixml/helix/api/pkg/types"
)

func main() {
	var (
		apiURL          = flag.String("api-url", envOr("HELIX_API_URL", "http://localhost:8080"), "Helix API server base URL")
		runnerID        = flag.String("runner-id", envOr("HELIX_RUNNER_ID", ""), "this sandbox's runner ID (must match its NATS-reported ID)")
		runnerToken     = flag.String("runner-token", envOr("HELIX_RUNNER_TOKEN", ""), "shared secret for runner-side endpoints")
		configDir       = flag.String("config-dir", envOr("HELIX_RUNNER_CONFIG_DIR", "/etc/helix"), "directory for active.yaml")
		registryMirror  = flag.String("registry-mirror", os.Getenv("HELIX_RUNNER_REGISTRY"), "rewrite leading registry portion of image: refs to this mirror")
		offline         = flag.Bool("offline", os.Getenv("HELIX_RUNNER_OFFLINE") == "true", "skip docker compose pull; fail fast if images absent")
		pollInterval    = flag.Duration("poll-interval", 15*time.Second, "how often to check for assignment changes")
		trimEvery       = flag.Duration("trim-every", 24*time.Hour, "how often to prune unreferenced images")
		trimOlderThan   = flag.Duration("trim-older-than", 72*time.Hour, "min age before pruning an unreferenced image")
	)
	flag.Parse()

	if *runnerID == "" {
		log.Fatal().Msg("--runner-id (or HELIX_RUNNER_ID) is required")
	}
	if *runnerToken == "" {
		log.Fatal().Msg("--runner-token (or HELIX_RUNNER_TOKEN) is required")
	}

	mgr := composemgr.New(composemgr.Options{
		ConfigDir:           *configDir,
		DockerComposeBinary: "docker",
		RegistryMirror:      *registryMirror,
		Offline:             *offline,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go waitForSignal(cancel)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	currentProfileID := ""

	// Periodic prune ticker — never run inline with a profile switch.
	go func() {
		t := time.NewTicker(*trimEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := mgr.Trim(ctx, *trimOlderThan); err != nil {
					log.Warn().Err(err).Msg("compose-manager: trim")
				}
			}
		}
	}()

	// Reconciliation loop: fetch assignment, apply if changed, sleep.
	t := time.NewTicker(*pollInterval)
	defer t.Stop()
	tick := func() {
		assignment, err := fetchAssignment(ctx, httpClient, *apiURL, *runnerToken, *runnerID)
		if err != nil {
			log.Debug().Err(err).Msg("compose-manager: fetch assignment")
			return
		}
		if assignment == nil {
			if currentProfileID != "" {
				if err := mgr.Clear(ctx); err != nil {
					log.Warn().Err(err).Msg("compose-manager: clear")
				}
				currentProfileID = ""
			}
			return
		}
		if assignment.ProfileID == currentProfileID {
			return
		}
		profile, err := fetchProfile(ctx, httpClient, *apiURL, *runnerToken, assignment.ProfileID)
		if err != nil {
			log.Warn().Err(err).Str("profile_id", assignment.ProfileID).Msg("compose-manager: fetch profile")
			return
		}
		if err := mgr.Apply(ctx, profile); err != nil {
			log.Error().Err(err).Str("profile_id", profile.ID).Msg("compose-manager: apply")
			// Don't update currentProfileID — we'll retry on next tick.
			return
		}
		currentProfileID = profile.ID
		log.Info().Str("profile_id", profile.ID).Str("profile_name", profile.Name).Msg("compose-manager: profile applied")
	}
	tick() // immediately on start
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("compose-manager: shutting down")
			return
		case <-t.C:
			tick()
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func waitForSignal(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	cancel()
}

// fetchAssignment hits GET /api/v1/runner/{id}/assignment (the runner-
// token-authenticated path). Returns nil without error if the runner has
// no assignment (404 is a normal state).
func fetchAssignment(ctx context.Context, c *http.Client, base, token, runnerID string) (*types.RunnerAssignment, error) {
	url := base + "/api/v1/runner/" + runnerID + "/assignment"
	return getJSON[types.RunnerAssignment](ctx, c, url, token, true)
}

// fetchProfile hits GET /api/v1/runner/profiles/{id} (runner-token path).
func fetchProfile(ctx context.Context, c *http.Client, base, token, profileID string) (*types.RunnerProfile, error) {
	url := base + "/api/v1/runner/profiles/" + profileID
	return getJSON[types.RunnerProfile](ctx, c, url, token, false)
}
