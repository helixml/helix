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
	Streams   []string `gorm:"serializer:json"`
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
	streams := make([]string, 0, len(r.Streams))
	for _, s := range r.Streams {
		streams = append(streams, string(s))
	}
	if len(tools) == 0 {
		tools = nil
	}
	if len(streams) == 0 {
		streams = nil
	}
	return roleRow{
		ID:        string(r.ID),
		OrgID:     r.OrganizationID,
		Content:   r.Content,
		Tools:     tools,
		Streams:   streams,
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
	var streams []streaming.StreamID
	if len(row.Streams) > 0 {
		streams = make([]streaming.StreamID, 0, len(row.Streams))
		for _, s := range row.Streams {
			streams = append(streams, streaming.StreamID(s))
		}
	}
	return orgchart.Role{
		ID:             orgchart.RoleID(row.ID),
		OrganizationID: row.OrgID,
		Content:        row.Content,
		Tools:          tools,
		Streams:        streams,
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
	streamsJSON, err := json.Marshal(row.Streams)
	if err != nil {
		return fmt.Errorf("marshal streams: %w", err)
	}
	return r.Repository.Update(ctx,
		store.WithOrg(row.OrgID),
		store.WithID(row.ID),
		store.WithUpdates(map[string]any{
			"content":    row.Content,
			"tools":      string(toolsJSON),
			"streams":    string(streamsJSON),
			"updated_at": row.UpdatedAt,
		}),
	)
}
