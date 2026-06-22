package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// TestCreateTopicParity_RESTvsMCP is the load-bearing test for Phase A:
// the REST POST /topics handler and the MCP create_topic tool now
// share one application service (application/topics). This asserts they
// produce byte-identical store state for the same logical request, so
// the two adapters can never drift.
func TestCreateTopicParity_RESTvsMCP(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }
	newID := func() string { return "fixed" }

	// --- REST path ---
	restDeps, restStore, _ := newDepsClock(t, clock, newID)
	h := orgapi.Handler(restDeps)

	body := orgapi.CreateTopicRequest{
		ID:          "s-parity",
		Name:        "parity",
		Description: "shared-service parity",
		As:          "w-owner", // the worker the human is acting as (matches the MCP caller below)
		Transport: &orgapi.TransportRequestField{
			Kind:   "webhook",
			Config: map[string]interface{}{"outbound_url": "https://example.com/in"},
		},
	}
	rec := do(t, h, "POST", "/topics", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("REST create: status %d, body=%s", rec.Code, rec.Body)
	}

	// --- MCP path (via the public registry) ---
	mcpStore := orggorm.GetOrgTestDB(t)
	mcpDeps := mcptools.DefaultDeps(mcpStore)
	mcpDeps.Now = clock
	mcpDeps.NewID = newID
	reg := mcptools.NewRegistry()
	if err := mcptools.RegisterBuiltins(reg, mcpDeps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	createTopic, err := reg.Get(mcptools.CreateTopicName)
	if err != nil {
		t.Fatalf("get create_topic tool: %v", err)
	}
	caller, err := orgchart.NewHumanWorker("w-owner", "r-owner", "", "org-test")
	if err != nil {
		t.Fatalf("new caller: %v", err)
	}
	args, _ := json.Marshal(map[string]any{
		"id":          "s-parity",
		"name":        "parity",
		"description": "shared-service parity",
		"transport": map[string]any{
			"kind":   "webhook",
			"config": map[string]any{"outbound_url": "https://example.com/in"},
		},
	})
	if _, err := createTopic.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("MCP create_topic invoke: %v", err)
	}

	// --- Compare resulting store state ---
	restRow, err := restStore.Topics.Get(context.Background(), "org-test", "s-parity")
	if err != nil {
		t.Fatalf("REST store get: %v", err)
	}
	mcpRow, err := mcpStore.Topics.Get(context.Background(), "org-test", "s-parity")
	if err != nil {
		t.Fatalf("MCP store get: %v", err)
	}

	if restRow.ID != mcpRow.ID {
		t.Errorf("ID differs: REST=%q MCP=%q", restRow.ID, mcpRow.ID)
	}
	if restRow.Name != mcpRow.Name {
		t.Errorf("Name differs: REST=%q MCP=%q", restRow.Name, mcpRow.Name)
	}
	if restRow.Description != mcpRow.Description {
		t.Errorf("Description differs: REST=%q MCP=%q", restRow.Description, mcpRow.Description)
	}
	if restRow.CreatedBy != mcpRow.CreatedBy {
		t.Errorf("CreatedBy differs: REST=%q MCP=%q", restRow.CreatedBy, mcpRow.CreatedBy)
	}
	if !restRow.CreatedAt.Equal(mcpRow.CreatedAt) {
		t.Errorf("CreatedAt differs: REST=%v MCP=%v", restRow.CreatedAt, mcpRow.CreatedAt)
	}
	if restRow.OrganizationID != mcpRow.OrganizationID {
		t.Errorf("OrganizationID differs: REST=%q MCP=%q", restRow.OrganizationID, mcpRow.OrganizationID)
	}
	if restRow.Transport.Kind != mcpRow.Transport.Kind {
		t.Errorf("Transport.Kind differs: REST=%q MCP=%q", restRow.Transport.Kind, mcpRow.Transport.Kind)
	}
	if string(restRow.Transport.Config) != string(mcpRow.Transport.Config) {
		t.Errorf("Transport.Config differs: REST=%s MCP=%s", restRow.Transport.Config, mcpRow.Transport.Config)
	}
}
