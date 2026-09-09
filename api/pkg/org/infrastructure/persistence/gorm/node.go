package gorm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/types"
)

// OrgBot is the persisted model for an org-chart participant. It has a
// composite primary key (id, org_id) so short readable handles
//
// (`b-root`, `b-engineer`) can repeat across helix tenants. OrganizationID
// additionally carries a FK to organizations(id) ON DELETE CASCADE -
// added out-of-band in OpenWithDB because GORM tag-driven FK creation
// to a table owned by another package is fragile.
//
// A Node is the merge of the former Role and Worker: it carries its own
// content + tool list (its capability) and is the live participant in
// the reporting graph. Reporting lines (who reports to whom) are a
// separate many-to-many relation - see reportingLineRow - so a Node
// carries no parent column. LegacyAppID is the remaining compatibility link
// to the Helix App while its instructions and tools are migrated. Execution
// configuration is already owned by this model through CodeAgentConfig.
type OrgBot struct {
	ID              string                          `json:"id" gorm:"column:id;primaryKey;type:text"`
	OrganizationID  string                          `json:"organization_id" gorm:"column:org_id;primaryKey;type:text;index"`
	LegacyAppID     *string                         `json:"legacy_app_id,omitempty" gorm:"column:agent_app_id;type:text;index"`
	CodeAgentConfig *types.CodeAgentExecutionConfig `json:"code_agent_config,omitempty" gorm:"column:code_agent_config;type:jsonb;serializer:json"`
	Name            string                          `json:"name" gorm:"column:name;not null;default:''"`
	Content         string                          `json:"content" gorm:"column:content;not null"`
	Tools           []string                        `json:"tools,omitempty" gorm:"column:tools;serializer:json"`
	ProjectIDs      []string                        `json:"project_ids,omitempty" gorm:"column:project_ids;serializer:json"`
	PreserveContext bool                            `json:"preserve_context" gorm:"column:preserve_context;not null;default:false"`
	SandboxRuntime  string                          `json:"sandbox_runtime,omitempty" gorm:"column:sandbox_runtime;not null;default:''"`
	SandboxVCPUs    int                             `json:"sandbox_vcpus,omitempty" gorm:"column:sandbox_vcpus;not null;default:0"`
	SandboxMemoryMB int                             `json:"sandbox_memory_mb,omitempty" gorm:"column:sandbox_memory_mb;not null;default:0"`
	CreatedAt       time.Time                       `json:"created_at" gorm:"column:created_at"`
	UpdatedAt       time.Time                       `json:"updated_at" gorm:"column:updated_at"`
}

// TableName keeps the persisted model on the established org_bots table.
func (OrgBot) TableName() string { return "org_bots" }

type nodeMapper struct{}

func (nodeMapper) ToRow(node orgchart.Node) (OrgBot, error) {
	tools := make([]string, 0, len(node.Tools))
	for _, t := range node.Tools {
		tools = append(tools, string(t))
	}
	if len(tools) == 0 {
		tools = nil
	}
	var agentID *string
	if node.AgentID != "" {
		agentID = &node.AgentID
	}
	return OrgBot{
		ID:              string(node.ID),
		OrganizationID:  node.OrganizationID,
		LegacyAppID:     agentID,
		CodeAgentConfig: node.CodeAgentConfig,
		Name:            node.Name,
		Content:         node.Content,
		Tools:           tools,
		ProjectIDs:      node.ProjectIDs,
		PreserveContext: node.PreserveContext,
		SandboxRuntime:  node.SandboxRuntime,
		SandboxVCPUs:    node.SandboxVCPUs,
		SandboxMemoryMB: node.SandboxMemoryMB,
		CreatedAt:       node.CreatedAt,
		UpdatedAt:       node.UpdatedAt,
	}, nil
}

func (nodeMapper) ToDomain(row OrgBot) (orgchart.Node, error) {
	var tools []tool.Name
	if len(row.Tools) > 0 {
		tools = make([]tool.Name, 0, len(row.Tools))
		for _, t := range row.Tools {
			tools = append(tools, tool.Name(t))
		}
	}
	var agentID string
	if row.LegacyAppID != nil {
		agentID = *row.LegacyAppID
	}
	return orgchart.Node{
		ID:              orgchart.NodeID(row.ID),
		OrganizationID:  row.OrganizationID,
		AgentID:         agentID,
		CodeAgentConfig: row.CodeAgentConfig,
		Name:            row.Name,
		Content:         row.Content,
		Tools:           tools,
		ProjectIDs:      row.ProjectIDs,
		PreserveContext: row.PreserveContext,
		SandboxRuntime:  row.SandboxRuntime,
		SandboxVCPUs:    row.SandboxVCPUs,
		SandboxMemoryMB: row.SandboxMemoryMB,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}, nil
}

