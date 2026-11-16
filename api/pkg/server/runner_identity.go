package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Runner credentials come in two shapes:
//
//   - the shared RUNNER_TOKEN, held by every sandbox host (and the control
//     plane itself). It says nothing about which host is calling.
//   - a per-runner token (types.RunnerTokenPrefix), minted by an admin for
//     one runner id. Only its hash is stored. The caller is bound to that id.
//
// Both authenticate as TokenTypeRunner. Routes where the caller acts as a
// particular runner (RevDial host tunnels, runner websockets, sandbox
// register/heartbeat, runner assignment) call authorizeRunnerIdentity.

var (
	// errRunnerTokenInvalid is the only error a client sees for a per-runner
	// token that cannot be used, whatever the cause (unknown, revoked, store
	// failure); the cause is logged.
	errRunnerTokenInvalid       = errors.New("invalid or revoked runner token")
	errSharedRunnerTokenRefused = errors.New("the shared runner token is not accepted here; use a per-runner token (helix runner token create)")
)

// runnerUserFromToken resolves a runner credential. ok is false when token is
// neither the shared runner token nor shaped like a per-runner token, so the
// caller can try other credential types. A per-runner-shaped token that is
// unknown or revoked returns errRunnerTokenInvalid.
func runnerUserFromToken(ctx context.Context, st store.Store, sharedToken, token string) (user *types.User, ok bool, err error) {
	if token == "" {
		return nil, false, nil
	}
	if sharedToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(sharedToken)) == 1 {
		return &types.User{
			ID:        "runner-system",
			Type:      types.OwnerTypeUser,
			Token:     token,
			TokenType: types.TokenTypeRunner,
		}, true, nil
	}
	if !strings.HasPrefix(token, types.RunnerTokenPrefix) {
		return nil, false, nil
	}
	if st == nil {
		return nil, true, errRunnerTokenInvalid
	}
	rt, err := st.GetActiveRunnerTokenByHash(ctx, types.HashRunnerToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return nil, true, errRunnerTokenInvalid
	}
	if err != nil {
		log.Error().Err(err).Msg("failed to load runner token")
		return nil, true, errRunnerTokenInvalid
	}
	return &types.User{
		ID:            "runner-system",
		Type:          types.OwnerTypeUser,
		Token:         token,
		TokenType:     types.TokenTypeRunner,
		RunnerID:      rt.RunnerID,
		RunnerTokenID: rt.ID,
	}, true, nil
}

// authorizeRunnerIdentity decides whether a runner-token caller may act as
// runnerID. Callers that are not runner tokens (admins on admin-or-runner
// routes, session keys on RevDial) are authorized elsewhere and pass.
//
//   - a per-runner token may act only as its own runner id (or that host's
//     hydra-/sandbox- tunnel ids);
//   - the shared token is refused when RUNNER_REQUIRE_PER_HOST_TOKENS is set,
//     and otherwise accepted with a one-time deprecation warning per runner.
func (apiServer *HelixAPIServer) authorizeRunnerIdentity(user *types.User, runnerID string) error {
	if !isRunner(user) {
		return nil
	}
	if user.RunnerID != "" {
		if !types.RunnerIDsMatch(user.RunnerID, runnerID) {
			runnerRefusalLog().Warn().
				Str("bound_runner_id", user.RunnerID).
				Str("requested_runner_id", runnerID).
				Msg("Refused runner request: per-runner token used for another runner")
			return fmt.Errorf("this runner token is bound to runner %q", user.RunnerID)
		}
		return nil
	}
	if apiServer.Cfg != nil && apiServer.Cfg.WebServer.RunnerRequirePerHostTokens {
		runnerRefusalLog().Warn().
			Str("runner_id", runnerID).
			Msg("Refused runner request: shared runner token used while RUNNER_REQUIRE_PER_HOST_TOKENS is set")
		return errSharedRunnerTokenRefused
	}
	apiServer.warnSharedRunnerToken(runnerID)
	return nil
}

// maxSharedRunnerTokenWarnings caps how many runner ids the deprecation
// warning remembers. Runner ids come from the caller, so the set must not
// grow without bound; past the cap no more warnings are logged.
const maxSharedRunnerTokenWarnings = 1024

