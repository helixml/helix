package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// authzFixture is the shared world every matrix row runs against.
//
// Placeholders available to rows (vars):
//
//	orgA, orgB, orgAName          organization ids / slug
//	aOwnerID, aMemberID, bOwnerID user ids
//	appA        helix agent in org A (aMember has a read grant)
//	orgAgentA   org agent in org A, default agent of both projects
//	appPersonal aOwner's agent outside any organization
//	repoA       internal repo in org A, attached to projA
//	repoA2      internal repo in org A, attached to projA2
//	projA       project in org A (aMember has a read grant; projKey's project)
//	projA2      second project in org A owned by aOwner
//	sessA       session in projA (projKey's session)
//	sessA2      session in projA2
//	sessChat    aOwner's personal session (sessKey's session)
//	sessOther   another personal session of aOwner
//	taskA       spec task in projA with specs in review; taskA2 in projA2
//	taskBacklog a backlog spec task in projA (nothing to approve yet)
//	secretProj  project secret on projA; secretProj2 on projA2
//	secretPersonal aOwner's personal secret
//	knowledgeA  knowledge on appA
//	triggerA    cron trigger on appA; webhookTriggerA Azure DevOps trigger
//	providerA   org A provider endpoint; providerPersonal aOwner's
//	artifactA   artifact in projA; artifactA2 in projA2
//	grantProjA  aMember's read grant on projA
//	grantAppA   aMember's read grant on appA
type authzFixture struct {
	ctx  context.Context
	db   *store.PostgresStore
	keys map[actor]string
	vars map[string]string
	// markers maps a resource name to a unique string stored in its name or
	// content; a refusal body must never contain one.
	markers map[string]string
	users   map[actor]*types.User
	owner   *client.HelixClient
}

