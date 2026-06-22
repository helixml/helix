package gorm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

type roleRow struct {
	ID        string   `gorm:"primaryKey;type:text"`
	OrgID     string   `gorm:"primaryKey;type:text;index"`
	Content   string   `gorm:"not null"`
	Tools     []string `gorm:"serializer:json"`
	Topics   []string `gorm:"serializer:json"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (roleRow) TableName() string { return "org_roles" }

type roleMapper struct{}

func (roleMapper) ToRow(r orgchart.Role) (roleRow, error) {
	tools := make([]string, 0, len(r.Tools))
	for _, t := range r.Tools {
		tools = append(tools, string(t))
	}
	topics := make([]string, 0, len(r.Topics))
	for _, s := range r.Topics {
		topics = append(topics, string(s))
	}
	if len(tools) == 0 {
		tools = nil
	}
	if len(topics) == 0 {
		topics = nil
	}
	return roleRow{
		ID:        string(r.ID),
		OrgID:     r.OrganizationID,
		Content:   r.Content,
		Tools:     tools,
		Topics:   topics,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}, nil
}

func (roleMapper) ToDomain(row roleRow) (orgchart.Role, error) {
	var tools []tool.Name
	if len(row.Tools) > 0 {
		tools = make([]tool.Name, 0, len(row.Tools))
		for _, t := range row.Tools {
			tools = append(tools, tool.Name(t))
		}
	}
	var topics []streaming.TopicID
	if len(row.Topics) > 0 {
		topics = make([]streaming.TopicID, 0, len(row.Topics))
		for _, s := range row.Topics {
			topics = append(topics, streaming.TopicID(s))
		}
	}
	return orgchart.Role{
		ID:             orgchart.RoleID(row.ID),
		OrganizationID: row.OrgID,
		Content:        row.Content,
		Tools:          tools,
		Topics:        topics,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

type rolesRepo struct {
	*Repository[orgchart.Role, roleRow]
}

func newRolesRepo(db *gorm.DB) *rolesRepo {
	return &rolesRepo{Repository: NewRepository[orgchart.Role, roleRow](db, roleMapper{}, "role")}
}

func (r *rolesRepo) Get(ctx context.Context, orgID string, id orgchart.RoleID) (orgchart.Role, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *rolesRepo) List(ctx context.Context, orgID string) ([]orgchart.Role, error) {
	return r.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("id"))
}

func (r *rolesRepo) Update(ctx context.Context, ro orgchart.Role) error {
	row, err := roleMapper{}.ToRow(ro)
	if err != nil {
		return fmt.Errorf("map role: %w", err)
	}
	// Pre-marshal JSON columns so the Updates() map carries typed
	// string literals; gorm's serializer:json tag works on full-row
	// Save but not on a map[string]any Updates — pgx can't infer the
	// column type from a bare []string parameter.
	toolsJSON, err := json.Marshal(row.Tools)
	if err != nil {
		return fmt.Errorf("marshal tools: %w", err)
	}
	topicsJSON, err := json.Marshal(row.Topics)
	if err != nil {
		return fmt.Errorf("marshal topics: %w", err)
	}
	return r.Repository.Update(ctx,
		store.WithOrg(row.OrgID),
		store.WithID(row.ID),
		store.WithUpdates(map[string]any{
			"content":    row.Content,
			"tools":      string(toolsJSON),
			"topics":    string(topicsJSON),
			"updated_at": row.UpdatedAt,
		}),
	)
}

// Delete removes one Role row. Cascading to positions + workers is
// the lifecycle service's responsibility, not the store layer.
func (r *rolesRepo) Delete(ctx context.Context, orgID string, id orgchart.RoleID) error {
	return r.Repository.Delete(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}