// warnSharedRunnerToken logs, once per runner id per process, that a host is
// still authenticating with the shared runner token. Only well-formed ids are
// recorded, and at most maxSharedRunnerTokenWarnings of them.
func (apiServer *HelixAPIServer) warnSharedRunnerToken(runnerID string) {
	key := types.CanonicalRunnerID(runnerID)
	if !runnerIDPattern.MatchString(key) {
		return
	}
	if _, seen := apiServer.sharedRunnerTokenWarned.Load(key); seen {
		return
	}
	if apiServer.sharedRunnerTokenWarnCount.Add(1) > maxSharedRunnerTokenWarnings {
		apiServer.sharedRunnerTokenWarnCount.Add(-1)
		return
	}
	if _, seen := apiServer.sharedRunnerTokenWarned.LoadOrStore(key, struct{}{}); seen {
		apiServer.sharedRunnerTokenWarnCount.Add(-1)
		return
	}
	log.Warn().
		Str("runner_id", runnerID).
		Msg("Runner is using the shared RUNNER_TOKEN, which is deprecated for runner hosts: mint a per-runner token with `helix runner token create --runner-id " + key + "` and set it as this host's RUNNER_TOKEN")
}

// authorizeRunnerProfileRead decides whether a runner-token caller may read
// a runner profile (GET /runner/profiles/{id}). A per-runner token reads only
// the profile assigned to its runner; the shared token, which names no
// runner, reads any profile unless RUNNER_REQUIRE_PER_HOST_TOKENS is set.
func (apiServer *HelixAPIServer) authorizeRunnerProfileRead(ctx context.Context, user *types.User, profileID string) error {
	if !isRunner(user) {
		return nil
	}
	if user.RunnerID == "" {
		if apiServer.Cfg != nil && apiServer.Cfg.WebServer.RunnerRequirePerHostTokens {
			return errSharedRunnerTokenRefused
		}
		return nil
	}
	assignment, err := apiServer.Store.GetRunnerAssignment(ctx, user.RunnerID)
	if err != nil || assignment.ProfileID != profileID {
		return errors.New("this runner token may only read the profile assigned to its runner")
	}
	return nil
}

// runnerRefusalSampler limits refused-runner-request warnings to a burst per
// minute: the requests that trigger them are unauthenticated-enough (any
// token holder, any id) to be sent in a loop.
var runnerRefusalSampler = &zerolog.BurstSampler{Burst: 20, Period: time.Minute}

// runnerRefusalLog is the rate-limited logger for refused runner requests.
func runnerRefusalLog() *zerolog.Logger {
	l := log.Logger.Sample(runnerRefusalSampler)
	return &l
}

// canRunnerReadSession decides whether a runner-token caller may read a
// session's credentials or sandbox configuration: only a per-runner token,
// and only for sessions running on its own sandbox host (Session.SandboxID is
// the host id its hydra-<id> tunnel uses). The shared token names no host, so
// it is refused: nothing legitimate reads these with it, since the sandbox's
// settings-sync daemon uses the session's own API key (fac73baae5).
func canRunnerReadSession(user *types.User, session *types.Session) bool {
	return types.RunnerIDsMatch(user.RunnerID, session.SandboxID)
}

// runnerTokenTunnel is a RevDial tunnel registered with a per-runner token.
type runnerTokenTunnel struct {
	tokenID string
	conn    net.Conn
}

// trackRunnerTokenTunnel records which per-runner token registered the tunnel
// now current under key, so revoking that token can cut it. A registration
// with any other credential clears the record.
func (apiServer *HelixAPIServer) trackRunnerTokenTunnel(key string, conn net.Conn, user *types.User) {
	if user.RunnerTokenID == "" {
		apiServer.runnerTokenTunnels.Delete(key)
		return
	}
	apiServer.runnerTokenTunnels.Store(key, runnerTokenTunnel{tokenID: user.RunnerTokenID, conn: conn})
}

// closeRunnerTokenTunnels closes the RevDial tunnels a revoked per-runner
// token registered and that are still current. Dials to them then fail until
// the host registers again, which the revoked token can no longer do.
func (apiServer *HelixAPIServer) closeRunnerTokenTunnels(tokenID string) int {
	if apiServer.connman == nil {
		return 0
	}
	closed := 0
	apiServer.runnerTokenTunnels.Range(func(k, v any) bool {
		key, tunnel := k.(string), v.(runnerTokenTunnel)
		if tunnel.tokenID != tokenID {
			return true
		}
		apiServer.runnerTokenTunnels.CompareAndDelete(key, v)
		if apiServer.connman.CloseIfCurrent(key, tunnel.conn) {
			closed++
		}
		return true
	})
	return closed
}