func newAuthzFixture(t *testing.T) *authzFixture {
	t.Helper()
	requireAuthzServer(t)

	db, err := getStoreClient()
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	authenticator, err := auth.NewHelixAuthenticator(&config.ServerConfig{}, db, "test-secret", nil)
	if err != nil {
		t.Fatalf("authenticator: %v", err)
	}

	f := &authzFixture{
		ctx:     context.Background(),
		db:      db,
		keys:    map[actor]string{},
		vars:    map[string]string{},
		markers: map[string]string{},
		users:   map[actor]*types.User{},
	}
	run := uuid.NewString()[:8]

	for _, who := range []actor{aOwner, aMember, bOwner, outsider, waitlisted, admin} {
		email := fmt.Sprintf("authz-%s-%s@example.com", who, uuid.NewString())
		user, key, err := createUser(t, db, authenticator, email)
		if err != nil {
			t.Fatalf("create %s: %v", who, err)
		}
		user.AlphaFeatures = []string{"helix-org"}
		switch who {
		case waitlisted:
			user.Waitlisted = true
		case admin:
			user.Admin = true
		}
		user, err = db.UpdateUser(f.ctx, user)
		if err != nil {
			t.Fatalf("update %s: %v", who, err)
		}
		f.users[who] = user
		f.keys[who] = key
	}
	f.vars["aOwnerID"] = f.users[aOwner].ID
	f.vars["aMemberID"] = f.users[aMember].ID
	f.vars["bOwnerID"] = f.users[bOwner].ID

	f.owner = f.clientFor(t, aOwner)
	bClient := f.clientFor(t, bOwner)

	orgA, err := f.owner.CreateOrganization(f.ctx, &types.Organization{Name: "authz-a-" + run})
	if err != nil {
		t.Fatalf("create org A: %v", err)
	}
	orgB, err := bClient.CreateOrganization(f.ctx, &types.Organization{Name: "authz-b-" + run})
	if err != nil {
		t.Fatalf("create org B: %v", err)
	}
	t.Cleanup(func() {
		_ = f.owner.DeleteOrganization(context.Background(), orgA.ID)
		_ = bClient.DeleteOrganization(context.Background(), orgB.ID)
	})
	f.vars["orgA"] = orgA.ID
	f.vars["orgAName"] = orgA.Name
	f.vars["orgB"] = orgB.ID

	if _, err := f.owner.AddOrganizationMember(f.ctx, orgA.ID, &types.AddOrganizationMemberRequest{
		UserReference: f.users[aMember].ID,
		Role:          types.OrganizationRoleMember,
	}); err != nil {
		t.Fatalf("add member: %v", err)
	}

	// Agents.
	f.vars["appA"] = f.createApp(t, "appA", orgA.ID, types.AgentKindHelix)
	f.vars["orgAgentA"] = f.createApp(t, "orgAgentA", orgA.ID, types.AgentKindOrg)
	f.vars["appPersonal"] = f.createApp(t, "appPersonal", "", types.AgentKindHelix)

	// Repositories and projects.
	f.vars["repoA"] = f.createRepo(t, "repoA", orgA.ID)
	f.vars["repoA2"] = f.createRepo(t, "repoA2", orgA.ID)
	f.vars["projA"] = f.createProject(t, "projA", f.vars["repoA"])
	f.vars["projA2"] = f.createProject(t, "projA2", f.vars["repoA2"])

	// Spec tasks and sessions (seeded in the store: creating them through
	// the API starts agents, which needs an LLM and a sandbox).
	f.vars["taskA"] = f.createSpecTask(t, "taskA", f.vars["projA"])
	f.vars["taskBacklog"] = f.createSpecTask(t, "taskBacklog", f.vars["projA"])
	f.setTaskStatus(t, f.vars["taskA"], types.TaskStatusSpecReview)
	f.vars["taskA2"] = f.createSpecTask(t, "taskA2", f.vars["projA2"])
	f.vars["sessA"] = f.createSession(t, "sessA", orgA.ID, f.vars["projA"], f.vars["taskA"])
	f.vars["sessA2"] = f.createSession(t, "sessA2", orgA.ID, f.vars["projA2"], f.vars["taskA2"])
	f.vars["sessChat"] = f.createSession(t, "sessChat", "", "", "")
	f.vars["sessOther"] = f.createSession(t, "sessOther", "", "", "")

	// Scoped credentials.
	f.keys[aOwnerOrgKey] = f.createOrgKey(t)
	f.keys[projKey] = f.createScopedKey(t, &types.ApiKey{
		Name:           "Session key - authz matrix (task)",
		OrganizationID: orgA.ID,
		SessionID:      f.vars["sessA"],
		ProjectID:      f.vars["projA"],
		SpecTaskID:     f.vars["taskA"],
	})
	f.keys[sessKey] = f.createScopedKey(t, &types.ApiKey{
		Name:      "Session key - authz matrix (chat)",
		SessionID: f.vars["sessChat"],
	})
	f.keys[appKey] = f.createAppKey(t, f.vars["appA"])
	// A helix-org worker's key: its session names the bot it runs as.
	f.vars["sessWorker"] = f.createWorkerSession(t, orgA.ID, "chief-of-staff")
	f.keys[workerKey] = f.createScopedKey(t, &types.ApiKey{
		Name:           "Session key - authz matrix (worker)",
		OrganizationID: orgA.ID,
		SessionID:      f.vars["sessWorker"],
	})

	// Grants: aMember may read projA and appA, nothing more.
	f.vars["grantProjA"] = f.createGrant(t, "/api/v1/projects/"+f.vars["projA"]+"/access-grants")
	f.vars["grantAppA"] = f.createGrant(t, "/api/v1/apps/"+f.vars["appA"]+"/access-grants")

	// Secrets.
	f.vars["secretProj"] = f.createSecret(t, "secretProj", f.vars["projA"])
	f.vars["secretProj2"] = f.createSecret(t, "secretProj2", f.vars["projA2"])
	f.vars["secretPersonal"] = f.createSecret(t, "secretPersonal", "")

	// Knowledge, triggers, providers, artifacts.
	f.vars["knowledgeA"] = f.createKnowledge(t, "knowledgeA", orgA.ID, f.vars["appA"])
	f.vars["triggerA"] = f.createTrigger(t, "triggerA", f.vars["appA"], types.Trigger{
		Cron: &types.CronTrigger{Enabled: true, Schedule: "0 0 1 1 *", Input: "authz matrix"},
	})
	f.vars["webhookTriggerA"] = f.createTrigger(t, "webhookTriggerA", f.vars["appA"], types.Trigger{
		AzureDevOps: &types.AzureDevOpsTrigger{Enabled: true},
	})
	f.vars["webhookSecretA"] = f.webhookSecret(t, f.vars["webhookTriggerA"])
	f.vars["providerA"] = f.createProvider(t, "providerA", types.OwnerTypeOrg, orgA.ID)
	f.vars["providerAName"] = f.markers["providerA"]
	f.vars["providerPersonal"] = f.createProvider(t, "providerPersonal", types.OwnerTypeUser, f.users[aOwner].ID)
	f.vars["artifactA"] = f.createArtifact(t, "artifactA", f.vars["projA"])
	f.vars["artifactA2"] = f.createArtifact(t, "artifactA2", f.vars["projA2"])
	f.uploadAppFile(t, f.vars["appA"])

	return f
}

