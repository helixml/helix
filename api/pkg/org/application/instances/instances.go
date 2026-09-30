// Package instances declares the port for a Bot's instances: extra sessions
// with the Bot's identity and their own sandboxes. The REST adapter and the
// MCP tools both drive it; the Helix host implements it over the session
// store and the external-agent executor.
package instances

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/types"
)

// ErrForbidden means the caller may not act on the instance.
var ErrForbidden = errors.New("forbidden")

// ErrInvalidRequest means the request can never succeed as sent.
var ErrInvalidRequest = errors.New("invalid request")

// Params are the caller's choices for a new instance.
type Params struct {
	Name           string
	SandboxRuntime types.SandboxRuntime
	// DiskSizeGB is the hard size of the instance's persistent home filesystem.
	// Zero selects types.DefaultBotInstanceDiskSizeGB.
	DiskSizeGB int
	// AllowSudo deliberately permits setuid privilege escalation inside a
	// headless instance. False enables no-new-privileges and is the safe
	// default. Desktop instances always allow sudo.
	AllowSudo bool
	// IdleTimeoutSeconds overrides the bot profile's (and the deployment's)
	// idle timeout for this instance. Zero inherits the profile.
	IdleTimeoutSeconds int
	// Secrets names the project development secrets explicitly granted to this
	// instance. Empty means the instance receives no project secrets.
	Secrets []string
	// Message is queued as the instance's first turn. Empty starts the
	// sandbox with no turn.
	Message string
}

// Manager manages a Bot's instances.
type Manager interface {
	List(ctx context.Context, orgID string, botID orgchart.NodeID) ([]*types.Session, error)
	Create(ctx context.Context, orgID string, botID orgchart.NodeID, params Params) (*types.Session, error)
	// Delete stops the instance's sandbox, deletes its workspace and its
	// session. The Bot is untouched.
	Delete(ctx context.Context, orgID string, botID orgchart.NodeID, sessionID string) error
	// SyncProfile copies the Bot's current instance profile onto its
	// instances. It takes effect on each instance's next sandbox start.
	SyncProfile(ctx context.Context, orgID string, botID orgchart.NodeID) error
}

// NormalizeDiskSize validates a requested home disk size, applying the
// default for zero.
func NormalizeDiskSize(requestedGB int) (int, error) {
	if requestedGB == 0 {
		return types.DefaultBotInstanceDiskSizeGB, nil
	}
	if requestedGB < 1 || requestedGB > types.MaxBotInstanceDiskSizeGB {
		return 0, fmt.Errorf("%w: disk_size_gb must be between 1 and %d", ErrInvalidRequest, types.MaxBotInstanceDiskSizeGB)
	}
	return requestedGB, nil
}

// ValidateCredentials rejects a bot whose code agent runs on a subscription.
func ValidateCredentials(assistant *types.AssistantConfig) error {
	if assistant.CodeAgentCredentialType.IsSubscription() {
		return fmt.Errorf("%w: %w", ErrInvalidRequest, types.ErrBotInstanceSubscriptionCredentials)
	}
	return nil
}

// SelectSecrets validates the requested secret names against the project's
// secrets. Only development-scoped secrets can be granted: they are what a
// sandbox environment receives.
func SelectSecrets(requested []string, projectSecrets []*types.Secret) ([]string, error) {
	available := make(map[string]bool, len(projectSecrets))
	for _, secret := range projectSecrets {
		scope := secret.Scope
		if scope == "" {
			scope = types.SecretScopeDev
		}
		if scope.AppliesTo(types.SecretScopeDev) {
			available[secret.Name] = true
		}
	}
	selected := make([]string, 0, len(requested))
	for _, name := range requested {
		switch {
		case name == "":
			return nil, fmt.Errorf("%w: instance secret names cannot be empty", ErrInvalidRequest)
		case slices.Contains(selected, name):
			return nil, fmt.Errorf("%w: instance secret %q is listed more than once", ErrInvalidRequest, name)
		case !available[name]:
			return nil, fmt.Errorf("%w: project development secret %q does not exist", ErrInvalidRequest, name)
		}
		selected = append(selected, name)
	}
	return selected, nil
}
