package data

import (
	"time"
)

type LicenseKey struct {
	ID         int       `db:"id" json:"id"`
	LicenseKey string    `db:"license_key" json:"license_key"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time `db:"updated_at" json:"updated_at"`
}
