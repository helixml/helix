package server

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestRedactGitRepository(t *testing.T) {
	original := &types.GitRepository{
		Username: "repo-user",
		Password: "repo-password",
		GitHub: &types.GitHub{
			PersonalAccessToken: "github-token",
			BaseURL:             "https://github.example.com",
			WebhookSecret:       "webhook-secret",
			ReviewBotUserID:     291906607,
			AppID:               1,
			InstallationID:      2,
			PrivateKey:          "private-key",
		},
		GitLab: &types.GitLab{
			PersonalAccessToken: "gitlab-token",
			BaseURL:             "https://gitlab.example.com",
		},
		AzureDevOps: &types.AzureDevOps{
			OrganizationURL:     "https://dev.azure.com/example",
			PersonalAccessToken: "ado-token",
			TenantID:            "tenant",
			ClientID:            "client",
			ClientSecret:        "client-secret",
		},
		Bitbucket: &types.Bitbucket{
			Username:    "bitbucket-user",
			AppPassword: "app-password",
			BaseURL:     "https://bitbucket.example.com",
		},
	}

	got := redactGitRepository(original)
	if got == original || got.GitHub == original.GitHub || got.GitLab == original.GitLab ||
		got.AzureDevOps == original.AzureDevOps || got.Bitbucket == original.Bitbucket {
		t.Fatal("redaction must return repository and provider copies")
	}
	if got.Password != "" || got.GitHub.PersonalAccessToken != "" || got.GitHub.PrivateKey != "" ||
		got.GitHub.WebhookSecret != "" || got.GitLab.PersonalAccessToken != "" ||
		got.AzureDevOps.PersonalAccessToken != "" || got.AzureDevOps.ClientSecret != "" ||
		got.Bitbucket.AppPassword != "" {
		t.Fatalf("redacted repository still contains credentials: %#v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal redacted repository: %v", err)
	}
	for _, secret := range []string{
		"repo-password", "github-token", "private-key", "webhook-secret",
		"gitlab-token", "ado-token", "client-secret", "app-password",
	} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("encoded repository contains credential %q: %s", secret, encoded)
		}
	}
	if got.Username != "repo-user" || got.GitHub.BaseURL != "https://github.example.com" ||
		got.GitHub.ReviewBotUserID != 291906607 || got.GitHub.AppID != 1 || got.GitHub.InstallationID != 2 ||
		got.GitLab.BaseURL != "https://gitlab.example.com" || got.AzureDevOps.OrganizationURL != "https://dev.azure.com/example" ||
		got.AzureDevOps.TenantID != "tenant" || got.AzureDevOps.ClientID != "client" ||
		got.Bitbucket.Username != "bitbucket-user" || got.Bitbucket.BaseURL != "https://bitbucket.example.com" {
		t.Fatalf("redaction removed non-secret settings: %#v", got)
	}
	if original.Password != "repo-password" || original.GitHub.PersonalAccessToken != "github-token" ||
		original.GitHub.PrivateKey != "private-key" || original.GitHub.WebhookSecret != "webhook-secret" ||
		original.GitLab.PersonalAccessToken != "gitlab-token" || original.AzureDevOps.PersonalAccessToken != "ado-token" ||
		original.AzureDevOps.ClientSecret != "client-secret" || original.Bitbucket.AppPassword != "app-password" {
		t.Fatalf("redaction mutated the stored repository: %#v", original)
	}
}

func TestRedactGitRepositories(t *testing.T) {
	if redactGitRepositories(nil) != nil {
		t.Fatal("nil repository list must remain nil")
	}

	original := &types.GitRepository{Password: "secret"}
	got := redactGitRepositories([]*types.GitRepository{original, nil})

	if len(got) != 2 || got[0] == original || got[0].Password != "" || got[1] != nil {
		t.Fatalf("redacted repositories = %#v", got)
	}
	if original.Password != "secret" {
		t.Fatal("list redaction mutated the stored repository")
	}
}
