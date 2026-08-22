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

// Claude OAuth access tokens last about 8 hours. Until this existed, the only
// thing that ever refreshed one was a running container pushing new credentials
// back (PUT /sessions/{id}/claude-credentials), so a subscription only stayed
// usable if you kept starting sessions: leave it 8 hours and the UI went red.
//
// What this does NOT do is make a subscription immortal. The refresh token is
// rotated on every use, but its expiry is anchored to the original sign-in and
// is not extended: two readings 9.2h apart, with a real refresh in between,
// showed the window shrink 9.25 -> 8.82 days. Anthropic's docs say the same
// ("The login lifetime itself is unchanged"). So this keeps the access token
// alive for the ~9 days the login is good for, and after that the user has to
// sign in again — which is exactly the trade-off the connect dialog states.
const (
	// claudeRefreshLeadTime is how far before expiry to refresh. Comfortably
	// longer than the reaper interval so a token is never left to lapse between
	// two passes.
	claudeRefreshLeadTime = 90 * time.Minute

	// claudeRefreshInterval is how often to sweep. Cheap: it only touches rows
	// that are actually close to expiring.
	claudeRefreshInterval = 15 * time.Minute

	// A rotated refresh token exists only in memory until it is persisted, so a
	// transient DB failure is retried rather than dropped.
	claudeRefreshPersistAttempts = 3
	claudeRefreshPersistBackoff  = 2 * time.Second
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
	if tokens.RefreshExpiresAt > 0 {
		creds.RefreshTokenExpiresAt = tokens.RefreshExpiresAt
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
	expiresAt := time.Time{}
	if creds.ExpiresAt > 0 {
		expiresAt = time.UnixMilli(creds.ExpiresAt)
	}
	refreshExpiresAt := time.Time{}
	if creds.RefreshTokenExpiresAt > 0 {
		refreshExpiresAt = time.UnixMilli(creds.RefreshTokenExpiresAt)
	}

	// Anthropic has already rotated the old refresh token, so the one we just
	// received is the only usable credential in existence. Losing it to a
	// transient DB error would brick the subscription permanently, which is
	// worth retrying for — and worth logging as an error, not a warning, if the
	// retries are exhausted.
	var persisted bool
	var persistErr error
	for attempt := 0; attempt < claudeRefreshPersistAttempts; attempt++ {
		persisted, persistErr = apiServer.Store.UpdateClaudeSubscriptionCredentialsIfNewer(
			ctx, sub.ID, encrypted, expiresAt, refreshExpiresAt, now)
		if persistErr == nil {
			break
		}
		if attempt+1 < claudeRefreshPersistAttempts {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(claudeRefreshPersistBackoff):
			}
		}
	}
	if persistErr != nil {
		log.Error().Err(persistErr).Str("subscription_id", sub.ID).
			Msg("Claude refresher: LOST a rotated refresh token — the stored one is now dead, re-authentication will be required")
		return false
	}
	if !persisted {
		// Something refreshed more recently than us — a container pushing its own
		// rotation. Theirs wins; ours is stale and must not overwrite it.
		log.Debug().Str("subscription_id", sub.ID).Msg("Claude refresher: newer credentials already stored, skipping")
		return false
	}

	log.Info().
		Str("subscription_id", sub.ID).
		Time("expires_at", expiresAt).
		Msg("Refreshed Claude subscription token")
	return true
}
