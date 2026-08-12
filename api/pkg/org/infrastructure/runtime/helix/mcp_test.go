package helix

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestRemoveHelixOrgMCP(t *testing.T) {
	svc := newFakeProjectService()
	svc.appConfig.Helix.Assistants[0].MCPs = []types.AssistantMCP{
		{Name: HelixOrgMCPName, URL: "http://old.example/mcp"},
		{Name: "other", URL: "stdio://other"},
	}

	if err := RemoveHelixOrgMCP(context.Background(), svc, "app_test"); err != nil {
		t.Fatalf("RemoveHelixOrgMCP: %v", err)
	}
	got := svc.updateAppLastCfg.Helix.Assistants[0].MCPs
	if len(got) != 1 || got[0].Name != "other" {
		t.Fatalf("MCPs = %+v, want only other", got)
	}
}

func TestRemoveHelixOrgMCPNoopWithoutLegacyEntry(t *testing.T) {
	svc := newFakeProjectService()
	svc.appConfig.Helix.Assistants[0].MCPs = []types.AssistantMCP{{Name: "other"}}

	if err := RemoveHelixOrgMCP(context.Background(), svc, "app_test"); err != nil {
		t.Fatalf("RemoveHelixOrgMCP: %v", err)
	}
	if svc.updateAppCalls != 0 {
		t.Fatalf("UpdateAppConfig calls = %d, want 0", svc.updateAppCalls)
	}
}
