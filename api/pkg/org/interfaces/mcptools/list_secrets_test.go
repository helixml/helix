package mcptools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	domainTool "github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
)

func invokeAsCaller(t *testing.T, tl domainTool.Tool) (json.RawMessage, error) {
	t.Helper()
	return tl.Invoke(context.Background(), domainTool.Invocation{
		Args:   json.RawMessage(`{}`),
		Caller: fakeOwnerCaller{},
	})
}

func TestListSecrets_RejectsLegacyProjectSecretValuePath(t *testing.T) {
	t.Parallel()
	fc := &fakeProjectConfig{
		list: func(orgID string, botID orgchart.NodeID) (map[string]string, error) {
			if orgID != "org-test" {
				t.Errorf("orgID = %q, want org-test", orgID)
			}
			// Scoped to the caller bot, never an arg.
			if botID != "b-owner" {
				t.Errorf("botID = %q, want b-owner", botID)
			}
			return map[string]string{"DRONE_TOKEN": "abc123", "DRONE_SERVER": "https://drone"}, nil
		},
	}
	_, err := invokeAsCaller(t, mcptools.NewListSecrets(mcptools.Deps{ProjectConfig: fc}))
	if err == nil {
		t.Fatal("expected legacy value-returning path to be disabled")
	}
	if fc.listN != 0 {
		t.Fatalf("legacy project secret reader called %d times", fc.listN)
	}
}

func TestListSecrets_EmptyIsEmptyObjectNotNull(t *testing.T) {
	t.Parallel()
	fc := &fakeProjectConfig{
		list: func(string, orgchart.NodeID) (map[string]string, error) { return nil, nil },
	}
	_, err := invokeAsCaller(t, mcptools.NewListSecrets(mcptools.Deps{ProjectConfig: fc}))
	if err == nil {
		t.Fatal("expected worker-secret service error")
	}
}

func TestListSecrets_UnwiredPortErrors(t *testing.T) {
	t.Parallel()
	_, err := invokeAsCaller(t, mcptools.NewListSecrets(mcptools.Deps{}))
	if err == nil {
		t.Fatal("expected worker-secret service error")
	}
}
