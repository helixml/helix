package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"

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
		return nil, true, fmt.Errorf("error loading runner token: %w", err)
	}
	return &types.User{
		ID:        "runner-system",
		Type:      types.OwnerTypeUser,
		Token:     token,
		TokenType: types.TokenTypeRunner,
		RunnerID:  rt.RunnerID,
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
			log.Warn().
				Str("bound_runner_id", user.RunnerID).
				Str("requested_runner_id", runnerID).
				Msg("Refused runner request: per-runner token used for another runner")
			return fmt.Errorf("this runner token is bound to runner %q", user.RunnerID)
		}
		return nil
	}
	if apiServer.Cfg != nil && apiServer.Cfg.WebServer.RunnerRequirePerHostTokens {
		log.Warn().
			Str("runner_id", runnerID).
			Msg("Refused runner request: shared runner token used while RUNNER_REQUIRE_PER_HOST_TOKENS is set")
		return errSharedRunnerTokenRefused
	}
	apiServer.warnSharedRunnerToken(runnerID)
	return nil
}

// warnSharedRunnerToken logs, once per runner id per process, that a host is
// still authenticating with the shared runner token.
func (apiServer *HelixAPIServer) warnSharedRunnerToken(runnerID string) {
	key := types.CanonicalRunnerID(runnerID)
	if _, seen := apiServer.sharedRunnerTokenWarned.LoadOrStore(key, struct{}{}); seen {
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
