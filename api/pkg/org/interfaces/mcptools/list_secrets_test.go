package mcptools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	domainTool "github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
)

func invokeAsCaller(t *testing.T, tl domainTool.Tool) (json.RawMessage, error) {
	t.Helper()
	return tl.Invoke(context.Background(), domainTool.Invocation{
		Args:   json.RawMessage(`{}`),
		Caller: fakeOwnerCaller{},
	})
}

func TestListSecrets_ReturnsNameValueMap(t *testing.T) {
	t.Parallel()
	fc := &fakeProjectConfig{
		list: func(orgID string, botID orgchart.BotID) (map[string]string, error) {
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
	raw, err := invokeAsCaller(t, mcptools.NewListSecrets(mcptools.Deps{ProjectConfig: fc}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var got struct {
		Secrets map[string]string `json:"secrets"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Secrets["DRONE_TOKEN"] != "abc123" || got.Secrets["DRONE_SERVER"] != "https://drone" {
		t.Fatalf("secrets = %#v, want the two DRONE_* entries", got.Secrets)
	}
	if fc.listN != 1 {
		t.Fatalf("listN = %d, want 1", fc.listN)
	}
}

func TestListSecrets_EmptyIsEmptyObjectNotNull(t *testing.T) {
	t.Parallel()
	fc := &fakeProjectConfig{
		list: func(string, orgchart.BotID) (map[string]string, error) { return nil, nil },
	}
	raw, err := invokeAsCaller(t, mcptools.NewListSecrets(mcptools.Deps{ProjectConfig: fc}))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if string(raw) != `{"secrets":{}}` {
		t.Fatalf("raw = %s, want {\"secrets\":{}}", raw)
	}
}

func TestListSecrets_UnwiredPortErrors(t *testing.T) {
	t.Parallel()
	_, err := invokeAsCaller(t, mcptools.NewListSecrets(mcptools.Deps{ProjectConfig: runtime.NoopProjectConfig{}}))
	if !errors.Is(err, runtime.ErrProjectConfigUnsupported) {
		t.Fatalf("err = %v, want ErrProjectConfigUnsupported", err)
	}
}
