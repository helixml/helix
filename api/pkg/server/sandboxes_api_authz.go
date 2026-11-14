package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Authorization for the org Sandboxes API (/organizations/{org}/sandboxes).
//
// Org membership alone is not enough: the table also holds the meter rows of
// spec-task, chat and org-bot desktops (types.Sandbox.SessionBacked), whose
// hydra ops id is the session id, so the files, commands and terminal routes
// act inside that session's desktop. A row is reached on the terms of what it
// belongs to:
//
//   - a session-backed row: authorizeUserToSession on its session, which
//     applies project access and the caller's key scope;
//   - a row in a project: authorizeUserToProject on that project;
//   - a row in neither: org membership (the Sandboxes API's own sandboxes are
//     shared across the organization).
//
// Reading the row, its billing and a screenshot need ActionGet; anything that
// touches the container's contents or lifecycle (files, commands and their
// logs, terminal, update, delete) needs ActionUpdate.
//
// A task-confined key gets none of the API: its own desktop is reached through
// the session routes, and a lane that could reach another lane's desktop
// could act as it.

// errTaskConfinedKeySandboxes refuses the org Sandboxes API to a
// task-confined key.
var errTaskConfinedKeySandboxes = errors.New("a task-confined key cannot use the sandboxes API")

func denySandboxesToConfinedKey(user *types.User) error {
	if user != nil && user.TaskConfined {
		return errTaskConfinedKeySandboxes
	}
	return nil
}

// enforceKeySandboxScope confines a key bound to a project or a session to
// the sandboxes of that project or session. Callers whose credential carries
// neither are unaffected.
func enforceKeySandboxScope(user *types.User, sb *types.Sandbox) error {
	if err := denySandboxesToConfinedKey(user); err != nil {
		return err
	}
	if user == nil {
		return nil
	}
	if user.ProjectID != "" {
		if sb.ProjectID != user.ProjectID {
			return fmt.Errorf("%w: key is scoped to project %s, sandbox %s is outside it", errProjectScopedKey, user.ProjectID, sb.ID)
		}
		return nil
	}
	if user.SessionID != "" && user.SessionID != sb.ID && user.SessionID != sb.SessionID {
		return fmt.Errorf("%w: key is scoped to session %s, request targets sandbox %s", errSessionScopedKey, user.SessionID, sb.ID)
	}
	return nil
}

// authorizeUserToSandbox decides whether user may act on sb. The caller must
// already have checked org membership of sb's organization.
func (s *HelixAPIServer) authorizeUserToSandbox(ctx context.Context, user *types.User, membership *types.OrganizationMembership, sb *types.Sandbox, action types.Action) error {
	if err := enforceKeySandboxScope(user, sb); err != nil {
		return err
	}
	if sb.SessionBacked() {
		session, err := s.Store.GetSession(ctx, sb.SessionID)
		if errors.Is(err, store.ErrNotFound) {
			// The session is gone but its meter row is not: only an org
			// owner may still see or clear it.
			if membership.Role == types.OrganizationRoleOwner {
				return nil
			}
			return err
		}
		if err != nil {
			return fmt.Errorf("get session %s: %w", sb.SessionID, err)
		}
		return s.authorizeUserToSession(ctx, user, session, action)
	}
	if sb.ProjectID != "" {
		return s.authorizeUserToProjectByID(ctx, user, sb.ProjectID, action)
	}
	return nil
}

