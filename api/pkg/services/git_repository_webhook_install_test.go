package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// fakeWebhookInstaller records UpsertWebhook calls for assertions.
type fakeWebhookInstaller struct {
	calls []upsertCall
	err   error
}

type upsertCall struct {
	owner, repo, name, url string
	events                 []string
	secret                 string
}

func (f *fakeWebhookInstaller) UpsertWebhook(_ context.Context, owner, repo, name, url string, events []string, secret string) error {
	f.calls = append(f.calls, upsertCall{owner, repo, name, url, events, secret})
	return f.err
}

func newInstallTestService(t *testing.T, ctrl *gomock.Controller) *GitRepositoryService {
	t.Helper()
	return NewGitRepositoryService(store.NewMockStore(ctrl), t.TempDir(), "http://api:8080", "helix", "helix@test")
}

func TestEnsureGitHubReviewWebhook_DisabledIsNoop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := newInstallTestService(t, ctrl)
	svc.SetGitHubReviewWebhooks("") // disabled

	installer := &fakeWebhookInstaller{}
	svc.ensureGitHubReviewWebhook(context.Background(), installer, "repo-1", "owner", "repo")

	if len(installer.calls) != 0 {
		t.Fatalf("disabled feature must not install hooks, got %d calls", len(installer.calls))
	}
}

func TestEnsureGitHubReviewWebhook_GeneratesAndPersistsSecret(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	svc := NewGitRepositoryService(mockStore, t.TempDir(), "http://api:8080", "helix", "helix@test")
	svc.SetGitHubReviewWebhooks("https://prime.example.com/api/v1/webhooks/github/reviews")

	repo := &types.GitRepository{ID: "repo-1", ExternalURL: "https://github.com/owner/repo"}
	mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo-1").Return(repo, nil)

	var persisted *types.GitRepository
	mockStore.EXPECT().UpdateGitRepository(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, r *types.GitRepository) error {
			persisted = r
			return nil
		})

	installer := &fakeWebhookInstaller{}
	svc.ensureGitHubReviewWebhook(context.Background(), installer, "repo-1", "owner", "repo")

	if len(installer.calls) != 1 {
		t.Fatalf("expected 1 upsert call, got %d", len(installer.calls))
	}
	call := installer.calls[0]
	if call.url != "https://prime.example.com/api/v1/webhooks/github/reviews/repo-1" {
		t.Fatalf("payload URL = %q", call.url)
	}
	if call.secret == "" || len(call.secret) < 32 {
		t.Fatalf("generated secret too short: %q", call.secret)
	}
	if call.secret != persisted.GitHub.WebhookSecret {
		t.Fatalf("hook secret %q != persisted secret %q", call.secret, persisted.GitHub.WebhookSecret)
	}
	if strings.Join(call.events, ",") != "pull_request_review" {
		t.Fatalf("events = %v", call.events)
	}
}

func TestEnsureGitHubReviewWebhook_ExistingSecretNotRegenerated(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	svc := NewGitRepositoryService(mockStore, t.TempDir(), "http://api:8080", "helix", "helix@test")
	svc.SetGitHubReviewWebhooks("https://prime.example.com/api/v1/webhooks/github/reviews")

	repo := &types.GitRepository{ID: "repo-1", ExternalURL: "https://github.com/owner/repo",
		GitHub: &types.GitHub{WebhookSecret: "existing-secret"}}
	mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo-1").Return(repo, nil)
	mockStore.EXPECT().UpdateGitRepository(gomock.Any(), gomock.Any()).Times(0)

	installer := &fakeWebhookInstaller{}
	svc.ensureGitHubReviewWebhook(context.Background(), installer, "repo-1", "owner", "repo")

	if len(installer.calls) != 1 || installer.calls[0].secret != "existing-secret" {
		t.Fatalf("existing secret must be reused, got %+v", installer.calls)
	}
}

func TestEnsureGitHubReviewWebhook_PersistFailureSkipsInstall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	svc := NewGitRepositoryService(mockStore, t.TempDir(), "http://api:8080", "helix", "helix@test")
	svc.SetGitHubReviewWebhooks("https://prime.example.com/api/v1/webhooks/github/reviews")

	repo := &types.GitRepository{ID: "repo-1", ExternalURL: "https://github.com/owner/repo"}
	mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo-1").Return(repo, nil)
	mockStore.EXPECT().UpdateGitRepository(gomock.Any(), gomock.Any()).Return(errors.New("db down"))

	installer := &fakeWebhookInstaller{}
	svc.ensureGitHubReviewWebhook(context.Background(), installer, "repo-1", "owner", "repo")

	if len(installer.calls) != 0 {
		t.Fatalf("must not install a hook whose secret could not be persisted, got %d calls", len(installer.calls))
	}
}

func TestEnsureGitHubReviewWebhook_InstallFailureIsBestEffort(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	svc := NewGitRepositoryService(mockStore, t.TempDir(), "http://api:8080", "helix", "helix@test")
	svc.SetGitHubReviewWebhooks("https://prime.example.com/api/v1/webhooks/github/reviews")

	repo := &types.GitRepository{ID: "repo-1", ExternalURL: "https://github.com/owner/repo",
		GitHub: &types.GitHub{WebhookSecret: "existing-secret"}}
	mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo-1").Return(repo, nil)

	installer := &fakeWebhookInstaller{err: errors.New("github 500")}
	svc.ensureGitHubReviewWebhook(context.Background(), installer, "repo-1", "owner", "repo") // must not panic/propagate
}
