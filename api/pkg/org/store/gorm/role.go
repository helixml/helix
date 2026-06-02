package gorm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/tool"
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

type rolesRepo struct {
	db *gorm.DB
}

func (r *rolesRepo) Create(ctx context.Context, ro role.Role) error {
	if err := r.db.WithContext(ctx).Create(roleToRow(ro)).Error; err != nil {
		return fmt.Errorf("create role: %w", err)
	}
	return nil
}

func (r *rolesRepo) Get(ctx context.Context, orgID string, id role.ID) (role.Role, error) {
	var row roleRow
	err := r.db.WithContext(ctx).First(&row, "org_id = ? AND id = ?", orgID, string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return role.Role{}, fmt.Errorf("role %q in org %q: %w", id, orgID, store.ErrNotFound)
		}
		return role.Role{}, fmt.Errorf("get role %q in org %q: %w", id, orgID, err)
	}
	return rowToRole(row), nil
}

func (r *rolesRepo) List(ctx context.Context, orgID string) ([]role.Role, error) {
	var rows []roleRow
	if err := r.db.WithContext(ctx).Where("org_id = ?", orgID).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list roles in org %q: %w", orgID, err)
	}
	out := make([]role.Role, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToRole(row))
	}
	return out, nil
}

func (r *rolesRepo) Update(ctx context.Context, ro role.Role) error {
	row := roleToRow(ro)
	// Pre-marshal the JSON columns so Postgres sees a typed string
	// literal. gorm's serializer:json tag works on full-row Save but
	// not on map[string]any Updates — passing the raw []string there
	// produces "could not determine data type of parameter" because
	// the pgx driver can't infer the column type from a generic any.
	toolsJSON, err := json.Marshal(row.Tools)
	if err != nil {
		return fmt.Errorf("marshal tools: %w", err)
	}
	streamsJSON, err := json.Marshal(row.Streams)
	if err != nil {
		return fmt.Errorf("marshal streams: %w", err)
	}
	res := r.db.WithContext(ctx).Model(&roleRow{}).Where("org_id = ? AND id = ?", row.OrgID, row.ID).Updates(map[string]any{
		"content":    row.Content,
		"tools":      string(toolsJSON),
		"streams":    string(streamsJSON),
		"updated_at": row.UpdatedAt,
	})
	if res.Error != nil {
		return fmt.Errorf("update role: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("role %q in org %q: %w", ro.ID, ro.OrganizationID, store.ErrNotFound)
	}
	return nil
}

func roleToRow(r role.Role) roleRow {
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
	}
}

func rowToRole(row roleRow) role.Role {
	var tools []tool.Name
	if len(row.Tools) > 0 {
		tools = make([]tool.Name, 0, len(row.Tools))
		for _, t := range row.Tools {
			tools = append(tools, tool.Name(t))
		}
	}
	var streams []stream.ID
	if len(row.Streams) > 0 {
		streams = make([]stream.ID, 0, len(row.Streams))
		for _, s := range row.Streams {
			streams = append(streams, stream.ID(s))
		}
	}
	return role.Role{
		ID:             role.ID(row.ID),
		OrganizationID: row.OrgID,
		Content:        row.Content,
		Tools:          tools,
		Streams:        streams,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}
