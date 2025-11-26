package external_agent

import (
	"context"
	"net/http"

	"github.com/helixml/helix/api/pkg/wolf"
)

// WolfClientInterface defines the operations needed from the Wolf client
// This interface allows for easier testing by enabling mock implementations
type WolfClientInterface interface {
	AddApp(ctx context.Context, app *wolf.App) error
	RemoveApp(ctx context.Context, appID string) error
	ListApps(ctx context.Context) ([]wolf.App, error)
	CreateLobby(ctx context.Context, req *wolf.CreateLobbyRequest) (*wolf.LobbyCreateResponse, error)
	JoinLobby(ctx context.Context, req *wolf.JoinLobbyRequest) error
	StopLobby(ctx context.Context, req *wolf.StopLobbyRequest) error
	ListLobbies(ctx context.Context) ([]wolf.Lobby, error)
	ListSessions(ctx context.Context) ([]wolf.WolfStreamSession, error)
	StopSession(ctx context.Context, clientID string) error
	GetSystemMemory(ctx context.Context) (*wolf.SystemMemoryResponse, error)
	GetSystemHealth(ctx context.Context) (*wolf.SystemHealthResponse, error)
	// Pairing operations (used by Wolf pairing handlers)
	GetPendingPairRequests() ([]wolf.PendingPairRequest, error)
	PairClient(pairSecret, pin string) error
	// Keyboard state observability (used for debugging stuck modifier keys)
	GetKeyboardState(ctx context.Context) (*wolf.KeyboardStateResponse, error)
	ResetKeyboardState(ctx context.Context, sessionID string) (*wolf.KeyboardResetResponse, error)
	// Raw HTTP access (used for SSE streaming)
	Get(ctx context.Context, path string) (*http.Response, error)
}

// Ensure *wolf.Client implements WolfClientInterface at compile time
var _ WolfClientInterface = (*wolf.Client)(nil)
