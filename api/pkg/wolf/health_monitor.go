package wolf

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/rs/zerolog/log"
)

// HealthMonitor monitors Wolf service health and auto-restarts on failure
type HealthMonitor struct {
	client              *Client
	consecutiveFailures int
	maxFailures         int
	checkInterval       time.Duration
	onWolfRestarted     func(ctx context.Context) // Callback for post-restart reconciliation
}

// NewHealthMonitor creates a new Wolf health monitor
func NewHealthMonitor(client *Client, onWolfRestarted func(ctx context.Context)) *HealthMonitor {
	return &HealthMonitor{
		client:          client,
		maxFailures:     3,                  // Fail after 3 consecutive failures
		checkInterval:   5 * time.Second,    // Check every 5 seconds
		onWolfRestarted: onWolfRestarted,
	}
}

// Start begins the health monitoring loop
func (m *HealthMonitor) Start(ctx context.Context) {
	log.Info().Msg("Starting Wolf health monitor")
	go m.monitorLoop(ctx)
}

// monitorLoop runs the health check loop
func (m *HealthMonitor) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Wolf health monitor stopped")
			return

		case <-ticker.C:
			if err := m.checkHealth(ctx); err != nil {
				m.consecutiveFailures++

				log.Error().
					Err(err).
					Int("consecutive_failures", m.consecutiveFailures).
					Int("max_failures", m.maxFailures).
					Msg("Wolf health check failed")

				if m.consecutiveFailures >= m.maxFailures {
					log.Error().Msg("Wolf health check failed 3 times, attempting to restart container")

					if err := m.restartWolfContainer(ctx); err != nil {
						log.Error().Err(err).Msg("Failed to restart Wolf container")
						// Reset failure count even on restart failure to avoid spam
						m.consecutiveFailures = 0
					} else {
						log.Info().Msg("Wolf container restarted successfully")
						m.consecutiveFailures = 0

						// Wait for Wolf to be healthy before reconciling
						if err := m.waitForHealthy(ctx, 30*time.Second); err != nil {
							log.Error().Err(err).Msg("Wolf failed to become healthy after restart")
						} else {
							log.Info().Msg("Wolf is healthy after restart, triggering reconciliation")
							// Trigger keepalive reconciliation
							if m.onWolfRestarted != nil {
								m.onWolfRestarted(ctx)
							}
						}
					}
				}
			} else {
				// Health check passed, reset failure count
				if m.consecutiveFailures > 0 {
					log.Info().
						Int("consecutive_failures", m.consecutiveFailures).
						Msg("Wolf health check passed, resetting failure count")
					m.consecutiveFailures = 0
				}
			}
		}
	}
}

// checkHealth performs a single health check
func (m *HealthMonitor) checkHealth(ctx context.Context) error {
	// Create a timeout context for this specific health check
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	return m.client.CheckHealth(checkCtx)
}

// restartWolfContainer restarts the Wolf Docker container
func (m *HealthMonitor) restartWolfContainer(ctx context.Context) error {
	log.Info().Msg("Restarting Wolf container via Docker Compose")

	// Use Docker Compose to restart Wolf
	// First, stop the container
	downCmd := exec.CommandContext(ctx, "docker", "compose", "-f", "docker-compose.dev.yaml", "down", "wolf")
	if output, err := downCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stop Wolf container: %w (output: %s)", err, string(output))
	}

	log.Info().Msg("Wolf container stopped, starting it again")

	// Then, start it again
	upCmd := exec.CommandContext(ctx, "docker", "compose", "-f", "docker-compose.dev.yaml", "up", "-d", "wolf")
	if output, err := upCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start Wolf container: %w (output: %s)", err, string(output))
	}

	log.Info().Msg("Wolf container started successfully")
	return nil
}

// waitForHealthy waits for Wolf to become healthy after restart
func (m *HealthMonitor) waitForHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if err := m.checkHealth(ctx); err == nil {
			return nil // Wolf is healthy
		}

		// Wait 2 seconds before next check
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
			// Continue checking
		}
	}

	return fmt.Errorf("Wolf did not become healthy within %v", timeout)
}
