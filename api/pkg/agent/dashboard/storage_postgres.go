package dashboard

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// NewPostgresStorage creates a new PostgresStorage instance with the provided connection string.
// It initializes the database schema if it doesn't exist.
func NewPostgresStorage(connStr string) (*PostgresStorage, error) {
	db, err := gorm.Open(postgres.Open(connStr), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	storage := &PostgresStorage{db: db}
	return storage, nil
}

// initDB creates the necessary tables if they don't exist.
func (s *PostgresStorage) InitDB() error {
	// Automigrate will create/update tables based on the model structs
	return s.db.AutoMigrate(&User{}, &OrgMembership{}, &Conversation{})
}

// Close closes the database connection.
func (s *PostgresStorage) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// GetDashboardUsers retrieves a paginated list of users for the
func (p *PostgresStorage) GetDashboardUsers(limit int, offset int) ([]DashboardUser, error) {
	// Default values for pagination
	if limit < 1 {
		limit = 10 // Default limit
	}
	if offset < 0 {
		offset = 0 // Default offset
	}

	// First, get unique user IDs from the most recent conversations
	type UserIDResult struct {
		CreatedBy    string
		MaxCreatedAt string `gorm:"column:max_created_at"`
	}
	var userIDs []UserIDResult

	// Query to get unique user IDs from conversations ordered by most recent
	if err := p.db.Model(&Conversation{}).
		Select("DISTINCT created_by, MAX(created_at) as max_created_at").
		Group("created_by").
		Order("max_created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&userIDs).Error; err != nil {
		return nil, err
	}

	if len(userIDs) == 0 {
		return []DashboardUser{}, nil
	}

	// Extract the user IDs into a slice
	var userIDStrings []string
	for _, uidResult := range userIDs {
		userIDStrings = append(userIDStrings, uidResult.CreatedBy)
	}

	// Define a struct to hold the joined query result
	type UserOrgResult struct {
		UserID           string
		UserName         string
		UserEmail        string
		OrganizationID   string
		OrganizationName string
	}

	// Create a slice to hold our results
	var results []UserOrgResult

	// Query users with left join to org_memberships and organizations
	// This allows us to get organization names for users who have memberships
	query := p.db.Table("users").
		Select("users.id as user_id, users.name as user_name, users.email as user_email, "+
			"organizations.id as organization_id, organizations.name as organization_name").
		Joins("LEFT JOIN org_memberships ON users.id = org_memberships.user_id").
		Joins("LEFT JOIN organizations ON org_memberships.organization_id = organizations.id").
		Where("users.id IN ?", userIDStrings)

	if err := query.Scan(&results).Error; err != nil {
		return nil, err
	}

	// Create a map to organize users with their organizations
	// Since a user can belong to multiple organizations, we'll use the first one we find
	userOrgMap := make(map[string]UserOrgResult)
	for _, result := range results {
		// Only add the user if they're not already in the map or if they have an organization
		// when the existing entry doesn't
		_, exists := userOrgMap[result.UserID]
		if !exists || (userOrgMap[result.UserID].OrganizationName == "" && result.OrganizationName != "") {
			userOrgMap[result.UserID] = result
		}
	}

	// Build the result list in the same order as the original conversation query
	dashboardUsers := make([]DashboardUser, 0, len(userIDs))
	for _, uidResult := range userIDs {
		if userOrg, ok := userOrgMap[uidResult.CreatedBy]; ok {
			// Get username, or extract from email if empty
			username := userOrg.UserName
			if username == "" && userOrg.UserEmail != "" {
				// Split email by @ and use first part as username
				parts := strings.Split(userOrg.UserEmail, "@")
				if len(parts) > 0 {
					username = parts[0]
				}
			}

			dashboardUser := DashboardUser{
				ID:               userOrg.UserID,
				Name:             username,
				Email:            userOrg.UserEmail,
				OrganizationName: userOrg.OrganizationName,
			}
			dashboardUsers = append(dashboardUsers, dashboardUser)
		}
	}

	return dashboardUsers, nil
}

// GetDashboardConversations retrieves conversations for a specific user with pagination for the
func (p *PostgresStorage) GetDashboardConversations(userID string, limit int, offset int) ([]DashboardConversation, error) {
	// Default values for pagination
	if limit < 1 {
		limit = 10 // Default limit
	}
	if offset < 0 {
		offset = 0 // Default offset
	}

	// Convert userID string to UUID
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}

	// First, get the total count of conversations for this user
	var count int64
	if err := p.db.Model(&Conversation{}).Where("created_by = ?", userUUID).Count(&count).Error; err != nil {
		return nil, err
	}

	// Calculate the effective offset from the end
	// If offset is greater than count, we'll return empty results
	effectiveOffset := 0
	if int64(offset) < count {
		effectiveOffset = int(count) - int(offset) - limit
	}

	// Ensure effectiveOffset is not negative
	if effectiveOffset < 0 {
		// Adjust limit if there are fewer conversations than requested after offset
		limit += effectiveOffset // Adding because effectiveOffset is negative
		effectiveOffset = 0
	}

	// Get conversations for the user - using ASC order to get older messages first
	// But skip the correct number from the beginning based on our calculated offset
	var conversations []Conversation
	if err := p.db.Where("created_by = ?", userUUID).Order("created_at ASC").Limit(limit).Offset(effectiveOffset).Find(&conversations).Error; err != nil {
		return nil, err
	}

	dashboardConversations := make([]DashboardConversation, 0, len(conversations))
	for _, conv := range conversations {
		// Extract user message and assistant message
		userMsg := ""
		assistantMsg := ""

		// Extract the text values from MessageContent
		if len(conv.UserMessage.Messages) > 0 {
			for _, msg := range conv.UserMessage.Messages {
				if msg.MessageType == TextMessageType {
					userMsg += msg.Value
				}
			}
		}

		if len(conv.AssistantMessage.Messages) > 0 {
			for _, msg := range conv.AssistantMessage.Messages {
				if msg.MessageType == TextMessageType {
					assistantMsg += msg.Value
				}
			}
		}

		// Create dashboard conversation object
		dashboardConv := DashboardConversation{
			SessionID:            conv.ID,
			UserMessage:          userMsg,
			UserMessageTime:      conv.CreatedAt,
			AssistantMessage:     assistantMsg,
			AssistantMessageTime: conv.CreatedAt,
		}
		dashboardConversations = append(dashboardConversations, dashboardConv)
	}

	return dashboardConversations, nil
}
