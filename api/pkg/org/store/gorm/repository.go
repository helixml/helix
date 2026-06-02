package gorm

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/store"
)

// Mapper converts between a domain entity D and its gorm row R.
// Per-entity files declare a mapper struct that implements this and
// thread it into a Repository. Modelled on kodit's
// internal/database.EntityMapper.
type Mapper[D any, R any] interface {
	ToRow(D) (R, error)
	ToDomain(R) (D, error)
}

// Repository is the generic gorm persistence primitive every
// per-entity store wraps. The per-entity wrappers keep the public
// store interface (Get / List / Create / Update / Delete) for
// readability at call sites; this Repository removes the gorm
// boilerplate underneath.
//
// Type parameters:
//   D — the domain entity (role.Role, position.Position, …)
//   R — the gorm row (roleRow, positionRow, …)
//
// label is woven into error messages so they read naturally
// ("create role: …" rather than "create entity: …").
type Repository[D any, R any] struct {
	db     *gorm.DB
	mapper Mapper[D, R]
	label  string
}

// NewRepository wires a Repository against a *gorm.DB and a mapper.
func NewRepository[D any, R any](db *gorm.DB, mapper Mapper[D, R], label string) *Repository[D, R] {
	return &Repository[D, R]{db: db, mapper: mapper, label: label}
}

// Create inserts a row built from the domain entity.
func (r *Repository[D, R]) Create(ctx context.Context, d D) error {
	row, err := r.mapper.ToRow(d)
	if err != nil {
		return fmt.Errorf("map %s for create: %w", r.label, err)
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create %s: %w", r.label, err)
	}
	return nil
}

// FindOne returns the first matching row mapped back to a domain
// entity. Returns store.ErrNotFound (wrapped) when no row matches.
func (r *Repository[D, R]) FindOne(ctx context.Context, opts ...store.Option) (D, error) {
	var row R
	var zero D
	db := ApplyOptions(r.db.WithContext(ctx).Model(new(R)), opts...)
	if err := db.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return zero, fmt.Errorf("%s: %w", r.label, store.ErrNotFound)
		}
		return zero, fmt.Errorf("find %s: %w", r.label, err)
	}
	d, err := r.mapper.ToDomain(row)
	if err != nil {
		return zero, fmt.Errorf("map %s: %w", r.label, err)
	}
	return d, nil
}

// Find returns all matching rows mapped to domain entities. Empty
// result is not an error.
func (r *Repository[D, R]) Find(ctx context.Context, opts ...store.Option) ([]D, error) {
	var rows []R
	db := ApplyOptions(r.db.WithContext(ctx).Model(new(R)), opts...)
	if err := db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list %s: %w", r.label, err)
	}
	out := make([]D, 0, len(rows))
	for _, row := range rows {
		d, err := r.mapper.ToDomain(row)
		if err != nil {
			return nil, fmt.Errorf("map %s: %w", r.label, err)
		}
		out = append(out, d)
	}
	return out, nil
}

// Save replaces the row identified by the domain entity's primary
// key (gorm Save). Use when the entity carries enough state to
// rebuild the row; otherwise reach for Update with explicit columns.
func (r *Repository[D, R]) Save(ctx context.Context, d D) error {
	row, err := r.mapper.ToRow(d)
	if err != nil {
		return fmt.Errorf("map %s for save: %w", r.label, err)
	}
	if err := r.db.WithContext(ctx).Save(&row).Error; err != nil {
		return fmt.Errorf("save %s: %w", r.label, err)
	}
	return nil
}

// Update writes a set of column updates against rows matching opts.
// Returns store.ErrNotFound when no rows match — the caller meant to
// update a specific row.
func (r *Repository[D, R]) Update(ctx context.Context, opts ...store.Option) error {
	q := store.Build(opts...)
	updates := q.Updates()
	if len(updates) == 0 {
		return fmt.Errorf("update %s: no updates supplied", r.label)
	}
	db := ApplyConditions(r.db.WithContext(ctx).Model(new(R)), opts...)
	res := db.Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("update %s: %w", r.label, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", r.label, store.ErrNotFound)
	}
	return nil
}

// Delete removes rows matching opts. Refuses to run with no
// conditions — an unscoped DELETE would wipe the whole table.
// Returns store.ErrNotFound when no rows match.
func (r *Repository[D, R]) Delete(ctx context.Context, opts ...store.Option) error {
	q := store.Build(opts...)
	if len(q.Conditions()) == 0 && len(q.Clauses()) == 0 {
		return fmt.Errorf("delete %s: refused — no conditions supplied", r.label)
	}
	db := ApplyConditions(r.db.WithContext(ctx), opts...)
	res := db.Delete(new(R))
	if res.Error != nil {
		return fmt.Errorf("delete %s: %w", r.label, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", r.label, store.ErrNotFound)
	}
	return nil
}

// Exists reports whether any row matches the given filters.
func (r *Repository[D, R]) Exists(ctx context.Context, opts ...store.Option) (bool, error) {
	var count int64
	db := ApplyConditions(r.db.WithContext(ctx).Model(new(R)), opts...)
	if err := db.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check %s exists: %w", r.label, err)
	}
	return count > 0, nil
}
