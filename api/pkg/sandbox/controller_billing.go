package sandbox

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func (c *Controller) billSandboxFinal(ctx context.Context, sb *types.Sandbox, now time.Time) error {
	if sb.Status != types.SandboxStatusRunning {
		return nil
	}
	settings, err := c.store.GetSystemSettings(ctx)
	if err != nil {
		return fmt.Errorf("get system settings: %w", err)
	}
	if !settings.SandboxBillingEnabled {
		return nil
	}
	return c.settleSandbox(ctx, settings, sb, now)
}

// ReapBilling charges running sandboxes for complete elapsed minutes. A
// sandbox whose org could not pay for the elapsed window has its wallet
// drained to zero and is deleted.
func (c *Controller) ReapBilling(ctx context.Context) error {
	settings, err := c.store.GetSystemSettings(ctx)
	if err != nil {
		return fmt.Errorf("get system settings: %w", err)
	}
	if !settings.SandboxBillingEnabled {
		return nil
	}

	running, err := c.store.ListSandboxes(ctx, &store.ListSandboxesQuery{
		Status: types.SandboxStatusRunning,
	})
	if err != nil {
		return fmt.Errorf("list running sandboxes: %w", err)
	}

	now := time.Now()
	for _, sb := range running {
		insufficient, err := c.chargeWindow(ctx, settings, sb, now, false)
		if errors.Is(err, store.ErrBillingWindowClaimed) {
			continue // another replica, or a stop, billed it first
		}
		if err != nil {
			log.Warn().Err(err).Str("sandbox_id", sb.ID).Str("org_id", sb.OrganizationID).Msg("sandbox billing failed")
			continue
		}
		if !insufficient {
			continue
		}
		if err := c.Delete(ctx, sb.ID); err != nil {
			log.Warn().Err(err).Str("sandbox_id", sb.ID).Str("org_id", sb.OrganizationID).Msg("failed to stop sandbox after insufficient credits")
		}
	}
	return nil
}

// settleMaxAttempts bounds settleSandbox's retries. Each retry follows a
// concurrent biller that has just moved the cursor forward, so a second
// attempt almost always succeeds.
const settleMaxAttempts = 3

// settleSandbox bills a running sandbox up to now, including the partial
// minute, before it stops or changes size. If a concurrent biller moves the
// cursor first, it reloads the row and bills the remainder, so neither the
// tail of the window nor the old core count is lost.
func (c *Controller) settleSandbox(ctx context.Context, settings *types.SystemSettings, sb *types.Sandbox, now time.Time) error {
	for range settleMaxAttempts {
		_, err := c.chargeWindow(ctx, settings, sb, now, true)
		if !errors.Is(err, store.ErrBillingWindowClaimed) {
			return err
		}
		if sb, err = c.store.GetSandbox(ctx, sb.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil // deleted by the biller that won; it settled the window
			}
			return fmt.Errorf("reload sandbox after concurrent charge: %w", err)
		}
		if sb.Status != types.SandboxStatusRunning {
			return nil
		}
	}
	return fmt.Errorf("settle sandbox %s: billing cursor kept moving after %d attempts", sb.ID, settleMaxAttempts)
}

// chargeWindow charges sb from its billing cursor up to now: whole elapsed
// minutes, or every elapsed second when partialMinute is set. A free runtime
// still advances the cursor, so enabling a price later never bills the past.
// Returns store.ErrBillingWindowClaimed if another biller moved the cursor.
func (c *Controller) chargeWindow(ctx context.Context, settings *types.SystemSettings, sb *types.Sandbox, now time.Time, partialMinute bool) (insufficient bool, err error) {
	pricePerSecond, pricingType := sandboxPrice(settings, sb.Runtime)
	charge := store.SandboxCharge{
		SandboxID:   sb.ID,
		OrgID:       sb.OrganizationID,
		From:        sb.BillingLastChargedAt,
		To:          now,
		Runtime:     sb.Runtime,
		PricingType: pricingType,
	}
	// charge.From stays the raw cursor: it is the compare-and-set key, and a
	// legacy row with no cursor is claimed by matching NULL. from is only
	// where pricing starts.
	from := sb.BillingLastChargedAt
	if from == nil {
		from = sb.StartedAt
	}
	if from != nil && !now.After(*from) {
		return false, nil
	}
	if pricePerSecond > 0 && from != nil {
		chargeSeconds := now.Sub(*from).Seconds()
		if !partialMinute {
			chargeSeconds = math.Floor(chargeSeconds/60) * 60
		}
		if chargeSeconds <= 0 {
			return false, nil
		}
		charge.To = from.Add(time.Duration(chargeSeconds * float64(time.Second)))
		charge.Amount = pricePerSecond * chargeSeconds * float64(sandboxBillableCores(sb))
	}
	insufficient, err = c.store.ChargeSandbox(ctx, charge)
	if err != nil && !errors.Is(err, store.ErrBillingWindowClaimed) {
		return false, fmt.Errorf("charge sandbox usage: %w", err)
	}
	return insufficient, err
}

