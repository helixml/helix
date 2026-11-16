package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// ListSandboxesQuery is the filter set for ListSandboxes.
type ListSandboxesQuery struct {
	OrganizationID string
	ProjectID      string
	Owner          string
	Status         types.SandboxStatus
	HostDeviceID   string
	SessionID      string
	IncludeDeleted bool
	// Visibility, when set, narrows the result to the rows a caller who is
	// not an organization owner may reach. Nil applies no access filter.
	Visibility *SandboxVisibility
}

// SandboxVisibility is the set of sandboxes a caller may see: rows in one of
// ProjectIDs, session-backed rows outside them owned by SessionOwner (a
// person may always reach their own sessions), and, when IncludeUnscoped,
// rows with neither a project nor a session. An empty value matches nothing.
type SandboxVisibility struct {
	ProjectIDs      []string
	SessionOwner    string
	IncludeUnscoped bool
}

// CreateSandbox inserts a new sandbox row.
func (s *PostgresStore) CreateSandbox(ctx context.Context, sandbox *types.Sandbox) (*types.Sandbox, error) {
	if sandbox.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}
	if sandbox.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}
	if sandbox.Runtime == "" {
		return nil, fmt.Errorf("runtime not specified")
	}
	if sandbox.ID == "" {
		sandbox.ID = system.GenerateSandboxID()
	}
	if sandbox.Status == "" {
		sandbox.Status = types.SandboxStatusPending
	}
	if sandbox.VCPUs == 0 {
		sandbox.VCPUs = 1
	}
	if sandbox.MemoryMB == 0 {
		sandbox.MemoryMB = 2048
	}
	if sandbox.TimeoutSeconds == 0 {
		sandbox.TimeoutSeconds = 3600
	}
	now := time.Now()
	sandbox.CreatedAt = now
	sandbox.UpdatedAt = now
	// TimeoutSeconds < 0 means "never expire" — leave ExpiresAt nil so the
	// reaper (which queries WHERE expires_at < now) skips it.
	if sandbox.ExpiresAt == nil && sandbox.TimeoutSeconds > 0 {
		exp := now.Add(time.Duration(sandbox.TimeoutSeconds) * time.Second)
		sandbox.ExpiresAt = &exp
	}

	if err := s.gdb.WithContext(ctx).Create(sandbox).Error; err != nil {
		return nil, err
	}
	return s.GetSandbox(ctx, sandbox.ID)
}

// GetSandbox returns a sandbox by ID, ignoring soft-deleted rows.
func (s *PostgresStore) GetSandbox(ctx context.Context, id string) (*types.Sandbox, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}
	var sb types.Sandbox
	err := s.gdb.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&sb).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sb, nil
}

// ListSandboxes returns sandboxes matching a query, newest first.
func (s *PostgresStore) ListSandboxes(ctx context.Context, q *ListSandboxesQuery) ([]*types.Sandbox, error) {
	query := s.gdb.WithContext(ctx).Model(&types.Sandbox{})

	if q != nil {
		if q.OrganizationID != "" {
			query = query.Where("organization_id = ?", q.OrganizationID)
		}
		if q.ProjectID != "" {
			query = query.Where("project_id = ?", q.ProjectID)
		}
		if q.Owner != "" {
			query = query.Where("owner = ?", q.Owner)
		}
		if q.Status != "" {
			query = query.Where("status = ?", q.Status)
		}
		if q.HostDeviceID != "" {
			query = query.Where("host_device_id = ?", q.HostDeviceID)
		}
		if q.SessionID != "" {
			query = query.Where("session_id = ?", q.SessionID)
		}
		if !q.IncludeDeleted {
			query = query.Where("deleted_at IS NULL")
		}
		if v := q.Visibility; v != nil {
			clause := s.gdb.Where("1 = 0")
			if len(v.ProjectIDs) > 0 {
				clause = clause.Or("project_id IN ?", v.ProjectIDs)
			}
			if v.SessionOwner != "" {
				clause = clause.Or("COALESCE(session_id, '') <> '' AND owner = ?", v.SessionOwner)
			}
			if v.IncludeUnscoped {
				clause = clause.Or("COALESCE(project_id, '') = '' AND COALESCE(session_id, '') = ''")
			}
			query = query.Where(clause)
		}
	}

	var sandboxes []*types.Sandbox
	err := query.Order("created_at DESC").Find(&sandboxes).Error
	if err != nil {
		return nil, err
	}
	return sandboxes, nil
}

// UpdateSandbox saves the entire row (use sparingly — prefer the targeted updaters below).
func (s *PostgresStore) UpdateSandbox(ctx context.Context, sandbox *types.Sandbox) (*types.Sandbox, error) {
	if sandbox.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}
	sandbox.UpdatedAt = time.Now()
	if err := s.gdb.WithContext(ctx).Save(sandbox).Error; err != nil {
		return nil, err
	}
	return s.GetSandbox(ctx, sandbox.ID)
}

