package types

import "time"

// GlobalCounter stores deployment-wide counters like task numbers
// Each counter is identified by a unique name (e.g., "task_number")
type GlobalCounter struct {
	Name      string    `json:"name" gorm:"primaryKey;type:varchar(255)"`
	Value     int       `json:"value" gorm:"default:0"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (GlobalCounter) TableName() string {
	return "global_counters"
}
