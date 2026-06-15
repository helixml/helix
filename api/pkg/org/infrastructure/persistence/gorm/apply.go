package gorm

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/store"
)

// ApplyOptions threads the Options through db, translating the
// abstract Query into gorm calls in the canonical order
// (WHERE → ORDER → LIMIT). UPDATE handlers consume q.Updates()
// directly via Repository.Update — ApplyOptions ignores it.
//
// Modelled on kodit's database.ApplyOptions
// (github.com/helixml/kodit/internal/database).
func ApplyOptions(db *gorm.DB, opts ...store.Option) *gorm.DB {
	q := store.Build(opts...)
	if t := q.Table(); t != "" {
		db = db.Table(t)
	}
	for _, j := range q.Joins() {
		db = db.Joins(j.SQL, j.Args...)
	}
	if sel := q.Selects(); sel != "" {
		db = db.Select(sel)
	}
	for _, cond := range q.Conditions() {
		if cond.In {
			db = db.Where(fmt.Sprintf("%s IN ?", cond.Field), cond.Value)
		} else {
			db = db.Where(fmt.Sprintf("%s = ?", cond.Field), cond.Value)
		}
	}
	for _, c := range q.Clauses() {
		db = db.Where(c.SQL, c.Args...)
	}
	for _, o := range q.Orders() {
		dir := "ASC"
		if !o.Ascending {
			dir = "DESC"
		}
		db = db.Order(fmt.Sprintf("%s %s", o.Expr, dir))
	}
	if l := q.Limit(); l > 0 {
		db = db.Limit(l)
	}
	return db
}

// ApplyConditions applies only the WHERE clauses — no ORDER, no
// LIMIT. Used by Count / Exists / Delete where the trailing modifiers
// are irrelevant or harmful.
func ApplyConditions(db *gorm.DB, opts ...store.Option) *gorm.DB {
	q := store.Build(opts...)
	for _, cond := range q.Conditions() {
		if cond.In {
			db = db.Where(fmt.Sprintf("%s IN ?", cond.Field), cond.Value)
		} else {
			db = db.Where(fmt.Sprintf("%s = ?", cond.Field), cond.Value)
		}
	}
	for _, c := range q.Clauses() {
		db = db.Where(c.SQL, c.Args...)
	}
	return db
}
