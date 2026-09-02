package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
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

// ReconcileWebServiceContainers removes physical containers whose exact
// sandbox row was already superseded and soft-deleted. Every missing piece of
// evidence retains the container so a current customer web service is never
// removed on an inference.
func (c *Controller) ReconcileWebServiceContainers(ctx context.Context) error {
	sandboxes, err := c.store.ListSandboxes(ctx, &store.ListSandboxesQuery{IncludeDeleted: true})
	if err != nil {
		return fmt.Errorf("list sandbox history: %w", err)
	}
	byID := make(map[string]*types.Sandbox, len(sandboxes))
	for _, sb := range sandboxes {
		if sb != nil {
			byID[sb.ID] = sb
		}
	}

	hosts, err := c.store.ListSandboxInstances(ctx)
	if err != nil {
		return fmt.Errorf("list sandbox instances: %w", err)
	}
	var errs []error
	for _, host := range hosts {
		if host.Status != "online" {
			continue
		}
		hydraClient := c.newHydraClient(host.ID)
		listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		containers, listErr := hydraClient.ListDevContainers(listCtx)
		cancel()
		if listErr != nil {
			errs = append(errs, fmt.Errorf("list dev containers on %s: %w", host.ID, listErr))
			continue
		}
		if containers == nil {
			continue
		}
		for _, container := range containers.Containers {
			sb := byID[container.SessionID]
			if sb == nil || sb.HostDeviceID != host.ID || sb.DeletedAt == nil || sb.Purpose != types.SandboxPurposeWebService || sb.ProjectID == "" {
				continue
			}

			deleteCtx, deleteCancel := context.WithTimeout(ctx, 5*time.Minute)
			state, stateErr := c.store.GetProjectWebServiceState(deleteCtx, sb.ProjectID)
			if stateErr != nil {
				deleteCancel()
				errs = append(errs, fmt.Errorf("get web service state for project %s: %w", sb.ProjectID, stateErr))
				continue
			}
			if state == nil || state.ActiveSandboxID == "" || state.ActiveSandboxID == sb.ID {
				deleteCancel()
				continue
			}
			if _, err := hydraClient.DeleteDevContainer(deleteCtx, sb.ID); err != nil {
				deleteCancel()
				errs = append(errs, fmt.Errorf("delete superseded web service sandbox %s: %w", sb.ID, err))
				continue
			}
			if err := c.store.DecrementSandboxContainerCount(deleteCtx, host.ID); err != nil {
				log.Warn().Err(err).Str("sandbox_id", sb.ID).Str("host_device_id", host.ID).
					Msg("failed to decrement active_sandboxes after reconciling web service container")
			}
			if err := hydraClient.ForgetSandboxOps(deleteCtx, sb.ID); err != nil {
				log.Debug().Err(err).Str("sandbox_id", sb.ID).Msg("hydra ForgetSandboxOps failed after web service reconciliation")
			}
			deleteCancel()
			log.Info().Str("sandbox_id", sb.ID).Str("active_sandbox_id", state.ActiveSandboxID).
				Msg("deleted superseded web service container")
		}
	}
	return errors.Join(errs...)
}
