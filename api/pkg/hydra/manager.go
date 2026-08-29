package hydra

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

const (
	// DefaultSocketDir is the runtime directory for active docker sockets
	DefaultSocketDir = "/var/run/hydra/active"

	// DefaultDataDir is the persistent directory for docker data
	DefaultDataDir = "/hydra-data"

	// SharedBuildKitContainerName is retained only to retire the obsolete shared
	// builder after the last pre-upgrade session stops using it.
	SharedBuildKitContainerName = "helix-buildkit"
	SharedBuildxBuilderName     = "helix-shared"

	// SharedRegistryContainerName is the host-side recovery registry. It is not
	// exposed to session containers.
	SharedRegistryContainerName = "helix-registry"
	SharedRegistryImage         = "registry:2"
	SharedRegistryPort          = "5000"
)

// Manager manages the Hydra runtime and dev containers.
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

	if err := m.retireUnusedSharedBuildKit(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to evaluate legacy shared BuildKit retirement")
	}
	if err := m.setupSharedRegistry(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to set up shared recovery registry")
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

// retireUnusedSharedBuildKit removes the privileged shared builder only after
// every pre-upgrade container that still references it has gone away. Its
// persistent cache volume is deliberately left intact.
func (m *Manager) retireUnusedSharedBuildKit(ctx context.Context) error {
	consumer, err := m.legacyContainerConsumer(ctx, SharedBuildKitContainerName, "BUILDKIT_HOST")
	if err != nil {
		return err
	}
	if consumer != "" {
		log.Info().Str("container", consumer).Msg("Retaining shared BuildKit for a pre-upgrade session")
		return nil
	}

	removed, err := removeLegacyContainer(ctx, SharedBuildKitContainerName)
	if err != nil {
		return err
	}
	if !removed {
		return nil
	}
	_ = exec.CommandContext(ctx, "docker", "buildx", "rm", SharedBuildxBuilderName).Run()
	log.Info().Msg("Retired obsolete shared BuildKit container; persistent cache was preserved")
	return nil
}

func (m *Manager) legacyContainerConsumer(ctx context.Context, infrastructureName, envName string) (string, error) {
	if err := exec.CommandContext(ctx, "docker", "inspect", infrastructureName).Run(); err != nil {
		return "", nil
	}

	// Stopped legacy sessions are recreated on the isolated network when they
	// next start, so only a running container can still depend on this service.
	containerIDs, err := exec.CommandContext(ctx, "docker", "ps", "-q").Output()
	if err != nil {
		return "", fmt.Errorf("list containers before retiring %s: %w", infrastructureName, err)
	}
	for _, containerID := range strings.Fields(string(containerIDs)) {
		nameOutput, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.Name}}", containerID).Output()
		if err != nil {
			return "", fmt.Errorf("inspect container %s name: %w", containerID, err)
		}
		if strings.TrimSpace(string(nameOutput)) == "/"+infrastructureName {
			continue
		}
		envOutput, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", containerID).Output()
		if err != nil {
			return "", fmt.Errorf("inspect container %s environment: %w", containerID, err)
		}
		for _, env := range strings.Split(string(envOutput), "\n") {
			if strings.HasPrefix(env, envName+"=") && strings.TrimPrefix(env, envName+"=") != "" {
				return containerID, nil
			}
		}
	}
	return "", nil
}

func removeLegacyContainer(ctx context.Context, containerName string) (bool, error) {
	if err := exec.CommandContext(ctx, "docker", "inspect", containerName).Run(); err != nil {
		return false, nil
	}
	if output, err := exec.CommandContext(ctx, "docker", "rm", "-f", containerName).CombinedOutput(); err != nil {
		return false, fmt.Errorf("remove obsolete %s container: %w, output: %s", containerName, err, strings.TrimSpace(string(output)))
	}
	return true, nil
}

// setupSharedRegistry keeps the host-side image recovery path available. The
// session firewall blocks this container, and Hydra never passes its address
// into session environments.
func (m *Manager) setupSharedRegistry(ctx context.Context) error {
	checkCmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", SharedRegistryContainerName)
	output, err := checkCmd.Output()
	if err == nil && strings.TrimSpace(string(output)) == "true" {
		log.Debug().Str("container", SharedRegistryContainerName).Msg("Shared recovery registry already running")
		return nil
	}

	_ = exec.CommandContext(ctx, "docker", "rm", "-f", SharedRegistryContainerName).Run()
	createCmd := exec.CommandContext(ctx, "docker", "run", "-d",
		"--name", SharedRegistryContainerName,
		"-v", "registry_data:/var/lib/registry",
		"--restart", "unless-stopped",
		SharedRegistryImage,
	)
	if output, err := createCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create shared recovery registry: %w, output: %s", err, strings.TrimSpace(string(output)))
	}

	log.Info().Str("container", SharedRegistryContainerName).Msg("Shared recovery registry started")
	return nil
}
