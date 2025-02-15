package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

type Organization struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at"`
	Name        string         `json:"name"`
	DisplayName string         `json:"display_name"`
	Owner       string         `json:"owner"` // Who created the org
}

type Team struct {
}

// this lives in the database
// the ID is the keycloak user ID
// there might not be a record for every user
type UserMeta struct {
	ID     string     `json:"id"`
	Config UserConfig `json:"config" gorm:"type:json"`
}

type UserConfig struct {
	StripeSubscriptionActive bool   `json:"stripe_subscription_active"`
	StripeCustomerID         string `json:"stripe_customer_id"`
	StripeSubscriptionID     string `json:"stripe_subscription_id"`
}

func (u UserConfig) Value() (driver.Value, error) {
	j, err := json.Marshal(u)
	return j, err
}

func (u *UserConfig) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return errors.New("type assertion .([]byte) failed")
	}
	var result UserConfig
	if err := json.Unmarshal(source, &result); err != nil {
		return err
	}
	*u = result
	return nil
}

func (UserConfig) GormDataType() string {
	return "json"
}
