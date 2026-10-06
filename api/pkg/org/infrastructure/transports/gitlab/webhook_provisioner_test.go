package gitlab_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	gitlabtransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/gitlab"
)

type webhookManager struct {
	payloadURL, signingToken, secretToken string
	active                                bool
}

func (m *webhookManager) InstallGitLabWebhook(_ context.Context, _, _, payloadURL, signingToken, secretToken string, _ []string) (int64, string, string, error) {
	m.payloadURL, m.signingToken, m.secretToken = payloadURL, signingToken, secretToken
	return 42, "https://gitlab.com/group/project/-/hooks/42/edit", "group/project", nil
}

func (m *webhookManager) FindGitLabWebhook(_ context.Context, _, _, payloadURL string) (int64, string, bool, bool, error) {
	m.payloadURL = payloadURL
	return 42, "https://gitlab.com/group/project/-/hooks/42/edit", true, m.active, nil
}

func provisionerTrigger(t *testing.T) trigger.Trigger {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"repository_id": "repo-1", "events": []string{"Merge Request Hook"}})
	row, err := trigger.New("s-trigger", "org/id", "trigger", "", transport.KindGitLab, raw, "b-owner", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestWebhookProvisionerInstallPreservesSecretsAndBuildsURL(t *testing.T) {
	const fixtureSigningToken = "test-signing-token"
	registry := configregistry.New(orggorm.GetOrgTestDB(t).Configs)
	registry.Register(configregistry.Spec{Key: "transport.gitlab", Type: configregistry.TypeObject, Secrets: []string{"signing_token", "secret_token"}})
	if err := registry.Set(context.Background(), "org/id", "transport.gitlab", `{"signing_token":"`+fixtureSigningToken+`"}`); err != nil {
		t.Fatal(err)
	}
	manager := &webhookManager{}
	result, err := gitlabtransport.NewWebhookProvisioner(registry, manager, "https://prime.helix.ml").Install(context.Background(), "org/id", provisionerTrigger(t))
	if err != nil {
		t.Fatal(err)
	}
	wantURL := "https://prime.helix.ml/api/v1/orgs/org%2Fid/triggers/s-trigger/gitlab/webhook"
	if result.PayloadURL != wantURL || manager.payloadURL != wantURL {
		t.Fatalf("url=%q", result.PayloadURL)
	}
	if manager.signingToken != fixtureSigningToken || manager.secretToken == "" {
		t.Fatalf("auth=%q/%q", manager.signingToken, manager.secretToken)
	}
	var auth gitlabtransport.Config
	if err := registry.GetObject(context.Background(), "org/id", "transport.gitlab", &auth); err != nil {
		t.Fatal(err)
	}
	if auth.SigningToken != fixtureSigningToken || auth.SecretToken == "" {
		t.Fatalf("stored auth=%+v", auth)
	}
}

func TestWebhookProvisionerStatusPreservesDisabledState(t *testing.T) {
	registry := configregistry.New(orggorm.GetOrgTestDB(t).Configs)
	manager := &webhookManager{active: false}
	status, err := gitlabtransport.NewWebhookProvisioner(registry, manager, "https://prime.helix.ml").Status(context.Background(), "org/id", provisionerTrigger(t))
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "installed" || status.Active {
		t.Fatalf("status=%+v", status)
	}
}
