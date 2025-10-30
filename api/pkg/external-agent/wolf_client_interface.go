package external_agent

import (
	"context"

	"github.com/helixml/helix/api/pkg/wolf"
)

// WolfClientInterface defines the operations needed from the Wolf client
// This interface allows for easier testing by enabling mock implementations
type WolfClientInterface interface {
	AddApp(ctx context.Context, app *wolf.App) error
	RemoveApp(ctx context.Context, appID string) error
	CreateSession(ctx context.Context, session *wolf.Session) (string, error)
	StopSession(ctx context.Context, sessionID string) error
	ListApps(ctx context.Context) ([]wolf.App, error)
	CreateLobby(ctx context.Context, req *wolf.CreateLobbyRequest) (*wolf.LobbyCreateResponse, error)
	StopLobby(ctx context.Context, req *wolf.StopLobbyRequest) error
	ListLobbies(ctx context.Context) ([]wolf.Lobby, error)
}

// Ensure *wolf.Client implements WolfClientInterface at compile time
var _ WolfClientInterface = (*wolf.Client)(nil)