// loadAuthorizedSandbox fetches the sandbox named by the URL and authorizes
// the caller for action on it (authorizeUserToSandbox). The URL's org must
// be the sandbox's org, so cross-org id-guessing is blocked. Every refusal
// answers 404 "sandbox not found", as for an unknown id, so another task's
// or project's desktop cannot be discovered by id. Writes the HTTP error and
// returns nil when access is denied.
func (s *HelixAPIServer) loadAuthorizedSandbox(rw http.ResponseWriter, r *http.Request, action types.Action) *types.Sandbox {
	user := getRequestUser(r)
	if err := denySandboxesToConfinedKey(user); err != nil {
		http.Error(rw, "sandbox not found", http.StatusNotFound)
		return nil
	}
	// Resolve the URL's {org_id} (an org id OR a slug) to the canonical id
	// stored on the row, so the equality check below works for either form.
	urlOrgID := s.resolveURLOrgID(rw, r)
	if urlOrgID == "" {
		return nil
	}
	sb, err := s.sandboxController.Get(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "sandbox not found", http.StatusNotFound)
		} else {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}
	if urlOrgID != sb.OrganizationID {
		http.Error(rw, "sandbox not found", http.StatusNotFound)
		return nil
	}
	membership, err := s.authorizeOrgMember(r.Context(), user, sb.OrganizationID)
	if err != nil {
		if isOutsideTenant(err) {
			http.Error(rw, "sandbox not found", http.StatusNotFound)
		} else {
			log.Error().Err(err).Msg("error checking org membership for sandbox")
			http.Error(rw, "could not check org membership", http.StatusInternalServerError)
		}
		return nil
	}
	if err := s.authorizeUserToSandbox(r.Context(), user, membership, sb, action); err != nil {
		log.Warn().Err(err).
			Str("user_id", user.ID).
			Str("sandbox_id", sb.ID).
			Str("action", string(action)).
			Msg("sandbox access denied")
		http.Error(rw, "sandbox not found", http.StatusNotFound)
		return nil
	}
	return sb
}

// sandboxVisibility is the subset of the org's sandboxes user may list:
// nil (all of them) for an org owner holding an unscoped credential,
// otherwise rows in the projects they can read, their own session desktops
// and, for an unscoped credential, the org's project-less sandboxes. It
// mirrors authorizeUserToSandbox with ActionGet.
func (s *HelixAPIServer) sandboxVisibility(ctx context.Context, user *types.User, orgID string, membership *types.OrganizationMembership) (*store.SandboxVisibility, error) {
	scopedKey := isAgentScopedKey(user)
	if !scopedKey && membership.Role == types.OrganizationRoleOwner {
		return nil, nil
	}
	projects, err := s.visibleOrganizationProjects(ctx, user, orgID, membership, types.ActionGet, false)
	if err != nil {
		return nil, err
	}
	visibility := &store.SandboxVisibility{}
	for _, project := range projects {
		visibility.ProjectIDs = append(visibility.ProjectIDs, project.ID)
	}
	if !scopedKey {
		visibility.SessionOwner = user.ID
		visibility.IncludeUnscoped = true
	}
	return visibility, nil
}

// authorizeSandboxCreateProject authorizes the project a new sandbox is put
// in: the caller must be able to update that project, and it must belong to
// the sandbox's organization. A project-scoped key creates in its own
// project, so the sandbox stays reachable by the key that made it. Writes
// the HTTP error and returns false when refused.
func (s *HelixAPIServer) authorizeSandboxCreateProject(rw http.ResponseWriter, r *http.Request, user *types.User, orgID string, req *types.CreateSandboxRequest) bool {
	if req.ProjectID == "" {
		req.ProjectID = user.ProjectID
	}
	if req.ProjectID == "" {
		if user.SessionID != "" {
			// A key bound to a session reaches only that session's sandbox.
			http.Error(rw, errSessionScopedKey.Error(), http.StatusForbidden)
			return false
		}
		return true
	}
	project, err := s.Store.GetProject(r.Context(), req.ProjectID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return false
	}
	if errors.Is(err, store.ErrNotFound) || project.OrganizationID != orgID {
		http.Error(rw, "project not found", http.StatusNotFound)
		return false
	}
	if err := s.authorizeUserToProject(r.Context(), user, project, types.ActionUpdate); err != nil {
		if isOutsideTenant(err) {
			http.Error(rw, "project not found", http.StatusNotFound)
		} else {
			http.Error(rw, err.Error(), http.StatusForbidden)
		}
		return false
	}
	return true
}
