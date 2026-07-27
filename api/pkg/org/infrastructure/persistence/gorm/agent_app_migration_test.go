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
			organization_id TEXT NOT NULL
		)`,
		`INSERT INTO apps (id, organization_id) VALUES
			('app-same-org', 'org-test'),
			('app-other-org', 'org-other')`,
		`INSERT INTO org_bots (org_id, id, kind) VALUES
			('org-test', 'b-same-org', ''),
			('org-test', 'b-other-org', '')`,
		`INSERT INTO org_bot_runtime_state (org_id, bot_id, backend, key, value) VALUES
			('org-test', 'b-same-org', 'helix', 'agent_app_id', 'app-same-org'),
			('org-test', 'b-other-org', 'helix', 'agent_app_id', 'app-other-org')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed migration fixture: %v", err)
		}
	}

	if err := backfillAgentAppLinks(db); err != nil {
		t.Fatalf("backfill agent links: %v", err)
	}

	var sameOrg, otherOrg struct {
		AgentAppID *string
	}
	if err := db.Table("org_bots").Where("id = ?", "b-same-org").First(&sameOrg).Error; err != nil {
		t.Fatalf("read same-org bot: %v", err)
	}
	if err := db.Table("org_bots").Where("id = ?", "b-other-org").First(&otherOrg).Error; err != nil {
		t.Fatalf("read cross-org bot: %v", err)
	}
	if sameOrg.AgentAppID == nil || *sameOrg.AgentAppID != "app-same-org" {
		t.Fatalf("same-org link = %v", sameOrg.AgentAppID)
	}
	if otherOrg.AgentAppID != nil {
		t.Fatalf("cross-org link was backfilled: %q", *otherOrg.AgentAppID)
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
			organization_id TEXT NOT NULL
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
		`INSERT INTO apps (id, organization_id) VALUES
			('app-canonical', 'org-test'),
			('app-cross-org', 'org-other'),
			('app-conflict-a', 'org-test'),
			('app-conflict-b', 'org-test')`,
		`INSERT INTO org_bots (org_id, id, agent_app_id) VALUES
			('org-test', 'b-agent', 'app-canonical'),
			('org-test', 'b-cross-app', 'app-cross-org'),
			('org-test', 'b-cross-project', 'app-canonical'),
			('org-test', 'b-deleted', 'app-canonical'),
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
		BotID   string
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
