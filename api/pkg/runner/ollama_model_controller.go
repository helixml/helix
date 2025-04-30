package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

func (r *Runner) startHelixModelReconciler(ctx context.Context) error {
	ticker := time.NewTicker(time.Second * 30)
	for {
		select {
		case <-ticker.C:
			err := r.reconcileHelixModels(ctx)
			if err != nil {
				log.Error().Err(err).Msg("error reconciling helix models")
			}
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while reconciling helix models")
		}
	}
}
func (r *Runner) reconcileHelixModels(ctx context.Context) error {
	// TODO:
	log.Info().Msg("reconciling helix models")
	return nil
}