type nodesRepo struct {
	*Repository[orgchart.Node, OrgBot]
	db *gorm.DB
}

func newNodesRepo(db *gorm.DB) *nodesRepo {
	return &nodesRepo{
		Repository: NewRepository[orgchart.Node, OrgBot](db, nodeMapper{}, "node"),
		db:         db,
	}
}

func (r *nodesRepo) Create(ctx context.Context, node orgchart.Node) error {
	if node.AgentID == "" {
		return errors.New("create node: agent app id is required")
	}
	return r.Repository.Create(ctx, node)
}

func (r *nodesRepo) Get(ctx context.Context, orgID string, id orgchart.NodeID) (orgchart.Node, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *nodesRepo) List(ctx context.Context, orgID string) ([]orgchart.Node, error) {
	return r.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("id"))
}

func (r *nodesRepo) Update(ctx context.Context, node orgchart.Node) error {
	row, err := nodeMapper{}.ToRow(node)
	if err != nil {
		return fmt.Errorf("map node: %w", err)
	}
	// Pre-marshal JSON columns so the Updates() map carries typed
	// string literals; gorm's serializer:json tag works on full-row
	// Save but not on a map[string]any Updates — pgx can't infer the
	// column type from a bare []string parameter.
	toolsJSON, err := json.Marshal(row.Tools)
	if err != nil {
		return fmt.Errorf("marshal tools: %w", err)
	}
	projectIDsJSON, err := json.Marshal(row.ProjectIDs)
	if err != nil {
		return fmt.Errorf("marshal project ids: %w", err)
	}
	codeAgentConfigJSON, err := json.Marshal(row.CodeAgentConfig)
	if err != nil {
		return fmt.Errorf("marshal code agent config: %w", err)
	}
	return r.Repository.Update(ctx,
		store.WithOrg(row.OrganizationID),
		store.WithID(row.ID),
		store.WithUpdates(map[string]any{
			"name":              row.Name,
			"agent_app_id":      row.LegacyAppID,
			"code_agent_config": string(codeAgentConfigJSON),
			"content":           row.Content,
			"tools":             string(toolsJSON),
			"project_ids":       string(projectIDsJSON),
			"preserve_context":  row.PreserveContext,
			// Sandbox config columns. This map is the complete column list an
			// Update writes; a field mapped in ToRow but missing here is
			// silently dropped on every PATCH.
			"sandbox_runtime":   row.SandboxRuntime,
			"sandbox_vcpus":     row.SandboxVCPUs,
			"sandbox_memory_mb": row.SandboxMemoryMB,
			"updated_at":        row.UpdatedAt,
		}),
	)
}

func (r *nodesRepo) ClaimLegacyApp(ctx context.Context, orgID string, id orgchart.NodeID, appID string, config *types.CodeAgentExecutionConfig) (bool, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return false, fmt.Errorf("marshal code agent config: %w", err)
	}
	res := r.db.WithContext(ctx).
		Model(&OrgBot{}).
		Where("org_id = ? AND id = ? AND agent_app_id IS NULL", orgID, string(id)).
		Updates(map[string]any{"agent_app_id": appID, "code_agent_config": string(configJSON)})
	if res.Error != nil {
		return false, fmt.Errorf("claim legacy app: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}

// Delete removes the node row. Its node-anchored attachments and the
// reporting lines that reference this node (as manager or report) are
// removed by the ON DELETE CASCADE foreign keys on
// org_worker_attachments and org_reporting_lines (installed in
// OpenWithDB), so no app code clears them - that's the whole point of
// the association tables.
func (r *nodesRepo) Delete(ctx context.Context, orgID string, id orgchart.NodeID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("org_id = ? AND id = ?", orgID, string(id)).Delete(&OrgBot{})
		if res.Error != nil {
			return fmt.Errorf("delete node: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("node: %w", store.ErrNotFound)
		}
		return nil
	})
}
