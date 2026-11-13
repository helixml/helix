package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
)

// Fresh builders give an allowed actor its own copy of a resource to change
// or delete, so the shared fixture stays intact for every other row. Each
// returns the placeholder overrides for the copy.

func freshProject(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	name := "fresh-project-" + uuid.NewString()[:8]
	repo := f.createRepo(t, name, f.vars["orgA"])
	return map[string]string{"projA": f.createProject(t, name, repo)}
}

// freshProject2 is the same thing under projA2's placeholder.
func freshProject2(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"projA2": freshProject(t, f)["projA"]}
}

func freshProjectGrant(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	project := freshProject(t, f)["projA"]
	return map[string]string{
		"projA":      project,
		"grantProjA": f.createGrant(t, "/api/v1/projects/"+project+"/access-grants"),
	}
}

func freshSession(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"sessA": f.createSession(t, "fresh-session-"+uuid.NewString()[:8], f.vars["orgA"], f.vars["projA"], f.vars["taskA"])}
}

func freshSession2(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"sessA2": f.createSession(t, "fresh-session-"+uuid.NewString()[:8], f.vars["orgA"], f.vars["projA2"], f.vars["taskA2"])}
}

func freshPersonalSession(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"sessChat": f.createSession(t, "fresh-session-"+uuid.NewString()[:8], "", "", "")}
}

func freshApp(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"appA": f.createApp(t, "fresh-app-"+uuid.NewString()[:8], f.vars["orgA"], types.AgentKindHelix)}
}

func freshAppGrant(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	app := freshApp(t, f)["appA"]
	return map[string]string{
		"appA":      app,
		"grantAppA": f.createGrant(t, "/api/v1/apps/"+app+"/access-grants"),
	}
}

func freshRepo(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"repoA": f.createRepo(t, "fresh-repo-"+uuid.NewString()[:8], f.vars["orgA"])}
}

func freshRepo2(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"repoA2": freshRepo(t, f)["repoA"]}
}

func freshTask(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"taskA": f.createSpecTask(t, "fresh-task-"+uuid.NewString()[:8], f.vars["projA"])}
}

func freshTask2(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"taskA2": f.createSpecTask(t, "fresh-task-"+uuid.NewString()[:8], f.vars["projA2"])}
}

func freshProjectSecret(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"secretProj": f.createSecret(t, "fresh-secret-"+uuid.NewString()[:8], f.vars["projA"])}
}

func freshProjectSecret2(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"secretProj2": f.createSecret(t, "fresh-secret-"+uuid.NewString()[:8], f.vars["projA2"])}
}

func freshPersonalSecret(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"secretPersonal": f.createSecret(t, "fresh-secret-"+uuid.NewString()[:8], "")}
}

func freshKnowledge(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"knowledgeA": f.createKnowledge(t, "fresh-knowledge-"+uuid.NewString()[:8], f.vars["orgA"], f.vars["appA"])}
}

func freshTrigger(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"triggerA": f.createTrigger(t, "fresh-trigger-"+uuid.NewString()[:8], f.vars["appA"], types.Trigger{
		Cron: &types.CronTrigger{Enabled: true, Schedule: "0 0 1 1 *", Input: "authz matrix"},
	})}
}

func freshOrgProvider(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"providerA": f.createProvider(t, "fresh-provider-"+uuid.NewString()[:8], types.OwnerTypeOrg, f.vars["orgA"])}
}

func freshPersonalProvider(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"providerPersonal": f.createProvider(t, "fresh-provider-"+uuid.NewString()[:8], types.OwnerTypeUser, f.users[aOwner].ID)}
}

func freshArtifact(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"artifactA": f.createArtifact(t, "fresh-artifact-"+uuid.NewString()[:8], f.vars["projA"])}
}

func freshArtifact2(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"artifactA2": f.createArtifact(t, "fresh-artifact-"+uuid.NewString()[:8], f.vars["projA2"])}
}

// freshOrg gives aOwner a throwaway organization (deleted with the run).
func freshOrg(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	org, err := f.owner.CreateOrganization(f.setupCtx(t), &types.Organization{Name: "authz-fresh-" + uuid.NewString()[:8]})
	if err != nil {
		t.Fatalf("create fresh org: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), authzSetupTimeout)
		defer cancel()
		_ = f.owner.DeleteOrganization(ctx, org.ID)
	})
	return map[string]string{"orgA": org.ID}
}

func freshOrgWithMember(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	overrides := freshOrg(t, f)
	f.api(t, aOwner, http.MethodPost, "/api/v1/organizations/"+overrides["orgA"]+"/members", &types.AddOrganizationMemberRequest{
		UserReference: f.users[aMember].ID,
		Role:          types.OrganizationRoleMember,
	}, nil)
	return overrides
}

// freshArchivedTask is a task ready to delete (delete refuses live tasks).
func freshArchivedTask(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"taskA": f.archivedTask(t, f.vars["projA"])}
}

func freshArchivedTask2(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"taskA2": f.archivedTask(t, f.vars["projA2"])}
}

func (f *authzFixture) archivedTask(t *testing.T, projectID string) string {
	t.Helper()
	id := f.createSpecTask(t, "fresh-task-"+uuid.NewString()[:8], projectID)
	task, err := f.db.GetSpecTask(f.ctx, id)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.Archived = true
	if err := f.db.UpdateSpecTask(f.ctx, task); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	return id
}