func (c *Controller) ensureSandboxCredits(ctx context.Context, orgID string, spec *RuntimeSpec, settings *types.SystemSettings, vcpus int) error {
	return c.ensureCreditsForType(ctx, orgID, sandboxPricingTypeForSpec(spec), settings, vcpus)
}

// ErrInsufficientCredits means the organization's wallet cannot pay for a
// minute of the requested sandbox. Retrying cannot succeed until it is topped
// up.
var ErrInsufficientCredits = errors.New("insufficient credits to start sandbox")

// ensureCreditsForType requires the org to hold at least one minute of runway
// at the requested size before we start (or grow) a container.
func (c *Controller) ensureCreditsForType(ctx context.Context, orgID, pricingType string, settings *types.SystemSettings, vcpus int) error {
	if !settings.SandboxBillingEnabled {
		return nil
	}
	pricePerSecond := sandboxPriceForType(settings, pricingType)
	if pricePerSecond <= 0 {
		return nil
	}
	if vcpus < 1 {
		vcpus = 1
	}
	required := pricePerSecond * 60 * float64(vcpus)
	wallet, err := c.store.GetWalletByOrg(ctx, orgID)
	if err != nil {
		return fmt.Errorf("get org wallet: %w", err)
	}
	if wallet.Balance < required {
		return fmt.Errorf("%w: balance %.2f, required %.2f", ErrInsufficientCredits, wallet.Balance, required)
	}
	return nil
}

func sandboxBillableCores(sb *types.Sandbox) int {
	if sb != nil && sb.VCPUs > 0 {
		return sb.VCPUs
	}
	return 1
}

func (c *Controller) ensureSandboxLimits(ctx context.Context, orgID string, spec *RuntimeSpec, settings *types.SystemSettings) error {
	return c.ensureSandboxLimitsForType(ctx, orgID, sandboxPricingTypeForSpec(spec), "", settings)
}

// ensureSandboxLimitsForType enforces the org's concurrency cap for one
// pricing class. excludeID drops a single row from the count, so a restart
// isn't blocked by the slot it is itself about to reoccupy.
func (c *Controller) ensureSandboxLimitsForType(ctx context.Context, orgID, pricingType, excludeID string, settings *types.SystemSettings) error {
	if settings == nil {
		settings = &types.SystemSettings{}
	}
	limit := settings.EffectiveMaxConcurrentHeadlessSandboxes()
	if pricingType == "desktop" {
		pro := false
		if settings.EnforceQuotas && settings.MaxConcurrentDesktopSandboxes <= 0 {
			wallet, err := c.store.GetWalletByOrg(ctx, orgID)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("get organization wallet: %w", err)
			}
			pro = wallet.UsesProQuotaTier()
		}
		limit = settings.EffectiveMaxConcurrentDesktopSandboxesForPlan(pro)
	}

	sandboxes, err := c.store.ListSandboxes(ctx, &store.ListSandboxesQuery{
		OrganizationID: orgID,
	})
	if err != nil {
		return fmt.Errorf("list organization sandboxes: %w", err)
	}

	active := 0
	for _, sb := range sandboxes {
		if sb == nil || !isActiveSandboxStatus(sb.Status) {
			continue
		}
		if excludeID != "" && sb.ID == excludeID {
			continue
		}
		if sandboxPricingTypeForRuntime(sb.Runtime) == pricingType {
			active++
		}
	}
	if active >= limit {
		return fmt.Errorf("organization has reached the %s sandbox concurrency limit (%d)", pricingType, limit)
	}
	return nil
}

func sandboxPriceForType(settings *types.SystemSettings, pricingType string) float64 {
	if pricingType == "desktop" {
		return settings.SandboxDesktopPriceCreditsPerSecond
	}
	return settings.SandboxHeadlessPriceCreditsPerSecond
}

func sandboxPrice(settings *types.SystemSettings, runtime types.SandboxRuntime) (float64, string) {
	pricingType := sandboxPricingTypeForRuntime(runtime)
	return sandboxPriceForType(settings, pricingType), pricingType
}

func sandboxPricingTypeForSpec(spec *RuntimeSpec) string {
	if spec != nil && spec.ContainerType == hydra.DevContainerTypeUbuntu {
		return "desktop"
	}
	return "headless"
}

func sandboxPricingTypeForRuntime(runtime types.SandboxRuntime) string {
	if runtime == types.SandboxRuntimeUbuntuDesktop {
		return "desktop"
	}
	return "headless"
}

func isActiveSandboxStatus(status types.SandboxStatus) bool {
	return status == types.SandboxStatusPending || status == types.SandboxStatusRunning || status == types.SandboxStatusStopping
}
