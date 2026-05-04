package sandbox

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

const stoppedSandboxCleanupDelay = time.Hour

// CleanupStoppedNonPersistent deletes stopped ephemeral sandboxes after a
// grace period. Persistent sandboxes are intentionally retained until the user
// explicitly deletes them because their workspace mount is part of the product
// contract.
func (c *Controller) CleanupStoppedNonPersistent(ctx context.Context) error {
	cutoff := time.Now().Add(-stoppedSandboxCleanupDelay)
	stopped, err := c.store.ListStoppedNonPersistentSandboxes(ctx, cutoff)
	if err != nil {
		return err
	}
	for _, sb := range stopped {
		log.Info().Str("sandbox_id", sb.ID).Msg("cleaning up stopped non-persistent sandbox")
		if err := c.Delete(ctx, sb.ID); err != nil {
			log.Warn().Err(err).Str("sandbox_id", sb.ID).Msg("failed to clean up stopped non-persistent sandbox")
		}
	}
	return nil
}
