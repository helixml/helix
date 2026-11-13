package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/attestation"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Identity attestations let an external verifier learn which spec task,
// session or sandbox a caller is without the caller handing over its Helix
// key. The caller mints a token with its session-scoped key; the verifier
// redeems it once with an org key (or as a user) that may read the project.
//
// Minting is refused for:
//   - any credential that is not a session-scoped key of the plain API type
//     (user, org, project and app keys, browser sessions, embed and bot
//     instance keys), so only code inside a Helix desktop or sandbox can
//     attest, and it can attest only the identity its key is bound to;
//   - keys outside a project: verification authorizes the verifier against
//     the project, so an attestation without one could not be checked;
//   - keys whose session, spec task or sandbox is gone or no longer matches
//     the key, and archived spec tasks. A done (but not archived) task still
//     attests: its desktop may legitimately still be running.

const attestationBodyLimit = 8 << 10

var errAttestationKey = errors.New("identity attestations are minted only with the session or sandbox key of a desktop in a project")

// attestableKey reports whether the caller holds a key that may mint: the
// plain API key Helix injects into a project's desktop or sandbox.
func attestableKey(user *types.User) bool {
	return user != nil && user.TokenType == types.TokenTypeAPIKey &&
		(user.APIKeyType == types.APIkeytypeAPI || user.APIKeyType == types.APIkeytypeNone) &&
		user.AppID == "" && user.SessionID != "" && user.ProjectID != ""
}

// attestationVerifierKey reports whether the caller may verify: a person
// (browser session, OIDC, personal key) or an org-scoped key. Keys bound to a
// project, session or app are refused, so a lane cannot probe attestations.
func attestationVerifierKey(user *types.User) bool {
	if user == nil || user.ID == "" || isAgentScopedKey(user) || user.AppID != "" {
		return false
	}
	switch user.TokenType {
	case types.TokenTypeAPIKey:
		return user.APIKeyType == types.APIkeytypeAPI || user.APIKeyType == types.APIkeytypeNone
	case types.TokenTypeSession, types.TokenTypeOIDC, types.TokenTypeNone:
		return true
	default:
		return false
	}
}

// createIdentityAttestation godoc
// @Summary Mint an identity attestation
// @Description Returns a short-lived token stating which spec task, session or sandbox the caller's key belongs to, for one audience and nonce. Only the session or sandbox key of a desktop in a project may call it.
// @Tags    identity
// @Accept  json
// @Produce json
// @Param   request body types.IdentityAttestationRequest true "Audience and nonce"
// @Success 200 {object} types.IdentityAttestationResponse
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Router /api/v1/identity/attestations [post]
// @Security BearerAuth
func (s *HelixAPIServer) createIdentityAttestation(_ http.ResponseWriter, r *http.Request) (*types.IdentityAttestationResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if !attestableKey(user) {
		return nil, system.NewHTTPError403(errAttestationKey.Error())
	}

	var req types.IdentityAttestationRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, attestationBodyLimit)).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}
	if err := attestation.ValidateAudience(req.Audience); err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}
	if err := attestation.ValidateNonce(req.Nonce); err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	subject, err := s.loadAttestationSubject(ctx, user)
	if err != nil {
		log.Warn().Err(err).Str("user_id", user.ID).Str("session_id", user.SessionID).Msg("identity attestation refused")
		return nil, system.NewHTTPError403(errAttestationKey.Error())
	}
	project, err := s.Store.GetProject(ctx, subject.ProjectID)
	if err != nil || project.OrganizationID != user.OrganizationID {
		return nil, system.NewHTTPError403(errAttestationKey.Error())
	}
	// The key's owner must still have the project: a member removed from the
	// org keeps a running desktop but loses the right to speak for it.
	if err := s.authorizeUserToProject(ctx, user, project, types.ActionGet); err != nil {
		return nil, system.NewHTTPError403(errAttestationKey.Error())
	}
	subject.OrganizationID = project.OrganizationID

	token, claims, err := s.attestations.Mint(*subject, req.Audience, req.Nonce)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	if err := s.Store.CreateProjectAuditLog(ctx, attestationAuditEntry(types.AuditEventIdentityAttestationMinted, user, claims)); err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return &types.IdentityAttestationResponse{Token: token, ExpiresAt: claims.ExpiresAt}, nil
}

