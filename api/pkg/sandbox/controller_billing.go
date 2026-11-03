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
	return c.billSandbox(ctx, settings, sb, now, true, false)
}

// ReapBilling charges running sandboxes for complete elapsed minutes. If the
// org wallet cannot cover the next minute, the sandbox is stopped immediately.
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
		if err := c.billSandbox(ctx, settings, sb, now, false, true); err != nil {
			log.Warn().Err(err).Str("sandbox_id", sb.ID).Msg("sandbox billing failed")
		}
	}
	return nil
}

func (c *Controller) billSandbox(ctx context.Context, settings *types.SystemSettings, sb *types.Sandbox, now time.Time, includePartialMinute bool, stopOnInsufficient bool) error {
	pricePerSecond, pricingType := sandboxPrice(settings, sb.Runtime)
	if pricePerSecond <= 0 {
		return c.store.SetSandboxBillingLastChargedAt(ctx, sb.ID, now)
	}

	from := sb.BillingLastChargedAt
	if from == nil {
		from = sb.StartedAt
	}
	if from == nil {
		return c.store.SetSandboxBillingLastChargedAt(ctx, sb.ID, now)
	}

	elapsedSeconds := now.Sub(*from).Seconds()
	chargeSeconds := math.Floor(elapsedSeconds/60) * 60
	if includePartialMinute {
		chargeSeconds = elapsedSeconds
	}
	if chargeSeconds <= 0 {
		return nil
	}

	wallet, err := c.store.GetWalletByOrg(ctx, sb.OrganizationID)
	if err != nil {
		return fmt.Errorf("get org wallet: %w", err)
	}
	insufficient, err := c.store.ChargeSandbox(ctx, store.SandboxCharge{
		SandboxID: sb.ID,
		From:      sb.BillingLastChargedAt,
		To:        from.Add(time.Duration(chargeSeconds * float64(time.Second))),
		WalletID:  wallet.ID,
		Amount:    pricePerSecond * chargeSeconds * float64(sandboxBillableCores(sb)),
		Meta: types.TransactionMetadata{
			SandboxID:          sb.ID,
			SandboxRuntime:     sb.Runtime,
			SandboxPricingType: pricingType,
			TransactionType:    types.TransactionTypeUsage,
		},
	})
	if errors.Is(err, store.ErrBillingWindowClaimed) {
		// Another replica, or a stop racing this tick, already billed it.
		return nil
	}
	if err != nil {
		return fmt.Errorf("charge sandbox usage: %w", err)
	}
	if !insufficient || !stopOnInsufficient {
		return nil
	}
	if err := c.Delete(ctx, sb.ID); err != nil {
		return fmt.Errorf("stop sandbox after insufficient credits: %w", err)
	}
	return nil
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
