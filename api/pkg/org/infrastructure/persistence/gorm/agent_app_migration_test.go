package gorm

import (
	"reflect"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBackfillAgentAppLinksRequiresSameOrganization(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE org_bots (
			org_id TEXT NOT NULL,
			id TEXT NOT NULL,
			kind TEXT NOT NULL,
			agent_app_id TEXT
		)`,
		`CREATE TABLE org_bot_runtime_state (
			org_id TEXT NOT NULL,
			bot_id TEXT NOT NULL,
			backend TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE apps (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			config JSON NOT NULL
		)`,
		`INSERT INTO apps (id, organization_id, config) VALUES
			('app-same-org', 'org-test', '{"helix":{"assistants":[{"name":"Agent"}]}}'),
			('app-other-org', 'org-other', '{"helix":{"assistants":[{"name":"Agent"}]}}'),
			('app-multiple', 'org-test', '{"helix":{"assistants":[{},{}]}}')`,
		`INSERT INTO org_bots (org_id, id, kind) VALUES
			('org-test', 'b-same-org', ''),
			('org-test', 'b-other-org', ''),
			('org-test', 'b-multiple', '')`,
		`INSERT INTO org_bot_runtime_state (org_id, bot_id, backend, key, value) VALUES
			('org-test', 'b-same-org', 'helix', 'agent_app_id', 'app-same-org'),
			('org-test', 'b-other-org', 'helix', 'agent_app_id', 'app-other-org'),
			('org-test', 'b-multiple', 'helix', 'agent_app_id', 'app-multiple')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed migration fixture: %v", err)
		}
	}

	if err := backfillAgentAppLinks(db); err != nil {
		t.Fatalf("backfill agent links: %v", err)
	}

	var sameOrg, otherOrg, multiple struct {
		AgentID *string `gorm:"column:agent_app_id"`
	}
	if err := db.Table("org_bots").Where("id = ?", "b-same-org").First(&sameOrg).Error; err != nil {
		t.Fatalf("read same-org bot: %v", err)
	}
	if err := db.Table("org_bots").Where("id = ?", "b-other-org").First(&otherOrg).Error; err != nil {
		t.Fatalf("read cross-org bot: %v", err)
	}
	if err := db.Table("org_bots").Where("id = ?", "b-multiple").First(&multiple).Error; err != nil {
		t.Fatalf("read multiple-assistant bot: %v", err)
	}
	if sameOrg.AgentID == nil || *sameOrg.AgentID != "app-same-org" {
		t.Fatalf("same-org link = %v", sameOrg.AgentID)
	}
	if otherOrg.AgentID != nil {
		t.Fatalf("cross-org link was backfilled: %q", *otherOrg.AgentID)
	}
	if multiple.AgentID != nil {
		t.Fatalf("multiple-assistant app was backfilled: %q", *multiple.AgentID)
	}
}

