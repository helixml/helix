package server

import (
	"context"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type recordingBotConfigClient struct {
	sessionID    string
	workerID     string
	instructions string
	launch       runtimehelix.SessionLaunchConfig
	calls        []string
}

func TestRestartOrgBotSessionRejectsMissingOwner(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetSession(gomock.Any(), "ses-cos").Return(&types.Session{ID: "ses-cos", Owner: "missing"}, nil)
	st.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, nil)

	err := (&HelixAPIServer{Store: st}).restartOrgBotSessionWithPreservedThread(context.Background(), "ses-cos")
	if err == nil || err.Error() != "session owner not found" {
		t.Fatalf("error = %v, want session owner not found", err)
	}
}

func (f *recordingBotConfigClient) GetAppConfig(context.Context, string) (types.AppConfig, error) {
	return types.AppConfig{}, nil
}

func (f *recordingBotConfigClient) SyncAgentProfile(_ context.Context, sessionID, _ string, workerID, instructions string, launch runtimehelix.SessionLaunchConfig) error {
	f.sessionID = sessionID
	f.workerID = workerID
	f.instructions = instructions
	f.launch = launch
	f.calls = append(f.calls, "sync")
	return nil
}

func TestBotConfigApplierSyncsLaunchBeforePreservedThreadRestart(t *testing.T) {
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	bot, err := orgchart.NewNode("chief-of-staff", "Lead the organization", nil, time.Now(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	bot = bot.WithSandboxRuntime(string(types.SandboxRuntimeHeadlessUbuntu))
	if err := st.Nodes.Create(ctx, bot); err != nil {
		t.Fatal(err)
	}
	if err := runtimehelix.SaveSession(ctx, st, "org-test", bot.ID, "ses-cos"); err != nil {
		t.Fatal(err)
	}

	client := &recordingBotConfigClient{}
	applier := botConfigApplier{
		client: client, store: st, configs: configregistry.New(st.Configs),
		restart: func(_ context.Context, sessionID string) error {
			client.calls = append(client.calls, "restart:"+sessionID)
			return nil
		},
	}
	if err := applier.ApplyConfig(ctx, "org-test", bot.ID); err != nil {
		t.Fatal(err)
	}

	if client.sessionID != "ses-cos" || client.workerID != "chief-of-staff" {
		t.Fatalf("synced session/worker = %q/%q", client.sessionID, client.workerID)
	}
	if client.launch.SandboxRuntime != types.SandboxRuntimeHeadlessUbuntu {
		t.Fatalf("sandbox runtime = %q, want headless-ubuntu", client.launch.SandboxRuntime)
	}
	if client.instructions == "" {
		t.Fatal("runtime instructions were not synced")
	}
	if len(client.calls) != 2 || client.calls[0] != "sync" || client.calls[1] != "restart:ses-cos" {
		t.Fatalf("call order = %v, want sync then restart", client.calls)
	}
}
