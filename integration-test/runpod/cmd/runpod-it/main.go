// Command runpod-it is the RunPod-backed integration test harness for
// the Helix sandbox-absorbs-runner architecture. It reads
// integration-test/runpod/matrix.yaml, provisions one RunPod pod per
// matrix entry, applies the entry's runner profile, runs the seven
// scenarios, and tears the pod down. See Decision 14 in
// helix-specs/design/tasks/001959_we-need-to-replace-all/design.md.
//
// Cost-control rules enforced:
//   - 30 min soft / 35 min hard wall-clock per entry
//   - skipped if (sandbox image digest + profile YAML SHA + harness
//     code SHA) matches a prior green run in the result cache
//   - parallelism cap (default 4 — configurable via --parallel)
//   - daily $ budget (--max-daily-usd)
//
// Required env / flags:
//   RUNPOD_API_KEY   RunPod REST API key
//   --api-url        Helix API the sandbox should connect to
//   --runner-token   Shared runner token the sandbox uses
//
// Usage:
//   runpod-it                        # run the full matrix
//   runpod-it --only rtx4090,h100-sxm-1x
//   runpod-it --dry-run              # plan only, don't provision
//   runpod-it --no-cache             # ignore result cache
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/integration-test/runpod/internal/cache"
	"github.com/helixml/helix/integration-test/runpod/internal/provision"
	"github.com/helixml/helix/integration-test/runpod/internal/report"
	"github.com/helixml/helix/integration-test/runpod/internal/scenarios"
)

