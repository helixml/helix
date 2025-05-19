package dashboard

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID    uuid.UUID `json:"id" gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	Email string    `json:"email" gorm:"type:varchar(255);unique;not null"`
	Name  string    `json:"name" gorm:"type:text"`
}

type Organization struct {
	ID   uuid.UUID `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	Name string    `json:"name" gorm:"type:varchar(255)"`
}

type OrgMembership struct {
	UserID         uuid.UUID `json:"user_id" gorm:"type:uuid;primary_key;index:idx_org_memberships_user_id"`
	OrganizationID uuid.UUID `json:"organization_id" gorm:"type:uuid;primary_key;index:idx_org_memberships_organization_id"`

	// Foreign key relationships
	Organization Organization `gorm:"foreignKey:OrganizationID"`
	User         User         `gorm:"foreignKey:UserID"`
}

type MessageType string

const (
	TaskMessageType MessageType = "task"
	TextMessageType MessageType = "text"
)

type MessageItem struct {
	MessageType MessageType `json:"messageType"`
	Value       string      `json:"value"`
}

type MessageContent struct {
	Messages []MessageItem `json:"messages"`
}

// Implement the sql.Scanner interface
func (mc *MessageContent) Scan(value interface{}) error {
	if value == nil {
		*mc = MessageContent{}
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(bytes, mc)
}

// Implement the driver.Valuer interface
func (mc MessageContent) Value() (driver.Value, error) {
	if len(mc.Messages) == 0 {
		return nil, nil
	}
	return json.Marshal(mc)
}

type Conversation struct {
	ID        string    `json:"id" gorm:"type:varchar(21);primary_key"`
	CreatedBy uuid.UUID `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"default:CURRENT_TIMESTAMP"`
	IsActive  bool      `json:"is_active" gorm:"default:true"`
	Error     string    `json:"error"`

	// every user message is associated with an assistant message
	UserMessage      MessageContent `json:"user_message" gorm:"type:jsonb;not null"`
	AssistantMessage MessageContent `json:"assistant_message" gorm:"type:jsonb"`
}

// PostgresStorage implements the dashboard.Storage interface using PostgreSQL with GORM.
type PostgresStorage struct {
	db *gorm.DB
}
