package mcptools_test

import (
	"context"
	"encoding/json"
	"testing"

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
	fc := &fakeProjectConfig{}
	_, err := invokeAsCaller(t, mcptools.NewListSecrets(mcptools.Deps{ProjectConfig: fc}))
	if err == nil {
		t.Fatal("expected legacy value-returning path to be disabled")
	}
}

func TestListSecrets_EmptyIsEmptyObjectNotNull(t *testing.T) {
	t.Parallel()
	fc := &fakeProjectConfig{}
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
