// Package instances declares the port for a Bot's instances: extra sessions
// with the Bot's identity and their own sandboxes. The REST adapter and the
// MCP tools both drive it; the Helix host implements it over the session
// store and the external-agent executor.
package instances

import (
	"context"
	"errors"

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