// loadAttestationSubject loads the rows the caller's key is bound to and
// returns the identity they establish, refusing a key that no longer matches
// them.
func (s *HelixAPIServer) loadAttestationSubject(ctx context.Context, user *types.User) (*attestation.Subject, error) {
	if system.IsSandboxID(user.SessionID) {
		sandbox, err := s.Store.GetSandbox(ctx, user.SessionID)
		if err != nil {
			return nil, fmt.Errorf("load sandbox: %w", err)
		}
		if sandbox.Owner != user.ID || sandbox.OrganizationID != user.OrganizationID || sandbox.ProjectID != user.ProjectID {
			return nil, errors.New("sandbox does not match the key")
		}
		return &attestation.Subject{SandboxID: sandbox.ID, ProjectID: sandbox.ProjectID, Owner: user.ID}, nil
	}

	session, err := s.Store.GetSession(ctx, user.SessionID)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	sessionProject := session.ProjectID
	if sessionProject == "" {
		sessionProject = session.Metadata.ProjectID
	}
	if session.Owner != user.ID || (session.OrganizationID != "" && session.OrganizationID != user.OrganizationID) ||
		session.Metadata.SpecTaskID != user.SpecTaskID ||
		(sessionProject != "" && sessionProject != user.ProjectID) {
		return nil, errors.New("session does not match the key")
	}
	subject := &attestation.Subject{SessionID: session.ID, ProjectID: user.ProjectID, Owner: user.ID}
	if user.SpecTaskID == "" {
		if sessionProject == "" {
			return nil, errors.New("session is in no project")
		}
		return subject, nil
	}

	task, err := s.Store.GetSpecTask(ctx, user.SpecTaskID)
	if err != nil {
		return nil, fmt.Errorf("load spec task: %w", err)
	}
	if task.ProjectID != user.ProjectID || (task.OrganizationID != "" && task.OrganizationID != user.OrganizationID) {
		return nil, errors.New("spec task does not match the key")
	}
	if task.Archived {
		return nil, errors.New("spec task is archived")
	}
	subject.SpecTaskID = task.ID
	return subject, nil
}

// verifyIdentityAttestation godoc
// @Summary Verify an identity attestation
// @Description Redeems an attestation once. The caller must be a user or an org-scoped API key that may read the attested project; session, project and app keys are refused. Every invalid, expired, foreign or replayed token gets the same 404.
// @Tags    identity
// @Accept  json
// @Produce json
// @Param   request body types.IdentityAttestationVerifyRequest true "Token, audience and nonce"
// @Success 200 {object} types.IdentityAttestationClaims
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} types.IdentityAttestationError
// @Router /api/v1/identity/attestations/verify [post]
// @Security BearerAuth
func (s *HelixAPIServer) verifyIdentityAttestation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if !attestationVerifierKey(user) {
		writeErrResponse(w, errors.New("identity attestations are verified with an org API key or as a user"), http.StatusForbidden)
		return
	}
	notValid := func() {
		writeResponse(w, &types.IdentityAttestationError{Error: attestation.ErrInvalid.Error()}, http.StatusNotFound)
	}

	var req types.IdentityAttestationVerifyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, attestationBodyLimit)).Decode(&req); err != nil {
		writeErrResponse(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}
	claims, err := s.loadVerifiedAttestation(req)
	if err != nil {
		notValid()
		return
	}
	if user.OrganizationID != "" && claims.OrganizationID != user.OrganizationID {
		notValid()
		return
	}
	project, err := s.Store.GetProject(ctx, claims.ProjectID)
	if err != nil || project.OrganizationID != claims.OrganizationID {
		notValid()
		return
	}
	if err := s.authorizeUserToProject(ctx, user, project, types.ActionGet); err != nil {
		notValid()
		return
	}

	redeemed, err := s.Store.RedeemIdentityAttestation(ctx, claims.JTI, claims.ExpiresAt)
	if err != nil {
		writeErrResponse(w, err, http.StatusInternalServerError)
		return
	}
	if !redeemed {
		notValid()
		return
	}
	if err := s.Store.CreateProjectAuditLog(ctx, attestationAuditEntry(types.AuditEventIdentityAttestationVerified, user, claims)); err != nil {
		writeErrResponse(w, err, http.StatusInternalServerError)
		return
	}
	writeResponse(w, claims, http.StatusOK)
}

// loadVerifiedAttestation checks the token's signature, expiry, audience and
// nonce. It touches no stored state.
func (s *HelixAPIServer) loadVerifiedAttestation(req types.IdentityAttestationVerifyRequest) (*types.IdentityAttestationClaims, error) {
	return s.attestations.Verify(req.Token, req.Audience, req.Nonce)
}

func attestationAuditEntry(event types.AuditEventType, caller *types.User, claims *types.IdentityAttestationClaims) *types.ProjectAuditLog {
	return &types.ProjectAuditLog{
		ID:         system.GenerateUUID(),
		ProjectID:  claims.ProjectID,
		SpecTaskID: claims.SpecTaskID,
		UserID:     caller.ID,
		UserEmail:  caller.Email,
		EventType:  event,
		Metadata: types.AuditMetadata{
			SessionID:           claims.SessionID,
			SandboxID:           claims.SandboxID,
			AttestationJTI:      claims.JTI,
			AttestationAudience: claims.Audience,
		},
		CreatedAt: time.Now(),
	}
}
