package webservice

import (
	"context"
	"errors"
	"testing"
)

func TestShellEscape(t *testing.T) {
	cases := map[string]string{
		"":                                "''",
		"simple":                          "'simple'",
		"with spaces":                     "'with spaces'",
		"path/to/file.txt":                "'path/to/file.txt'",
		"abc'def":                         `'abc'\''def'`,
		"; rm -rf / # injection attempt":  "'; rm -rf / # injection attempt'",
		"$(cat /etc/passwd)":              "'$(cat /etc/passwd)'",
		"`hostname`":                      "'`hostname`'",
		"https://user:p@ss@github.com/o/r": "'https://user:p@ss@github.com/o/r'",
	}
	for in, want := range cases {
		if got := shellEscape(in); got != want {
			t.Errorf("shellEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProjectSecretEnv(t *testing.T) {
	ctx := context.Background()

	// No getter wired: returns empty, never nil-panics.
	c := &Controller{}
	if got := c.projectSecretEnv(ctx, "prj_1", "sbx_1"); len(got) != 0 {
		t.Errorf("expected empty env with no getter, got %v", got)
	}

	// Empty project ID: getter must not be called.
	called := false
	c.SetProjectSecretsGetter(func(context.Context, string) ([]string, error) {
		called = true
		return []string{"X=1"}, nil
	})
	if got := c.projectSecretEnv(ctx, "", "sbx_1"); len(got) != 0 || called {
		t.Errorf("expected no getter call for empty project, got %v called=%v", got, called)
	}

	// Getter returns secrets: they are passed through verbatim.
	c.SetProjectSecretsGetter(func(_ context.Context, projectID string) ([]string, error) {
		if projectID != "prj_1" {
			t.Errorf("unexpected project id %q", projectID)
		}
		return []string{"API_KEY=prod", "DB_URL=prod"}, nil
	})
	got := c.projectSecretEnv(ctx, "prj_1", "sbx_1")
	if len(got) != 2 || got[0] != "API_KEY=prod" || got[1] != "DB_URL=prod" {
		t.Errorf("expected prod secrets passed through, got %v", got)
	}

	// Getter error: deploy is not blocked, env is empty.
	c.SetProjectSecretsGetter(func(context.Context, string) ([]string, error) {
		return nil, errors.New("boom")
	})
	if got := c.projectSecretEnv(ctx, "prj_1", "sbx_1"); len(got) != 0 {
		t.Errorf("expected empty env on getter error, got %v", got)
	}
}
