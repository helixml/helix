package helix

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/types"
)

// findMCP returns the AssistantMCP named `name` on assistants[0], or
// nil if not present.
func findMCP(cfg types.AppConfig, name string) *types.AssistantMCP {
	if len(cfg.Helix.Assistants) == 0 {
		return nil
	}
	for i := range cfg.Helix.Assistants[0].MCPs {
		if cfg.Helix.Assistants[0].MCPs[i].Name == name {
			return &cfg.Helix.Assistants[0].MCPs[i]
		}
	}
	return nil
}

// TestAttachHelixOrgMCPAppends writes a fresh entry when no MCP with
// HelixOrgMCPName exists yet.
func TestAttachHelixOrgMCPAppends(t *testing.T) {
	t.Parallel()
	svc := newFakeProjectService()
	err := AttachHelixOrgMCP(context.Background(), svc, "app_test", "http://helix-org:8081", orgchart.BotID("w-eng"), "k_service")
	if err != nil {
		t.Fatalf("AttachHelixOrgMCP: %v", err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.getAppCalls != 1 || svc.updateAppCalls != 1 {
		t.Fatalf("expected one GetAppConfig + one UpdateAppConfig; got get=%d update=%d", svc.getAppCalls, svc.updateAppCalls)
	}
	mcp := findMCP(svc.updateAppLastCfg, HelixOrgMCPName)
	if mcp == nil {
		t.Fatalf("UpdateApp config missing %q MCP entry: %+v", HelixOrgMCPName, svc.updateAppLastCfg)
	}
	if mcp.Transport != "http" {
		t.Errorf("Transport = %q, want http", mcp.Transport)
	}
	if !strings.HasSuffix(mcp.URL, "/workers/w-eng/mcp") {
		t.Errorf("URL = %q, want suffix /workers/w-eng/mcp", mcp.URL)
	}
	if mcp.Headers["Authorization"] != "Bearer k_service" {
		t.Errorf("Headers[Authorization] = %q, want %q", mcp.Headers["Authorization"], "Bearer k_service")
	}
}

// TestAttachHelixOrgMCPUpsertReplacesExisting pins the idempotency
// contract — re-attaching with a different bearer overwrites the
// entry in place instead of appending a duplicate.
func TestAttachHelixOrgMCPUpsertReplacesExisting(t *testing.T) {
	t.Parallel()
	svc := newFakeProjectService()
	svc.appConfig.Helix.Assistants[0].MCPs = []types.AssistantMCP{
		{Name: HelixOrgMCPName, Transport: "http", URL: "http://old/workers/w-eng/mcp", Headers: map[string]string{"Authorization": "Bearer old"}},
		{Name: "other", URL: "http://other/mcp"},
	}
	if err := AttachHelixOrgMCP(context.Background(), svc, "app_test", "http://helix-org:8081", orgchart.BotID("w-eng"), "k_new"); err != nil {
		t.Fatalf("AttachHelixOrgMCP: %v", err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	asst := svc.updateAppLastCfg.Helix.Assistants[0]
	if got := len(asst.MCPs); got != 2 {
		t.Fatalf("MCP entries = %d, want 2 (upsert must not append a duplicate); got %+v", got, asst.MCPs)
	}
	helix := findMCP(svc.updateAppLastCfg, HelixOrgMCPName)
	if helix == nil {
		t.Fatalf("helix MCP entry missing")
	}
	if helix.Headers["Authorization"] != "Bearer k_new" {
		t.Errorf("upsert did not refresh bearer; got %q", helix.Headers["Authorization"])
	}
	if !strings.HasSuffix(helix.URL, "/workers/w-eng/mcp") {
		t.Errorf("URL not rewritten; got %q", helix.URL)
	}
}

// TestAttachHelixOrgMCPIgnoresRequestBearer pins that a short-lived
// activation/session token is never persisted on the long-lived app.
func TestAttachHelixOrgMCPIgnoresRequestBearer(t *testing.T) {
	t.Parallel()
	svc := newFakeProjectService()
	ctx := WithBearerToken(context.Background(), "k_user")
	if err := AttachHelixOrgMCP(ctx, svc, "app_test", "http://helix-org:8081", orgchart.BotID("w-eng"), "k_service"); err != nil {
		t.Fatalf("AttachHelixOrgMCP: %v", err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	mcp := findMCP(svc.updateAppLastCfg, HelixOrgMCPName)
	if mcp == nil || mcp.Headers["Authorization"] != "Bearer k_service" {
		t.Errorf("service bearer must be persisted; got %+v", mcp)
	}
}

// TestAttachHelixOrgMCPEmptyBearerOmitsHeader pins the standalone-
// deployment shape: when neither ctx nor fallback carry a bearer, no
// Authorization header is written. The standalone helix-org MCP is
// not auth-gated so an empty header is correct.
func TestAttachHelixOrgMCPEmptyBearerOmitsHeader(t *testing.T) {
	t.Parallel()
	svc := newFakeProjectService()
	if err := AttachHelixOrgMCP(context.Background(), svc, "app_test", "http://helix-org:8081", orgchart.BotID("w-eng"), ""); err != nil {
		t.Fatalf("AttachHelixOrgMCP: %v", err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	mcp := findMCP(svc.updateAppLastCfg, HelixOrgMCPName)
	if mcp == nil {
		t.Fatalf("MCP entry missing")
	}
	if mcp.Headers != nil {
		t.Errorf("Headers must be nil when no bearer is set; got %+v", mcp.Headers)
	}
}

// TestAttachHelixOrgMCPRejectsMissingInputs pins the up-front
// validation. Each failure mode returns a clear error rather than
// nil-derefing or writing a malformed entry.
func TestAttachHelixOrgMCPRejectsMissingInputs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		svc     ProjectService
		appID   string
		url     string
		worker  orgchart.BotID
		errFrag string
	}{
		{"nil service", nil, "app_test", "http://helix-org", "w-eng", "ProjectService is nil"},
		{"empty appID", newFakeProjectService(), "", "http://helix-org", "w-eng", "appID is empty"},
		{"empty URL", newFakeProjectService(), "app_test", "", "w-eng", "helixOrgURL is empty"},
		{"empty workerID", newFakeProjectService(), "app_test", "http://helix-org", "", "workerID is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := AttachHelixOrgMCP(context.Background(), tc.svc, tc.appID, tc.url, tc.worker, "")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errFrag)
			}
			if !strings.Contains(err.Error(), tc.errFrag) {
				t.Errorf("err = %q, want contains %q", err.Error(), tc.errFrag)
			}
		})
	}
}

// TestAttachHelixOrgMCPRequiresAssistant pins the contract on the
// agent app shape: a project's auto-provisioned agent app always
// has a [0]th assistant; if it doesn't, attach refuses rather than
// silently appending into a zero-length slice.
func TestAttachHelixOrgMCPRequiresAssistant(t *testing.T) {
	t.Parallel()
	svc := newFakeProjectService()
	svc.appConfig = types.AppConfig{} // no assistants
	err := AttachHelixOrgMCP(context.Background(), svc, "app_test", "http://helix-org", orgchart.BotID("w-eng"), "")
	if err == nil || !strings.Contains(err.Error(), "no assistants") {
		t.Errorf("expected 'no assistants' error, got %v", err)
	}
}

// TestAttachHelixOrgMCPPropagatesGetError pins the error path: a
// failure from GetAppConfig surfaces (wrapped) rather than crashing
// or writing garbage.
func TestAttachHelixOrgMCPPropagatesGetError(t *testing.T) {
	t.Parallel()
	svc := &failingProjectService{getErr: errors.New("boom")}
	err := AttachHelixOrgMCP(context.Background(), svc, "app_test", "http://helix-org", orgchart.BotID("w-eng"), "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected wrapped GetAppConfig error, got %v", err)
	}
}

// failingProjectService stubs ProjectService so the GetAppConfig
// error-path test can isolate failure-mode behaviour without
// touching the fake's other state.
type failingProjectService struct {
	noopProjectService
	getErr error
}

func (f *failingProjectService) GetAppConfig(_ context.Context, _ string) (types.AppConfig, error) {
	return types.AppConfig{}, f.getErr
}

// noopProjectService satisfies ProjectService with no-op
// implementations so test stand-ins can embed it and override only
// the method under test.
type noopProjectService struct{}

func (noopProjectService) WhoAmI(_ context.Context) (string, error) { return "u-test", nil }
func (noopProjectService) ApplyProject(_ context.Context, _ types.ProjectApplyRequest) (types.ProjectApplyResponse, error) {
	return types.ProjectApplyResponse{}, nil
}
func (noopProjectService) GetProject(_ context.Context, _ string) (types.Project, error) {
	return types.Project{}, nil
}
func (noopProjectService) UpdateProject(_ context.Context, _ string, _ types.ProjectUpdateRequest) (types.Project, error) {
	return types.Project{}, nil
}
func (noopProjectService) PutProjectSecret(_ context.Context, _, _, _ string) error { return nil }
func (noopProjectService) CreateGitRepo(_ context.Context, _ types.GitRepositoryCreateRequest) (types.GitRepository, error) {
	return types.GitRepository{}, nil
}
func (noopProjectService) GetGitRepo(_ context.Context, repoID string) (types.GitRepository, error) {
	return types.GitRepository{ID: repoID, Name: repoID}, nil
}
func (noopProjectService) DeleteGitRepo(_ context.Context, _ string) error { return nil }
func (noopProjectService) AttachRepoToProject(_ context.Context, _, _ string, _ bool) error {
	return nil
}
func (noopProjectService) CreateBranch(_ context.Context, _, _, _ string) error { return nil }
func (noopProjectService) GetAppConfig(_ context.Context, _ string) (types.AppConfig, error) {
	return types.AppConfig{}, nil
}
func (noopProjectService) UpdateAppConfig(_ context.Context, _ string, _ types.AppConfig) error {
	return nil
}
func (noopProjectService) DeleteProject(_ context.Context, _ string) error { return nil }
func (noopProjectService) DeleteApp(_ context.Context, _ string) error     { return nil }
