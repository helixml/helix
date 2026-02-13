package hydra

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// DefaultSocketDir is the runtime directory for active docker sockets
	DefaultSocketDir = "/var/run/hydra/active"

	// DefaultDataDir is the persistent directory for docker data
	DefaultDataDir = "/hydra-data"

	// SharedBuildKitCacheDir is the directory for shared BuildKit cache across all sessions
	// BuildKit uses content-addressed storage, so concurrent access is safe
	SharedBuildKitCacheDir = "buildkit-cache"

	// SharedBuildKitContainerName is the name of the shared BuildKit container
	SharedBuildKitContainerName = "helix-buildkit"

	// SharedBuildKitImage is the BuildKit image to use
	SharedBuildKitImage = "moby/buildkit:latest"

	// SharedBuildxBuilderName is the name of the shared buildx builder
	SharedBuildxBuilderName = "helix-shared"
)

// Manager manages the Hydra runtime (dev containers, shared BuildKit).
// With docker-in-desktop mode, each desktop container runs its own dockerd.
// The manager no longer needs to manage per-session dockerd subprocess instances,
// bridge interfaces, veth pairs, or DNS proxies.
type Manager struct {
	socketDir string
	dataDir   string
	mutex     sync.RWMutex
}

// NewManager creates a new Hydra manager
func NewManager(socketDir, dataDir string) *Manager {
	if socketDir == "" {
		socketDir = DefaultSocketDir
	}
	if dataDir == "" {
		dataDir = DefaultDataDir
	}

	return &Manager{
		socketDir: socketDir,
		dataDir:   dataDir,
	}
}

// Start initializes the manager and starts background tasks
func (m *Manager) Start(ctx context.Context) error {
	// Create runtime directories
	if err := os.MkdirAll(m.socketDir, 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}
	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create shared BuildKit cache directory for all sessions
	buildkitCacheDir := filepath.Join(m.dataDir, SharedBuildKitCacheDir)
	if err := os.MkdirAll(buildkitCacheDir, 0777); err != nil {
		return fmt.Errorf("failed to create buildkit cache directory: %w", err)
	}
	if err := os.Chmod(buildkitCacheDir, 0777); err != nil {
		log.Warn().Err(err).Str("dir", buildkitCacheDir).Msg("Failed to set buildkit cache directory permissions")
	}

	// Setup shared BuildKit container and builder for cache sharing
	if err := m.setupSharedBuildKit(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to setup shared BuildKit container, builds will work but cache won't be shared")
	}

	log.Info().
		Str("socket_dir", m.socketDir).
		Str("data_dir", m.dataDir).
		Msg("Hydra manager started (docker-in-desktop mode)")

	return nil
}

// Stop gracefully shuts down the manager
func (m *Manager) Stop(ctx context.Context) error {
	log.Info().Msg("Hydra manager stopped")
	return nil
}

// setupSharedBuildKit creates a shared BuildKit container and buildx builder
// that all dev containers can use for cached builds.
func (m *Manager) setupSharedBuildKit(ctx context.Context) error {
	buildkitCacheDir := filepath.Join(m.dataDir, SharedBuildKitCacheDir)

	// Check if buildkit container already exists and is running
	checkCmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", SharedBuildKitContainerName)
	output, err := checkCmd.Output()
	if err == nil && strings.TrimSpace(string(output)) == "true" {
		log.Debug().Str("container", SharedBuildKitContainerName).Msg("Shared BuildKit container already running")
		return m.ensureBuildxBuilder(ctx)
	}

	// Remove old container if exists (might be stopped)
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", SharedBuildKitContainerName).Run()

	// Create buildkit container with cache directory mounted
	log.Info().
		Str("container", SharedBuildKitContainerName).
		Str("cache_dir", buildkitCacheDir).
		Msg("Creating shared BuildKit container")

	createCmd := exec.CommandContext(ctx, "docker", "run", "-d",
		"--name", SharedBuildKitContainerName,
		"--privileged",
		"-v", buildkitCacheDir+":/buildkit-cache",
		"-v", "buildkit_state:/var/lib/buildkit",
		"--restart", "unless-stopped",
		SharedBuildKitImage,
		"--addr", "unix:///run/buildkit/buildkitd.sock",
		"--addr", "tcp://0.0.0.0:1234",
	)

	if output, err := createCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create buildkit container: %w, output: %s", err, string(output))
	}

	// Wait for container to be running
	time.Sleep(2 * time.Second)

	return m.ensureBuildxBuilder(ctx)
}

// ensureBuildxBuilder creates or updates the buildx builder pointing to the shared BuildKit container
func (m *Manager) ensureBuildxBuilder(ctx context.Context) error {
	// Get buildkit container IP
	ipCmd := exec.CommandContext(ctx, "docker", "inspect", "-f",
		"{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
		SharedBuildKitContainerName)
	ipOutput, err := ipCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get buildkit container IP: %w", err)
	}
	buildkitIP := strings.TrimSpace(string(ipOutput))
	if buildkitIP == "" {
		return fmt.Errorf("buildkit container has no IP address")
	}

	// Check if builder already exists
	checkCmd := exec.CommandContext(ctx, "docker", "buildx", "inspect", SharedBuildxBuilderName)
	if err := checkCmd.Run(); err == nil {
		log.Debug().Str("builder", SharedBuildxBuilderName).Msg("Buildx builder already exists")
		_ = exec.CommandContext(ctx, "docker", "buildx", "use", SharedBuildxBuilderName).Run()
		return nil
	}

	// Remove stale builder if exists
	_ = exec.CommandContext(ctx, "docker", "buildx", "rm", SharedBuildxBuilderName).Run()

	// Create buildx builder pointing to the container
	log.Info().
		Str("builder", SharedBuildxBuilderName).
		Str("endpoint", "tcp://"+buildkitIP+":1234").
		Msg("Creating shared buildx builder")

	createCmd := exec.CommandContext(ctx, "docker", "buildx", "create",
		"--name", SharedBuildxBuilderName,
		"--driver", "remote",
		"tcp://"+buildkitIP+":1234",
		"--use",
	)
	if output, err := createCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create buildx builder: %w, output: %s", err, string(output))
	}

	// Bootstrap the builder
	bootstrapCmd := exec.CommandContext(ctx, "docker", "buildx", "inspect", "--bootstrap", SharedBuildxBuilderName)
	if output, err := bootstrapCmd.CombinedOutput(); err != nil {
		log.Warn().Err(err).Str("output", string(output)).Msg("Failed to bootstrap buildx builder")
	}

	return nil
}

