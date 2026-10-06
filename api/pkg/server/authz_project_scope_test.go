package server

import (
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

// A session key minted for one spec task must not reach another project, even
// though its owner is a member of both. See enforceKeyProjectScope.
func TestEnforceKeyProjectScope(t *testing.T) {
	tests := []struct {
		name      string
		user      *types.User
		projectID string
		wantErr   bool
	}{
		{
			name:      "unscoped credential reaches any project",
			user:      &types.User{ID: "usr_1"},
			projectID: "prj_a",
		},
		{
			name:      "scoped credential reaches its own project",
			user:      &types.User{ID: "usr_1", ProjectID: "prj_a"},
			projectID: "prj_a",
		},
		{
			name:      "scoped credential is refused another project",
			user:      &types.User{ID: "usr_1", ProjectID: "prj_a"},
			projectID: "prj_b",
			wantErr:   true,
		},
		{
			// helix-org workers and plain chat sessions get a session key with
			// no project; they keep their existing reach.
			name:      "session key without a project is unconstrained",
			user:      &types.User{ID: "usr_1", SessionID: "ses_1"},
			projectID: "prj_b",
		},
		{
			name:      "nil user is not a scope decision",
			user:      nil,
			projectID: "prj_a",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := enforceKeyProjectScope(tc.user, tc.projectID)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected scope error, got nil")
				}
				if !errors.Is(err, errProjectScopedKey) {
					t.Fatalf("expected errProjectScopedKey, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

// The scope check must run before any ownership or membership rule, otherwise
// a project owner's own session key would still reach their other projects.
func TestAuthorizeUserToProjectRejectsForeignProjectForScopedKey(t *testing.T) {
	apiServer := &HelixAPIServer{}

	owner := &types.User{ID: "usr_owner", ProjectID: "prj_a"}
	foreign := &types.Project{ID: "prj_b", UserID: "usr_owner"}

	err := apiServer.authorizeUserToProject(t.Context(), owner, foreign, types.ActionUpdate)
	if err == nil {
		t.Fatal("expected project owner with a scoped key to be refused another project")
	}
	if !errors.Is(err, errProjectScopedKey) {
		t.Fatalf("expected errProjectScopedKey, got %v", err)
	}

	// Same key, its own project: ownership applies as before.
	own := &types.Project{ID: "prj_a", UserID: "usr_owner"}
	if err := apiServer.authorizeUserToProject(t.Context(), owner, own, types.ActionUpdate); err != nil {
		t.Fatalf("expected owner to keep access to its scoped project, got %v", err)
	}
}