func main() {
	var (
		matrixFile  = flag.String("matrix", "integration-test/runpod/matrix.yaml", "path to the form-factor matrix")
		only        = flag.String("only", "", "comma-separated list of entry IDs to run (default: all enabled)")
		dryRun      = flag.Bool("dry-run", false, "plan only, don't provision pods")
		noCache     = flag.Bool("no-cache", false, "ignore result cache, always run")
		parallel    = flag.Int("parallel", 4, "max concurrent pods")
		maxDailyUSD = flag.Float64("max-daily-usd", 200.0, "refuse to schedule if today's RunPod spend already exceeds this")
		apiURL      = flag.String("api-url", os.Getenv("HELIX_API_URL"), "Helix API URL the sandboxes should connect to")
		runnerToken = flag.String("runner-token", os.Getenv("RUNNER_TOKEN"), "Shared runner token")
		junitOut    = flag.String("junit-out", "runpod-it.xml", "JUnit XML output path")
		mdOut       = flag.String("md-out", "runpod-it.md", "Markdown report output path")
		cacheDir    = flag.String("cache-dir", ".runpod-it-cache", "result cache directory")
	)
	flag.Parse()

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	apiKey := os.Getenv("RUNPOD_API_KEY")
	if apiKey == "" && !*dryRun {
		log.Fatal().Msg("RUNPOD_API_KEY required (or pass --dry-run)")
	}
	if *apiURL == "" && !*dryRun {
		log.Fatal().Msg("--api-url (or HELIX_API_URL) required")
	}
	if *runnerToken == "" && !*dryRun {
		log.Fatal().Msg("--runner-token (or RUNNER_TOKEN) required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go waitForSignal(cancel)

	mat, err := loadMatrix(*matrixFile)
	if err != nil {
		log.Fatal().Err(err).Msg("load matrix")
	}

	entries := mat.filter(*only)
	if len(entries) == 0 {
		log.Fatal().Msg("no enabled entries match")
	}
	log.Info().Int("entries", len(entries)).Bool("dry_run", *dryRun).Msg("starting runpod-it")

	resultCache := cache.New(*cacheDir)

	provisioner := provision.NewRunPodProvisioner(apiKey, *apiURL, *runnerToken)
	if *dryRun {
		// Print the plan and exit. We could exercise the dry-run
		// provisioner (which returns fake pods) but the scenarios then
		// hit a real API expecting real sandboxes — not informative.
		log.Info().Msg("dry-run plan:")
		for _, e := range entries {
			log.Info().
				Str("entry", e.ID).
				Str("gpu_type", e.RunPodGPUType).
				Int("gpu_count", e.GPUCount).
				Str("region", e.Region).
				Str("profile", e.Profile).
				Msg("would provision")
		}
		return
	}

	// Daily $ budget pre-flight.
	if !*dryRun {
		spent, err := provisioner.TodaySpentUSD(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("could not query RunPod billing — proceeding without budget check")
		} else if spent > *maxDailyUSD {
			log.Fatal().Float64("spent_usd", spent).Float64("budget_usd", *maxDailyUSD).Msg("daily RunPod budget exceeded — refusing to schedule")
		}
	}

	results := runMatrix(ctx, entries, provisioner, resultCache, *parallel, *noCache)

	if err := report.WriteJUnit(*junitOut, results); err != nil {
		log.Error().Err(err).Msg("write junit")
	}
	if err := report.WriteMarkdown(*mdOut, results); err != nil {
		log.Error().Err(err).Msg("write md report")
	}

	failed := 0
	for _, r := range results {
		if !r.Passed {
			failed++
		}
	}
	if failed > 0 {
		log.Error().Int("failed", failed).Int("total", len(results)).Msg("some matrix entries failed")
		os.Exit(1)
	}
	log.Info().Int("passed", len(results)).Msg("all matrix entries passed")
}

// runMatrix executes the matrix with bounded parallelism.
func runMatrix(ctx context.Context, entries []matrixEntry, prov provision.Provisioner, c *cache.Cache, parallel int, noCache bool) []report.Result {
	sem := make(chan struct{}, parallel)
	out := make([]report.Result, len(entries))
	var wg sync.WaitGroup
	for i, e := range entries {
		wg.Add(1)
		go func(idx int, entry matrixEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[idx] = runEntry(ctx, entry, prov, c, noCache)
		}(i, e)
	}
	wg.Wait()
	return out
}

// runEntry handles one matrix entry: cache check, provision, run scenarios,
// teardown, record. Wall-clock kill at 35 min.
func runEntry(parentCtx context.Context, e matrixEntry, prov provision.Provisioner, c *cache.Cache, noCache bool) report.Result {
	res := report.Result{Entry: e.ID}
	start := time.Now()

	// Cache check.
	if !noCache {
		key, err := c.Key(e.ID, e.Profile)
		if err == nil {
			if cached, ok := c.Lookup(key); ok && cached.Passed {
				res.Passed = true
				res.CachedFrom = cached.RecordedAt
				res.DurationSeconds = 0
				log.Info().Str("entry", e.ID).Time("cached_at", cached.RecordedAt).Msg("cache hit (green)")
				return res
			}
		}
	}

	// Hard 35-min cap.
	ctx, cancel := context.WithTimeout(parentCtx, 35*time.Minute)
	defer cancel()

	// Provision.
	pod, err := prov.Provision(ctx, provision.PodSpec{
		EntryID:    e.ID,
		GPUType:    e.RunPodGPUType,
		GPUCount:   e.GPUCount,
		Region:     e.Region,
		ImageRef:   "helixml/helix-sandbox:latest", // TODO: parameterise
	})
	if err != nil {
		res.Failure = "provision: " + err.Error()
		res.DurationSeconds = int(time.Since(start).Seconds())
		return res
	}
	defer prov.Teardown(context.Background(), pod.ID) // best-effort even on ctx cancel

	// Run scenarios.
	runner := scenarios.NewRunner(pod, e.Profile, e.SecondaryProfilesForSwitch, e.IncompatibleProfile)
	scenarioResults, err := runner.RunAll(ctx)
	res.Scenarios = scenarioResults
	res.DurationSeconds = int(time.Since(start).Seconds())
	if err != nil {
		res.Failure = "scenarios: " + err.Error()
		return res
	}

	allPassed := true
	for _, s := range scenarioResults {
		if !s.Passed {
			allPassed = false
			break
		}
	}
	res.Passed = allPassed
	if allPassed && !noCache {
		key, err := c.Key(e.ID, e.Profile)
		if err == nil {
			c.Record(key, cache.Entry{Passed: true, RecordedAt: time.Now()})
		}
	}
	return res
}

func waitForSignal(cancel context.CancelFunc) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Info().Msg("shutdown signal received — letting in-flight tests teardown")
	cancel()
}

// matrixEntry is the in-memory representation of one matrix.yaml entry.
type matrixEntry struct {
	ID                          string   `yaml:"id"`
	Enabled                     bool     `yaml:"enabled"`
	RunPodGPUType               string   `yaml:"runpod_gpu_type"`
	GPUCount                    int      `yaml:"gpu_count"`
	Region                      string   `yaml:"region"`
	Profile                     string   `yaml:"profile"`
	ProfileMetadata             any      `yaml:"profile_metadata"`
	SecondaryProfilesForSwitch  []string `yaml:"secondary_profiles_for_switch"`
	IncompatibleProfile         string   `yaml:"incompatible_profile"`
}

type matrix struct {
	DefaultScenarios []string      `yaml:"default_scenarios"`
	Entries          []matrixEntry `yaml:"entries"`
}

func (m *matrix) filter(only string) []matrixEntry {
	var out []matrixEntry
	wanted := map[string]bool{}
	for _, id := range strings.Split(only, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			wanted[id] = true
		}
	}
	for _, e := range m.Entries {
		if !e.Enabled {
			continue
		}
		if len(wanted) > 0 && !wanted[e.ID] {
			continue
		}
		out = append(out, e)
	}
	return out
}

func loadMatrix(path string) (*matrix, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read matrix: %w", err)
	}
	var m matrix
	// gopkg.in/yaml.v3 is already vendored in the helix module.
	if err := yamlUnmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse matrix: %w", err)
	}
	return &m, nil
}
