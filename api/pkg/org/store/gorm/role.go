package gorm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/store"
)

// roleRow is the GORM-mapped representation. Tools and Streams are
// JSON-encoded arrays so that adding to the typed manifest never
// needs a column-level schema migration: the slice serialises
// transparently. An empty slice and a missing column both decode to
// nil — equivalent semantics in role.Role.
type roleRow struct {
	ID        string   `gorm:"primaryKey;type:text"`
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

func (r *rolesRepo) Create(ctx context.Context, role role.Role) error {
	if err := r.db.WithContext(ctx).Create(roleToRow(role)).Error; err != nil {
		return fmt.Errorf("create role: %w", err)
	}
	return nil
}

func (r *rolesRepo) Get(ctx context.Context, id role.ID) (role.Role, error) {
	var row roleRow
	err := r.db.WithContext(ctx).First(&row, "id = ?", string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return role.Role{}, fmt.Errorf("role %q: %w", id, store.ErrNotFound)
		}
		return role.Role{}, fmt.Errorf("get role %q: %w", id, err)
	}
	return rowToRole(row), nil
}

func (r *rolesRepo) List(ctx context.Context) ([]role.Role, error) {
	var rows []roleRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	out := make([]role.Role, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToRole(row))
	}
	return out, nil
}

func (r *rolesRepo) Update(ctx context.Context, role role.Role) error {
	row := roleToRow(role)
	res := r.db.WithContext(ctx).Model(&roleRow{}).Where("id = ?", row.ID).Updates(map[string]any{
		"content":    row.Content,
		"tools":      row.Tools,
		"streams":    row.Streams,
		"updated_at": row.UpdatedAt,
	})
	if res.Error != nil {
		return fmt.Errorf("update role: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("role %q: %w", role.ID, store.ErrNotFound)
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
	// Preserve the nil-vs-empty distinction the role.Role API exposes:
	// New() stores nil when no values are passed, so the row should
	// reflect the same. GORM's json serializer encodes nil as NULL and
	// []string{} as `[]` — both decode back appropriately.
	if len(tools) == 0 {
		tools = nil
	}
	if len(streams) == 0 {
		streams = nil
	}
	return roleRow{
		ID:        string(r.ID),
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
		ID:        role.ID(row.ID),
		Content:   row.Content,
		Tools:     tools,
		Streams:   streams,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