// UpdateSandboxDetails writes the user-editable fields of a sandbox (name,
// tags, timeout and expiry) and nothing else, so a PATCH that overlaps a
// billing charge or a delete cannot write back a stale cursor or status.
func (s *PostgresStore) UpdateSandboxDetails(ctx context.Context, sandbox *types.Sandbox) (*types.Sandbox, error) {
	if sandbox.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}
	sandbox.UpdatedAt = time.Now()
	res := s.gdb.WithContext(ctx).Model(&types.Sandbox{}).
		Where("id = ? AND deleted_at IS NULL", sandbox.ID).
		Select("name", "tags", "timeout_seconds", "expires_at", "updated_at").
		Updates(sandbox)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return s.GetSandbox(ctx, sandbox.ID)
}

// SetSandboxStatus does a targeted UPDATE of the status fields, optionally
// recording a status message and started/stopped timestamps.
func (s *PostgresStore) SetSandboxStatus(ctx context.Context, id string, status types.SandboxStatus, message string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}
	updates := map[string]interface{}{
		"status":         status,
		"status_message": message,
		"updated_at":     time.Now(),
	}
	switch status {
	case types.SandboxStatusRunning:
		// Open the billing window only on the transition to running. A repeat
		// running write (a duplicate start) must not move the cursor past
		// usage that has not been billed yet.
		now := time.Now()
		updates["started_at"] = gorm.Expr("CASE WHEN status = ? THEN started_at ELSE ? END", types.SandboxStatusRunning, now)
		updates["billing_last_charged_at"] = gorm.Expr("CASE WHEN status = ? THEN billing_last_charged_at ELSE ? END", types.SandboxStatusRunning, now)
	case types.SandboxStatusStopped, types.SandboxStatusFailed:
		now := time.Now()
		updates["stopped_at"] = &now
	}
	res := s.gdb.WithContext(ctx).Model(&types.Sandbox{}).Where("id = ? AND deleted_at IS NULL", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SandboxCharge bills one window of a running sandbox's usage to its org.
type SandboxCharge struct {
	SandboxID string
	OrgID     string
	// From is the billing cursor (billing_last_charged_at) the caller read,
	// nil when it was unset. The charge applies only while it is still current.
	From *time.Time
	To   time.Time
	// Amount is the price of the window in credits. Zero advances the cursor
	// without touching the wallet (free runtimes, or opening the first window).
	Amount      float64
	Runtime     types.SandboxRuntime
	PricingType string
}

// ErrBillingWindowClaimed means another biller advanced the sandbox's billing
// cursor after it was read, so the window has already been charged.
var ErrBillingWindowClaimed = errors.New("sandbox billing window already charged")

// ChargeSandbox advances a sandbox's billing cursor from charge.From to
// charge.To and debits the org's wallet in one transaction. Two billers that
// read the same cursor — API replicas, or a stop racing the per-minute reaper —
// cannot both charge it, and a sandbox that is no longer running is never
// charged: the second gets ErrBillingWindowClaimed and changes
// nothing. When the wallet holds less than Amount it is drained to zero, the
// cursor still advances, and insufficient is true. If the debit fails, the
// cursor does not move.
func (s *PostgresStore) ChargeSandbox(ctx context.Context, charge SandboxCharge) (insufficient bool, err error) {
	if charge.SandboxID == "" || charge.OrgID == "" {
		return false, fmt.Errorf("sandbox_id and org_id are required")
	}
	if charge.Amount < 0 || math.IsNaN(charge.Amount) || math.IsInf(charge.Amount, 0) {
		return false, fmt.Errorf("charge amount must be a finite, non-negative number, got %f", charge.Amount)
	}
	if charge.To.IsZero() {
		return false, fmt.Errorf("charge window end is required")
	}
	if charge.From != nil && !charge.To.After(*charge.From) {
		return false, fmt.Errorf("charge window must end after %s, got %s", charge.From, charge.To)
	}
	err = s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&types.Sandbox{}).Where("id = ? AND organization_id = ? AND deleted_at IS NULL AND status = ?", charge.SandboxID, charge.OrgID, types.SandboxStatusRunning)
		if charge.From == nil {
			claim = claim.Where("billing_last_charged_at IS NULL")
		} else {
			claim = claim.Where("billing_last_charged_at = ?", *charge.From)
		}
		res := claim.Updates(map[string]interface{}{
			"billing_last_charged_at": charge.To,
			"updated_at":              time.Now(),
		})
		if res.Error != nil {
			return fmt.Errorf("advance billing cursor: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrBillingWindowClaimed
		}
		if charge.Amount == 0 {
			return nil
		}

		wallet, err := lockWallet(tx, "org_id = ?", charge.OrgID)
		if err != nil {
			return fmt.Errorf("lock wallet of org %s: %w", charge.OrgID, err)
		}
		debit := charge.Amount
		if wallet.Balance < debit {
			insufficient = true
			debit = max(wallet.Balance, 0)
		}
		if debit == 0 {
			return nil
		}
		return applyWalletDelta(tx, wallet.ID, wallet.Balance, -debit, types.TransactionMetadata{
			SandboxID:          charge.SandboxID,
			SandboxRuntime:     charge.Runtime,
			SandboxPricingType: charge.PricingType,
			TransactionType:    types.TransactionTypeUsage,
		})
	})
	if err != nil {
		return false, err
	}
	return insufficient, nil
}