func (f *authzFixture) clientFor(t *testing.T, who actor) *client.HelixClient {
	t.Helper()
	c, err := getAPIClient(f.keys[who])
	if err != nil {
		t.Fatalf("client for %s: %v", who, err)
	}
	return c
}

// mark registers a unique marker for a resource and returns it, to be used
// in the resource's name.
func (f *authzFixture) mark(name string) string {
	marker := "authzmark-" + strings.ToLower(name) + "-" + uuid.NewString()[:12]
	f.markers[name] = marker
	return marker
}

// api performs a request as aOwner (or another actor) and decodes a 2xx
// response into out. Any other status fails the fixture build.
func (f *authzFixture) api(t *testing.T, who actor, method, path string, body any, out any) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
	}
	resp := f.do(t, who, method, path, raw, nil)
	if resp.status < 200 || resp.status > 299 {
		t.Fatalf("fixture %s %s as %s: %d %s", method, path, who, resp.status, truncate(resp.body, 500))
	}
	if out != nil {
		if err := json.Unmarshal([]byte(resp.body), out); err != nil {
			t.Fatalf("fixture %s %s: decode %q: %v", method, path, truncate(resp.body, 300), err)
		}
	}
}

func (f *authzFixture) createApp(t *testing.T, name, orgID, kind string) string {
	t.Helper()
	app, err := f.owner.CreateApp(f.ctx, &types.App{
		OrganizationID: orgID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        f.mark(name),
				Description: "authz matrix agent",
				Assistants: []types.AssistantConfig{{
					Name:  "assistant",
					Model: "openai/gpt-oss-20b",
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("create app %s: %v", name, err)
	}
	if app.AgentKind != kind {
		app.AgentKind = kind
		if app, err = f.db.UpdateApp(f.ctx, app); err != nil {
			t.Fatalf("set agent kind %s: %v", name, err)
		}
	}
	return app.ID
}

func (f *authzFixture) createRepo(t *testing.T, name, orgID string) string {
	t.Helper()
	var repo types.GitRepository
	f.api(t, aOwner, http.MethodPost, "/api/v1/git/repositories", &types.GitRepositoryCreateRequest{
		Name:           strings.ToLower(name) + "-" + uuid.NewString()[:8],
		Description:    f.mark(name),
		RepoType:       types.GitRepositoryTypeCode,
		OwnerID:        f.users[aOwner].ID,
		OrganizationID: orgID,
		DefaultBranch:  "main",
		InitialFiles:   map[string]string{"README.md": "# " + f.markers[name] + "\n"},
		Metadata:       map[string]interface{}{},
	}, &repo)
	return repo.ID
}

func (f *authzFixture) createProject(t *testing.T, name, repoID string) string {
	t.Helper()
	var project types.Project
	f.api(t, aOwner, http.MethodPost, "/api/v1/projects", &types.ProjectCreateRequest{
		OrganizationID:    f.vars["orgA"],
		Name:              f.mark(name),
		Description:       "authz matrix project",
		DefaultRepoID:     repoID,
		DefaultHelixAppID: f.vars["orgAgentA"],
	}, &project)
	return project.ID
}

func (f *authzFixture) createSpecTask(t *testing.T, name, projectID string) string {
	t.Helper()
	task := &types.SpecTask{
		ID:             system.GenerateSpecTaskID(),
		ProjectID:      projectID,
		UserID:         f.users[aOwner].ID,
		OrganizationID: f.vars["orgA"],
		Name:           f.mark(name),
		Description:    "authz matrix task",
		Type:           "feature",
		// Specs present so the spec read routes have something to return.
		RequirementsSpec:   "authz matrix requirements",
		TechnicalDesign:    "authz matrix design",
		ImplementationPlan: "authz matrix plan",
		Status:             types.TaskStatusBacklog,
		OriginalPrompt:     "authz matrix",
	}
	if err := f.db.CreateSpecTask(f.ctx, task); err != nil {
		t.Fatalf("create spec task %s: %v", name, err)
	}
	return task.ID
}

func (f *authzFixture) createSession(t *testing.T, name, orgID, projectID, taskID string) string {
	t.Helper()
	session, err := f.db.CreateSession(f.ctx, types.Session{
		ID:             system.GenerateSessionID(),
		Name:           f.mark(name),
		Owner:          f.users[aOwner].ID,
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: orgID,
		ProjectID:      projectID,
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		Metadata: types.SessionMetadata{
			ProjectID:  projectID,
			SpecTaskID: taskID,
		},
	})
	if err != nil {
		t.Fatalf("create session %s: %v", name, err)
	}
	t.Cleanup(func() { _, _ = f.db.DeleteSession(context.Background(), session.ID) })
	return session.ID
}

// createScopedKey stores a key exactly as the server mints it for a sandbox
// or worker (services.GetOrCreateSessionAPIKey): aOwner's key, carrying the
// scope in template.
func (f *authzFixture) createScopedKey(t *testing.T, template *types.ApiKey) string {
	t.Helper()
	key, err := system.GenerateAPIKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template.Key = key
	template.Owner = f.users[aOwner].ID
	template.OwnerType = types.OwnerTypeUser
	template.Type = types.APIkeytypeAPI
	if _, err := f.db.CreateAPIKey(f.ctx, template); err != nil {
		t.Fatalf("create scoped key: %v", err)
	}
	return key
}

func (f *authzFixture) createOrgKey(t *testing.T) string {
	t.Helper()
	var key types.ApiKey
	f.api(t, aOwner, http.MethodPost, "/api/v1/organizations/"+f.vars["orgA"]+"/api_keys",
		map[string]string{"name": "authz matrix org key"}, &key)
	if key.Key == "" {
		t.Fatalf("org key response carried no key")
	}
	return key.Key
}

func (f *authzFixture) createAppKey(t *testing.T, appID string) string {
	t.Helper()
	key, err := f.owner.CreateAppAPIKey(f.ctx, appID, "authz matrix app key")
	if err != nil {
		t.Fatalf("create app key: %v", err)
	}
	return key
}

func (f *authzFixture) createGrant(t *testing.T, path string) string {
	t.Helper()
	var grant types.AccessGrant
	f.api(t, aOwner, http.MethodPost, path, &types.CreateAccessGrantRequest{
		UserReference: f.users[aMember].ID,
		Roles:         []string{"read"},
	}, &grant)
	return grant.ID
}

func (f *authzFixture) createSecret(t *testing.T, name, projectID string) string {
	t.Helper()
	var secret types.Secret
	f.api(t, aOwner, http.MethodPost, "/api/v1/secrets", &types.CreateSecretRequest{
		Name:      strings.ToUpper(strings.ReplaceAll(f.mark(name), "-", "_")),
		Value:     "authz-matrix-secret-value-" + uuid.NewString(),
		ProjectID: projectID,
	}, &secret)
	// The stored name is upper-cased; track that form as the marker.
	f.markers[name] = secret.Name
	return secret.ID
}

func (f *authzFixture) createKnowledge(t *testing.T, name, orgID, appID string) string {
	t.Helper()
	knowledgeText := "authz matrix knowledge"
	knowledge, err := f.db.CreateKnowledge(f.ctx, &types.Knowledge{
		ID:             system.GenerateKnowledgeID(),
		Name:           f.mark(name),
		Owner:          f.users[aOwner].ID,
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: orgID,
		AppID:          appID,
		State:          types.KnowledgeStateReady,
		Source: types.KnowledgeSource{
			Text: &knowledgeText,
		},
	})
	if err != nil {
		t.Fatalf("create knowledge: %v", err)
	}
	return knowledge.ID
}

func (f *authzFixture) createTrigger(t *testing.T, name, appID string, trigger types.Trigger) string {
	t.Helper()
	var created types.TriggerConfiguration
	f.api(t, aOwner, http.MethodPost, "/api/v1/triggers", &types.TriggerConfiguration{
		AppID:   appID,
		Name:    f.mark(name),
		Enabled: true,
		Trigger: trigger,
	}, &created)
	return created.ID
}

// createProvider seeds an endpoint in the store: the API only accepts them
// when provider management is switched on, which is not what this suite is
// about.
func (f *authzFixture) createProvider(t *testing.T, name string, ownerType types.OwnerType, owner string) string {
	t.Helper()
	endpointType := types.ProviderEndpointTypeUser
	if ownerType == types.OwnerTypeOrg {
		endpointType = types.ProviderEndpointTypeOrg
	}
	endpoint, err := f.db.CreateProviderEndpoint(f.ctx, &types.ProviderEndpoint{
		ID:           system.GenerateProviderEndpointID(),
		Name:         f.mark(name),
		Description:  "authz matrix provider",
		EndpointType: endpointType,
		Owner:        owner,
		OwnerType:    ownerType,
		BaseURL:      "https://provider.authz-matrix.invalid/v1",
		APIKey:       "authz-matrix-provider-key-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("create provider %s: %v", name, err)
	}
	return endpoint.ID
}

func (f *authzFixture) createArtifact(t *testing.T, name, projectID string) string {
	t.Helper()
	marker := f.mark(name)
	visibility := types.ArtifactVisibilityProject
	artifact, err := f.owner.CreateArtifact(f.ctx, projectID, &client.ArtifactUploadRequest{
		Name:       &marker,
		Visibility: &visibility,
		Content: &client.ArtifactContent{
			Filename: "index.html",
			Reader:   strings.NewReader("<h1>" + marker + "</h1>"),
		},
	})
	if err != nil {
		t.Fatalf("create artifact %s: %v", name, err)
	}
	return artifact.ID
}

func (f *authzFixture) webhookSecret(t *testing.T, triggerID string) string {
	t.Helper()
	trigger, err := f.db.GetTriggerConfiguration(f.ctx, &store.GetTriggerConfigurationQuery{ID: triggerID})
	if err != nil {
		t.Fatalf("load trigger: %v", err)
	}
	if trigger.Trigger.AzureDevOps == nil || trigger.Trigger.AzureDevOps.WebhookSecret == "" {
		t.Fatalf("webhook trigger has no secret")
	}
	return trigger.Trigger.AzureDevOps.WebhookSecret
}

func (f *authzFixture) createWorkerSession(t *testing.T, orgID, workerID string) string {
	t.Helper()
	session, err := f.db.CreateSession(f.ctx, types.Session{
		ID:             system.GenerateSessionID(),
		Name:           f.mark("sessWorker"),
		Owner:          f.users[aOwner].ID,
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: orgID,
		Metadata:       types.SessionMetadata{OrgWorkerID: workerID},
	})
	if err != nil {
		t.Fatalf("create worker session: %v", err)
	}
	t.Cleanup(func() { _, _ = f.db.DeleteSession(context.Background(), session.ID) })
	return session.ID
}

// uploadAppFile puts one file in the agent's filestore folder so listing it
// has something to return.
func (f *authzFixture) uploadAppFile(t *testing.T, appID string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", "authz.txt")
	if err != nil {
		t.Fatalf("multipart: %v", err)
	}
	_, _ = part.Write([]byte(f.mark("appFile")))
	if err := writer.Close(); err != nil {
		t.Fatalf("multipart: %v", err)
	}
	resp := f.do(t, aOwner, http.MethodPost, "/api/v1/filestore/upload?path=apps/"+appID, body.Bytes(),
		map[string]string{"Content-Type": writer.FormDataContentType()})
	if resp.status != http.StatusOK {
		t.Fatalf("upload app file: %d %s", resp.status, truncate(resp.body, 300))
	}
}

func (f *authzFixture) setTaskStatus(t *testing.T, id string, status types.SpecTaskStatus) {
	t.Helper()
	task, err := f.db.GetSpecTask(f.ctx, id)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.Status = status
	if err := f.db.UpdateSpecTask(f.ctx, task); err != nil {
		t.Fatalf("update task: %v", err)
	}
}
