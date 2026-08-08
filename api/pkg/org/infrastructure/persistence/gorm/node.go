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
)

//	nodeRow has composite PK (id, org_id) so short readable handles
//
// (`b-root`, `b-engineer`) can repeat across helix tenants. OrgID
// additionally carries a FK to organizations(id) ON DELETE CASCADE -
// added out-of-band in OpenWithDB because GORM tag-driven FK creation
// to a table owned by another package is fragile.
//
// A Node is the merge of the former Role and Worker: it carries its own
// content + tool list (its capability) and is the live participant in
// the reporting graph. Reporting lines (who reports to whom) are a
// separate many-to-many relation - see reportingLineRow - so a Node
// carries no parent column.
type nodeRow struct {
	ID              string   `gorm:"primaryKey;type:text"`
	OrgID           string   `gorm:"primaryKey;type:text;index"`
	AgentID         *string  `gorm:"column:agent_app_id;type:text;index"`
	Name            string   `gorm:"not null;default:''"`
	Content         string   `gorm:"not null"`
	Tools           []string `gorm:"serializer:json"`
	ProjectIDs      []string `gorm:"serializer:json"`
	PreserveContext bool     `gorm:"not null;default:false"`
	// Kind is "" (agent) or "human". HelixUserID / Identity are only
	// populated for human placeholder rows.
	Kind        string            `gorm:"not null;default:'';index"`
	HelixUserID string            `gorm:"not null;default:''"`
	Identity    map[string]string `gorm:"serializer:json"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// org_bots is the legacy physical name retained for existing Node rows.
func (nodeRow) TableName() string { return "org_bots" }

type nodeMapper struct{}

func (nodeMapper) ToRow(node orgchart.Node) (nodeRow, error) {
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
	return nodeRow{
		ID:              string(node.ID),
		OrgID:           node.OrganizationID,
		AgentID:         agentID,
		Name:            node.Name,
		Content:         node.Content,
		Tools:           tools,
		ProjectIDs:      node.ProjectIDs,
		PreserveContext: node.PreserveContext,
		Kind:            node.Kind,
		HelixUserID:     node.HelixUserID,
		Identity:        node.Identity,
		CreatedAt:       node.CreatedAt,
		UpdatedAt:       node.UpdatedAt,
	}, nil
}

	func (nodeMapper) ToDomain(row nodeRow) (orgchart.Node, error) {
	var tools []tool.Name
	if len(row.Tools) > 0 {
		tools = make([]tool.Name, 0, len(row.Tools))
		for _, t := range row.Tools {
			tools = append(tools, tool.Name(t))
		}
	}
	var agentID string
	if row.AgentID != nil {
		agentID = *row.AgentID
	}
	return orgchart.Node{
		ID:              orgchart.NodeID(row.ID),
		OrganizationID:  row.OrgID,
		AgentID:         agentID,
		Name:            row.Name,
		Content:         row.Content,
		Tools:           tools,
		ProjectIDs:      row.ProjectIDs,
		PreserveContext: row.PreserveContext,
		Kind:            row.Kind,
		HelixUserID:     row.HelixUserID,
		Identity:        row.Identity,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}, nil
}

type nodesRepo struct {
	*Repository[orgchart.Node, nodeRow]
	db *gorm.DB
}

func newNodesRepo(db *gorm.DB) *nodesRepo {
	return &nodesRepo{
		Repository: NewRepository[orgchart.Node, nodeRow](db, nodeMapper{}, "node"),
		db:         db,
	}
}

func (r *nodesRepo) Create(ctx context.Context, node orgchart.Node) error {
	if node.IsHuman() {
		if node.AgentID != "" {
			return errors.New("create node: human node cannot reference an agent app")
		}
	} else if node.AgentID == "" {
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
	// Pre-marshal identity for the same reason as tools: the serializer:json
	// tag does not apply on a map[string]any Updates, so pgx can't infer the
	// jsonb column type from a bare map[string]string parameter.
	identityJSON, err := json.Marshal(row.Identity)
	if err != nil {
		return fmt.Errorf("marshal identity: %w", err)
	}
	return r.Repository.Update(ctx,
		store.WithOrg(row.OrgID),
		store.WithID(row.ID),
		store.WithUpdates(map[string]any{
			"name":             row.Name,
			"agent_app_id":     row.AgentID,
			"content":          row.Content,
			"tools":            string(toolsJSON),
			"project_ids":      string(projectIDsJSON),
			"preserve_context": row.PreserveContext,
			"kind":             row.Kind,
			"helix_user_id":    row.HelixUserID,
			"identity":         string(identityJSON),
			"updated_at":       row.UpdatedAt,
		}),
	)
}

func (r *nodesRepo) ClaimAgentApp(ctx context.Context, orgID string, id orgchart.NodeID, appID string) (bool, error) {
	res := r.db.WithContext(ctx).
		Model(&nodeRow{}).
		Where("org_id = ? AND id = ? AND agent_app_id IS NULL", orgID, string(id)).
		Update("agent_app_id", appID)
	if res.Error != nil {
		return false, fmt.Errorf("claim agent app: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}

// Delete removes the node row and drops its node-anchored subscriptions
// in the same transaction. The reporting lines that reference this node
// (as manager or report) are removed by the ON DELETE CASCADE foreign
// keys on org_reporting_lines (installed in OpenWithDB), so no app code
// clears them - that's the whole point of the association table.
func (r *nodesRepo) Delete(ctx context.Context, orgID string, id orgchart.NodeID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("org_id = ? AND bot_id = ?", orgID, string(id)).
			Delete(&subscriptionRow{}).Error; err != nil {
			return fmt.Errorf("delete node: drop subscriptions: %w", err)
		}
		res := tx.Where("org_id = ? AND id = ?", orgID, string(id)).Delete(&nodeRow{})
		if res.Error != nil {
			return fmt.Errorf("delete node: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("node: %w", store.ErrNotFound)
		}
		return nil
	})
}
