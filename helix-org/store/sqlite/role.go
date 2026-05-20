package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
)

type roleRow struct {
	ID        string `gorm:"primaryKey;type:text"`
	Content   string `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (roleRow) TableName() string { return "roles" }

type rolesRepo struct {
	db *gorm.DB
}

func (r *rolesRepo) Create(ctx context.Context, role domain.Role) error {
	if err := r.db.WithContext(ctx).Create(roleToRow(role)).Error; err != nil {
		return fmt.Errorf("create role: %w", err)
	}
	return nil
}

func (r *rolesRepo) Get(ctx context.Context, id domain.RoleID) (domain.Role, error) {
	var row roleRow
	err := r.db.WithContext(ctx).First(&row, "id = ?", string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Role{}, fmt.Errorf("role %q: %w", id, store.ErrNotFound)
		}
		return domain.Role{}, fmt.Errorf("get role %q: %w", id, err)
	}
	return rowToRole(row), nil
}

func (r *rolesRepo) List(ctx context.Context) ([]domain.Role, error) {
	var rows []roleRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	out := make([]domain.Role, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToRole(row))
	}
	return out, nil
}

func (r *rolesRepo) Update(ctx context.Context, role domain.Role) error {
	row := roleToRow(role)
	res := r.db.WithContext(ctx).Model(&roleRow{}).Where("id = ?", row.ID).Updates(map[string]any{
		"content":    row.Content,
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

func roleToRow(role domain.Role) roleRow {
	return roleRow{
		ID:        string(role.ID),
		Content:   role.Content,
		CreatedAt: role.CreatedAt,
		UpdatedAt: role.UpdatedAt,
	}
}

func rowToRole(row roleRow) domain.Role {
	return domain.Role{
		ID:        domain.RoleID(row.ID),
		Content:   row.Content,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
