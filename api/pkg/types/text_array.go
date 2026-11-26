package types

import (
	"database/sql/driver"

	"github.com/lib/pq"
)

// TextArray is a Postgres text[] column whose GORM tag carries a quoted
// default, e.g. `gorm:"type:text[];default:'{}'"`.
//
// GORM compares a column's catalog default with its tag to decide whether
// AutoMigrate must ALTER it. The postgres driver reports '{}'::text[] as {},
// and GORM strips the quotes from the tag before comparing only for string
// data types. With pq.StringArray the two never match, so every boot issued
// ALTER COLUMN ... SET DEFAULT '{}' under an ACCESS EXCLUSIVE lock. Reporting
// a string data type makes the comparison match; the column type still comes
// from the type tag.
type TextArray []string

// Value implements driver.Valuer.
func (a TextArray) Value() (driver.Value, error) { return pq.StringArray(a).Value() }

// Scan implements sql.Scanner.
func (a *TextArray) Scan(src any) error { return (*pq.StringArray)(a).Scan(src) }

// GormDataType implements schema.GormDataTypeInterface.
func (TextArray) GormDataType() string { return "string" }