func TestBackfillProjectAgentAppsUpdatesProjectInPlace(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE org_bots (
			org_id TEXT NOT NULL,
			id TEXT NOT NULL,
			agent_app_id TEXT
		)`,
		`CREATE TABLE org_bot_runtime_state (
			org_id TEXT NOT NULL,
			bot_id TEXT NOT NULL,
			backend TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE apps (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			config JSON NOT NULL
		)`,
		`CREATE TABLE projects (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			default_helix_app_id TEXT,
			default_repo_id TEXT,
			deleted_at DATETIME
		)`,
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL
		)`,
		`INSERT INTO apps (id, organization_id, config) VALUES
			('app-canonical', 'org-test', '{"helix":{"assistants":[{"name":"Agent"}]}}'),
			('app-cross-org', 'org-other', '{"helix":{"assistants":[{"name":"Agent"}]}}'),
			('app-cross-project', 'org-test', '{"helix":{"assistants":[{"name":"Agent"}]}}'),
			('app-deleted', 'org-test', '{"helix":{"assistants":[{"name":"Agent"}]}}'),
			('app-conflict-a', 'org-test', '{"helix":{"assistants":[{"name":"Agent"}]}}'),
			('app-conflict-b', 'org-test', '{"helix":{"assistants":[{"name":"Agent"}]}}')`,
		`INSERT INTO org_bots (org_id, id, agent_app_id) VALUES
			('org-test', 'b-agent', 'app-canonical'),
			('org-test', 'b-cross-app', 'app-cross-org'),
			('org-test', 'b-cross-project', 'app-cross-project'),
			('org-test', 'b-deleted', 'app-deleted'),
			('org-test', 'b-conflict-a', 'app-conflict-a'),
			('org-test', 'b-conflict-b', 'app-conflict-b')`,
		`INSERT INTO org_bot_runtime_state (org_id, bot_id, backend, key, value)
		 VALUES
			('org-test', 'b-agent', 'helix', 'agent_app_id', 'app-canonical'),
			('org-test', 'b-agent', 'helix', 'project_id', 'project-existing'),
			('org-test', 'b-cross-app', 'helix', 'project_id', 'project-cross-app'),
			('org-test', 'b-cross-project', 'helix', 'project_id', 'project-cross-project'),
			('org-test', 'b-deleted', 'helix', 'project_id', 'project-deleted'),
			('org-test', 'b-conflict-a', 'helix', 'project_id', 'project-conflict'),
			('org-test', 'b-conflict-b', 'helix', 'project_id', 'project-conflict')`,
		`INSERT INTO projects (id, organization_id, default_helix_app_id, default_repo_id, deleted_at)
		 VALUES
			('project-existing', 'org-test', 'app-legacy', 'repo-existing', NULL),
			('project-cross-app', 'org-test', 'app-legacy-cross-app', 'repo-cross-app', NULL),
			('project-cross-project', 'org-other', 'app-legacy-cross-project', 'repo-cross-project', NULL),
			('project-deleted', 'org-test', 'app-legacy-deleted', 'repo-deleted', '2026-07-27 00:00:00'),
			('project-conflict', 'org-test', 'app-legacy-conflict', 'repo-conflict', NULL)`,
		`INSERT INTO sessions (id, project_id)
		 VALUES ('session-existing', 'project-existing')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed migration fixture: %v", err)
		}
	}

	type projectRow struct {
		ID                string
		OrganizationID    string
		DefaultHelixAppID string
		DefaultRepoID     string
	}
	type sessionRow struct {
		ID        string
		ProjectID string
	}
	type stateRow struct {
		OrgID   string
		NodeID  string
		Backend string
		Key     string
		Value   string
	}

	var projectBefore projectRow
	if err := db.Table("projects").Where("id = ?", "project-existing").First(&projectBefore).Error; err != nil {
		t.Fatalf("read project before migration: %v", err)
	}
	var sessionBefore sessionRow
	if err := db.Table("sessions").First(&sessionBefore).Error; err != nil {
		t.Fatalf("read session before migration: %v", err)
	}
	var statesBefore []stateRow
	if err := db.Table("org_bot_runtime_state").Order("bot_id, key, value").Find(&statesBefore).Error; err != nil {
		t.Fatalf("read runtime state before migration: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := backfillProjectAgentApps(db); err != nil {
			t.Fatalf("backfill project agent apps pass %d: %v", i+1, err)
		}
	}

	var projectAfter projectRow
	if err := db.Table("projects").Where("id = ?", "project-existing").First(&projectAfter).Error; err != nil {
		t.Fatalf("read project after migration: %v", err)
	}
	var sessionAfter sessionRow
	if err := db.Table("sessions").First(&sessionAfter).Error; err != nil {
		t.Fatalf("read session after migration: %v", err)
	}
	var statesAfter []stateRow
	if err := db.Table("org_bot_runtime_state").Order("bot_id, key, value").Find(&statesAfter).Error; err != nil {
		t.Fatalf("read runtime state after migration: %v", err)
	}

	if projectAfter.DefaultHelixAppID != "app-canonical" {
		t.Fatalf("default app = %q, want app-canonical", projectAfter.DefaultHelixAppID)
	}
	if projectAfter.ID != projectBefore.ID ||
		projectAfter.OrganizationID != projectBefore.OrganizationID ||
		projectAfter.DefaultRepoID != projectBefore.DefaultRepoID {
		t.Fatalf("project identity changed: before=%+v after=%+v", projectBefore, projectAfter)
	}
	if !reflect.DeepEqual(sessionAfter, sessionBefore) {
		t.Fatalf("session changed: before=%+v after=%+v", sessionBefore, sessionAfter)
	}
	if !reflect.DeepEqual(statesAfter, statesBefore) {
		t.Fatalf("runtime state changed: before=%+v after=%+v", statesBefore, statesAfter)
	}

	for projectID, wantAppID := range map[string]string{
		"project-cross-app":     "app-legacy-cross-app",
		"project-cross-project": "app-legacy-cross-project",
		"project-deleted":       "app-legacy-deleted",
		"project-conflict":      "app-legacy-conflict",
	} {
		var got projectRow
		if err := db.Table("projects").Where("id = ?", projectID).First(&got).Error; err != nil {
			t.Fatalf("read rejected project %s: %v", projectID, err)
		}
		if got.DefaultHelixAppID != wantAppID {
			t.Fatalf("%s default app = %q, want unchanged %q", projectID, got.DefaultHelixAppID, wantAppID)
		}
	}
}

func TestBackfillProjectAgentAppsPreservesExplicitDefaultAndConvergesLinks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE org_bots (
			org_id TEXT NOT NULL,
			id TEXT NOT NULL,
			agent_app_id TEXT
		)`,
		`CREATE TABLE org_bot_runtime_state (
			org_id TEXT NOT NULL,
			bot_id TEXT NOT NULL,
			backend TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			updated_at DATETIME,
			PRIMARY KEY (org_id, bot_id, backend, key)
		)`,
		`CREATE TABLE apps (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			config JSON NOT NULL
		)`,
		`CREATE TABLE projects (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			default_helix_app_id TEXT,
			deleted_at DATETIME
		)`,
		`INSERT INTO apps (id, organization_id, config) VALUES
			('app-explicit', 'org-test', '{"helix":{"assistants":[{"name":"Explicit"}]}}'),
			('app-new', 'org-test', '{"helix":{"assistants":[{"name":"New"}]}}'),
			('app-claimed', 'org-test', '{"helix":{"assistants":[{"name":"Claimed"}]}}'),
			('app-target', 'org-test', '{"helix":{"assistants":[{"name":"Target"}]}}')`,
		`INSERT INTO org_bots (org_id, id, agent_app_id)
		 VALUES
			('org-test', 'b-agent', 'app-new'),
			('org-test', 'b-claimed-owner', 'app-claimed'),
			('org-test', 'b-target', 'app-target'),
			('org-test', 'b-ambiguous-linked', 'app-new'),
			('org-test', 'b-ambiguous-empty', NULL)`,
		`INSERT INTO org_bot_runtime_state (org_id, bot_id, backend, key, value) VALUES
			('org-test', 'b-agent', 'helix', 'project_id', 'project-agent'),
			('org-test', 'b-agent', 'helix', 'agent_app_id', 'app-legacy'),
			('org-test', 'b-target', 'helix', 'project_id', 'project-claimed'),
			('org-test', 'b-target', 'helix', 'agent_app_id', 'app-legacy-target'),
			('org-test', 'b-ambiguous-linked', 'helix', 'project_id', 'project-ambiguous'),
			('org-test', 'b-ambiguous-empty', 'helix', 'project_id', 'project-ambiguous')`,
		`INSERT INTO projects (id, organization_id, default_helix_app_id, deleted_at)
		 VALUES
			('project-agent', 'org-test', 'app-explicit', NULL),
			('project-claimed', 'org-test', 'app-claimed', NULL),
			('project-ambiguous', 'org-test', '', NULL)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed migration fixture: %v", err)
		}
	}

	if err := backfillProjectAgentApps(db); err != nil {
		t.Fatalf("backfill project agent apps: %v", err)
	}

	var project struct{ DefaultHelixAppID string }
	if err := db.Table("projects").Where("id = ?", "project-agent").First(&project).Error; err != nil {
		t.Fatal(err)
	}
	if project.DefaultHelixAppID != "app-explicit" {
		t.Fatalf("project default = %q, want app-explicit", project.DefaultHelixAppID)
	}
	var bot struct {
		AgentID string `gorm:"column:agent_app_id"`
	}
	if err := db.Table("org_bots").Where("id = ?", "b-agent").First(&bot).Error; err != nil {
		t.Fatal(err)
	}
	if bot.AgentID != "app-explicit" {
		t.Fatalf("bot app = %q, want app-explicit", bot.AgentID)
	}
	var state struct{ Value string }
	if err := db.Table("org_bot_runtime_state").
		Where("bot_id = ? AND key = ?", "b-agent", "agent_app_id").
		First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.Value != "app-explicit" {
		t.Fatalf("runtime app = %q, want app-explicit", state.Value)
	}

	if err := db.Table("projects").Where("id = ?", "project-claimed").First(&project).Error; err != nil {
		t.Fatal(err)
	}
	if project.DefaultHelixAppID != "app-target" {
		t.Fatalf("claimed project default = %q, want app-target", project.DefaultHelixAppID)
	}
	if err := db.Table("org_bots").Where("id = ?", "b-target").First(&bot).Error; err != nil {
		t.Fatal(err)
	}
	if bot.AgentID != "app-target" {
		t.Fatalf("target bot app = %q, want app-target", bot.AgentID)
	}

	if err := db.Table("projects").Where("id = ?", "project-ambiguous").First(&project).Error; err != nil {
		t.Fatal(err)
	}
	if project.DefaultHelixAppID != "" {
		t.Fatalf("ambiguous project default = %q, want unchanged empty default", project.DefaultHelixAppID)
	}
}
