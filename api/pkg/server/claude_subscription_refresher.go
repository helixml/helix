package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/helixml/helix/api/pkg/anthropic"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Claude OAuth access tokens last about 8 hours; the refresh token behind them
// lasts about 9 days and is rotated on every use. Until this existed, the only
// thing that ever refreshed a subscription was a running container pushing new
// credentials back (PUT /sessions/{id}/claude-credentials). That meant a
// subscription only stayed alive if you kept starting sessions: leave it 8
// hours and the UI went red, leave it ~9 days and the refresh window closed and
// the subscription was dead for good.
const (
	// claudeRefreshLeadTime is how far before expiry to refresh. Comfortably
	// longer than the reaper interval so a token is never left to lapse between
	// two passes.
	claudeRefreshLeadTime = 90 * time.Minute

	// claudeRefreshInterval is how often to sweep. Cheap: it only touches rows
	// that are actually close to expiring.
	claudeRefreshInterval = 15 * time.Minute
)

// StartClaudeSubscriptionRefresher keeps stored Claude OAuth credentials alive
// in the background, so a subscription stays healthy whether or not anyone is
// running sessions with it.
func (apiServer *HelixAPIServer) StartClaudeSubscriptionRefresher(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = claudeRefreshInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Sweep once at boot: an API that has been down over a weekend should not
	// wait a further interval before rescuing tokens that are about to lapse.
	apiServer.refreshExpiringClaudeSubscriptions(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			apiServer.refreshExpiringClaudeSubscriptions(ctx)
		}
	}
}

func (apiServer *HelixAPIServer) refreshExpiringClaudeSubscriptions(ctx context.Context) {
	// An empty owner id lists every subscription.
	subs, err := apiServer.Store.ListClaudeSubscriptions(ctx, "")
	if err != nil {
		log.Warn().Err(err).Msg("Claude refresher: failed to list subscriptions")
		return
	}

	refreshed := 0
	for _, sub := range subs {
		if !claudeSubscriptionNeedsRefresh(sub, time.Now()) {
			continue
		}
		if apiServer.refreshClaudeSubscription(ctx, sub) {
			refreshed++
		}
	}
	if refreshed > 0 {
		log.Info().Int("refreshed", refreshed).Msg("Refreshed Claude subscription tokens")
	}
}

// claudeSubscriptionNeedsRefresh selects rows worth spending a request on.
func claudeSubscriptionNeedsRefresh(sub *types.ClaudeSubscription, now time.Time) bool {
	if sub == nil {
		return false
	}
	// Setup tokens carry no refresh token and effectively never expire, so
	// there is nothing to refresh and nothing to gain from trying.
	if sub.CredentialType == "setup_token" {
		return false
	}
	// A zero expiry means we never recorded one; refreshing blind would burn a
	// request on every sweep forever.
	if sub.AccessTokenExpiresAt.IsZero() {
		return false
	}
	// Already-expired rows are still worth trying: the access token is dead but
	// the refresh token usually has days left, and this is exactly the case
	// that used to strand a subscription permanently.
	return sub.AccessTokenExpiresAt.Before(now.Add(claudeRefreshLeadTime))
}

// refreshClaudeSubscription refreshes one subscription in place. Returns true
// when the row was updated.
func (apiServer *HelixAPIServer) refreshClaudeSubscription(ctx context.Context, sub *types.ClaudeSubscription) bool {
	key, err := crypto.GetEncryptionKey()
	if err != nil {
		log.Warn().Err(err).Msg("Claude refresher: encryption key unavailable")
		return false
	}
	plaintext, err := crypto.DecryptAES256GCM(sub.EncryptedCredentials, key)
	if err != nil {
		log.Warn().Str("subscription_id", sub.ID).Msg("Claude refresher: failed to decrypt credentials")
		return false
	}
	var creds types.ClaudeOAuthCredentials
	if err := json.Unmarshal(plaintext, &creds); err != nil || creds.RefreshToken == "" {
		log.Debug().Str("subscription_id", sub.ID).Msg("Claude refresher: no refresh token on this credential")
		return false
	}

	tokens, err := anthropic.RefreshClaudeToken(ctx, creds.RefreshToken)
	if err != nil {
		// Do not downgrade the row on a refresh failure. A network blip must not
		// mark a working subscription dead, and a genuinely dead refresh token
		// is caught by the liveness probe, which owns Status.
		log.Debug().Str("subscription_id", sub.ID).Str("detail", err.Error()).Msg("Claude refresher: refresh failed")
		return false
	}

	creds.AccessToken = tokens.AccessToken
	creds.RefreshToken = tokens.RefreshToken
	if tokens.ExpiresAt > 0 {
		creds.ExpiresAt = tokens.ExpiresAt
	}
	if len(tokens.Scopes) > 0 {
		creds.Scopes = tokens.Scopes
	}

	updatedJSON, err := json.Marshal(creds)
	if err != nil {
		return false
	}
	encrypted, err := crypto.EncryptAES256GCM(updatedJSON, key)
	if err != nil {
		return false
	}

	now := time.Now()
	sub.EncryptedCredentials = encrypted
	sub.LastRefreshedAt = &now
	if creds.ExpiresAt > 0 {
		sub.AccessTokenExpiresAt = time.UnixMilli(creds.ExpiresAt)
	}
	// A row that was showing an expiry-related error is demonstrably fine again.
	if sub.Status != "active" {
		sub.Status = "active"
		sub.LastError = ""
	}

	if _, err := apiServer.Store.UpdateClaudeSubscription(ctx, sub); err != nil {
		log.Warn().Err(err).Str("subscription_id", sub.ID).Msg("Claude refresher: failed to persist refreshed credentials")
		return false
	}
	log.Info().
		Str("subscription_id", sub.ID).
		Time("expires_at", sub.AccessTokenExpiresAt).
		Msg("Refreshed Claude subscription token")
	return true
}