// SetRunningSandboxesBillingLastChargedAt starts billing windows for every
// currently running sandbox. Used when billing is enabled at runtime so old
// free usage is not charged retroactively.
func (s *PostgresStore) SetRunningSandboxesBillingLastChargedAt(ctx context.Context, chargedAt time.Time) error {
	return s.gdb.WithContext(ctx).Model(&types.Sandbox{}).
		Where("deleted_at IS NULL AND status = ?", types.SandboxStatusRunning).
		Updates(map[string]interface{}{
			"billing_last_charged_at": &chargedAt,
			"updated_at":              time.Now(),
		}).Error
}

// SetSandboxContainer records the host_device_id and container_id once the
// scheduler has placed the sandbox on a hydra host.
func (s *PostgresStore) SetSandboxContainer(ctx context.Context, id string, hostDeviceID, containerID string) error {
	res := s.gdb.WithContext(ctx).Model(&types.Sandbox{}).Where("id = ? AND deleted_at IS NULL", id).Updates(map[string]interface{}{
		"host_device_id": hostDeviceID,
		"container_id":   containerID,
		"updated_at":     time.Now(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetSandboxBySession returns the live sandbox row that meters the container
// owned by a Helix session, or ErrNotFound when the session has none.
func (s *PostgresStore) GetSandboxBySession(ctx context.Context, sessionID string) (*types.Sandbox, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session id not specified")
	}
	var sb types.Sandbox
	err := s.gdb.WithContext(ctx).
		Where("session_id = ? AND deleted_at IS NULL", sessionID).
		Order("created_at DESC").
		First(&sb).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sb, nil
}

// SetSandboxResources records a new CPU/memory allocation after the runtime
// has actually been resized.
//
// Callers must settle any outstanding billing at the previous core count
// first — billing multiplies the whole unbilled window by the row's current
// VCPUs, so updating the allocation before flushing would retroactively
// reprice already-elapsed usage. See sandbox.Controller.ResizeSession.
func (s *PostgresStore) SetSandboxResources(ctx context.Context, id string, vcpus, memoryMB int) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}
	if vcpus <= 0 || memoryMB <= 0 {
		return fmt.Errorf("vcpus and memory_mb must be positive")
	}
	// Column is `v_cpus`, not `vcpus` — GORM's naming strategy splits the
	// leading acronym in the VCPUs field name.
	res := s.gdb.WithContext(ctx).Model(&types.Sandbox{}).Where("id = ? AND deleted_at IS NULL", id).Updates(map[string]interface{}{
		"v_cpus":     vcpus,
		"memory_mb":  memoryMB,
		"updated_at": time.Now(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSandbox soft-deletes by setting deleted_at + status=stopped.
func (s *PostgresStore) DeleteSandbox(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}
	now := time.Now()
	return s.gdb.WithContext(ctx).Model(&types.Sandbox{}).Where("id = ?", id).Updates(map[string]interface{}{
		"deleted_at": &now,
		"status":     types.SandboxStatusStopped,
		"stopped_at": &now,
		"updated_at": now,
	}).Error
}

// SumSandboxCharges totals the magnitude of every transaction tagged with
// this sandbox id, treating amounts as credits debited from the wallet (we
// abs() because usage transactions are recorded as negative deltas).
func (s *PostgresStore) SumSandboxCharges(ctx context.Context, sandboxID string) (float64, error) {
	if sandboxID == "" {
		return 0, fmt.Errorf("sandbox id not specified")
	}
	var sum float64
	err := s.gdb.WithContext(ctx).
		Model(&types.Transaction{}).
		Where("sandbox_id = ?", sandboxID).
		Select("COALESCE(SUM(ABS(amount)), 0)").
		Row().
		Scan(&sum)
	if err != nil {
		return 0, err
	}
	return sum, nil
}

// ListExpiredSandboxes returns sandboxes whose expires_at has passed and which
// haven't been deleted yet — used by the TTL reaper.
func (s *PostgresStore) ListExpiredSandboxes(ctx context.Context, now time.Time) ([]*types.Sandbox, error) {
	var sandboxes []*types.Sandbox
	err := s.gdb.WithContext(ctx).
		Where("deleted_at IS NULL AND expires_at IS NOT NULL AND expires_at < ?", now).
		Find(&sandboxes).Error
	if err != nil {
		return nil, err
	}
	return sandboxes, nil
}

// ListStoppedNonPersistentSandboxes returns stopped ephemeral sandboxes that
// have been stopped since before the cutoff and have not been deleted yet.
func (s *PostgresStore) ListStoppedNonPersistentSandboxes(ctx context.Context, before time.Time) ([]*types.Sandbox, error) {
	var sandboxes []*types.Sandbox
	err := s.gdb.WithContext(ctx).
		Where("deleted_at IS NULL AND status = ? AND persistent = ? AND stopped_at IS NOT NULL AND stopped_at < ?", types.SandboxStatusStopped, false, before).
		Find(&sandboxes).Error
	if err != nil {
		return nil, err
	}
	return sandboxes, nil
}
