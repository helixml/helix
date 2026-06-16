package slack

import (
	"context"
	"errors"
)

// Ingress modes (FR-16). Selected by an explicit admin toggle on the
// global app, never inferred from which credentials are present.
const (
	IngressREST   = "rest"
	IngressSocket = "socket"
)

// App is the global, instance-wide Slack application an administrator
// configures once (§9.2). It carries everything the ingress sources and
// the OAuth install flow need; the per-org bot token + team id live
// separately in the configregistry.
type App struct {
	// ClientID / ClientSecret authorise the OAuth install exchange.
	ClientID     string
	ClientSecret string
	// SigningSecret verifies REST Events API request authenticity (NFR-4).
	SigningSecret string
	// AppToken is the app-level token (xapp-…) Socket Mode authenticates
	// the WebSocket with.
	AppToken string
	// IngressMode is the explicit REST/Socket toggle (FR-16). Empty means
	// no ingress is active.
	IngressMode string
	// Enabled mirrors the OAuthProvider row's Enabled flag. A disabled (or
	// absent) app makes the whole subsystem inert (FR-3).
	Enabled bool
}

// ErrNoApp signals that no global Slack app is configured (or it is
// disabled). Callers treat it as "feature dormant", not a hard error
// (FR-3).
var ErrNoApp = errors.New("no slack app configured")

// GlobalApp resolves the admin-configured global Slack app. Backed at
// the composition root by store.ListOAuthProviders(type=slack,
// enabled=true). Returns ErrNoApp when none is configured/enabled.
type GlobalApp func(ctx context.Context) (App, error)
