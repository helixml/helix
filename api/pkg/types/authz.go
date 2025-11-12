package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

/*

- Organization is the top level entity in the hierarchy.
- Users join the organization through OrganizationMembership and are assigned a role, either owner or member.
- Owners can create teams within organization.
- Teams can have multiple members and multiple roles (roles provide permissions to resources)
- Members of a team example:
	1. User1 has Read role - can see and access most of the resources
	2. User2 has Write role - can see and access most of the resources, update and delete apps
	3. User3 has Admin role - can see and access all resources, invite new members

- Users grant access to Apps using AccessGrant. You can create many instances of AccessGrant for multiple
  users and teams. Each instance can have different roles.
*/

type Organization struct {
	ID          string                   `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
	DeletedAt   gorm.DeletedAt           `gorm:"index" json:"deleted_at"`
	Name        string                   `json:"name" gorm:"uniqueIndex"`
	DisplayName string                   `json:"display_name"`
	Owner       string                   `json:"owner"`                                                           // Who created the org
	Teams       []Team                   `json:"teams" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`       // Teams in the organization
	Memberships []OrganizationMembership `json:"memberships" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"` // Memberships in the organization
	Roles       []Role                   `json:"roles" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`       // Roles in the organization
}

type Team struct {
	ID             string           `gorm:"primaryKey" json:"id"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	DeletedAt      gorm.DeletedAt   `gorm:"index" json:"deleted_at"`
	OrganizationID string           `json:"organization_id" gorm:"index"`
	Name           string           `json:"name"`
	Memberships    []TeamMembership `json:"memberships" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"` // Memberships in the team
}

// OrganizationMembership - organization membership is simple, once added, the user is either an owner or a member
type OrganizationMembership struct {
	UserID         string `json:"user_id" yaml:"user_id" gorm:"primaryKey"` // composite key
	OrganizationID string `json:"organization_id" yaml:"organization_id" gorm:"primaryKey"`

	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`

	// Role - the role of the user in the organization (owner or member)
	Role OrganizationRole `json:"role" yaml:"role"`
	User User             `json:"user" yaml:"user"`
}

type AddOrganizationMemberRequest struct {
	UserReference string           `json:"user_reference"` // Either user ID or user email
	Role          OrganizationRole `json:"role"`
}

type AddTeamMemberRequest struct {
	UserReference string `json:"user_reference"` // Either user ID or user email
}

type UpdateOrganizationMemberRequest struct {
	Role OrganizationRole `json:"role"`
}

type OrganizationRole string

const (
	OrganizationRoleOwner  OrganizationRole = "owner"  // Has full administrative access to the entire organization.
	OrganizationRoleMember OrganizationRole = "member" // Can see every member and team in the organization and can create new apps
)

// Role - a role is a collection of permissions that can be assigned to a user or team.
// Roles are defined within an organization and can be used across teams.
type Role struct {
	ID             string    `json:"id" yaml:"id" gorm:"primaryKey"`
	CreatedAt      time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" yaml:"updated_at"`
	OrganizationID string    `json:"organization_id" yaml:"organization_id" gorm:"index"`
	Name           string    `json:"name" yaml:"name"`
	Description    string    `json:"description" yaml:"description"`
	Config         Config    `json:"config" yaml:"config"`
}

type TeamMembership struct {
	UserID string `json:"user_id" yaml:"user_id" gorm:"primaryKey"` // composite key
	TeamID string `json:"team_id" yaml:"team_id" gorm:"primaryKey"`

	OrganizationID string `json:"organization_id" yaml:"organization_id" gorm:"index"`

	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`

	// extra data fields (optional)
	User User `json:"user,omitempty" yaml:"user,omitempty"`
	Team Team `json:"team,omitempty" yaml:"team,omitempty"`
}

type AuthProvider string

const (
	AuthProviderRegular  AuthProvider = "regular" // Embedded in Helix, no external dependencies
	AuthProviderKeycloak AuthProvider = "keycloak"
	AuthProviderOIDC     AuthProvider = "oidc"
	// TODO: oauth github, google, etc
)

type User struct {
	ID        string         `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	// the actual token used and its type
	Token string `json:"token"`
	// none, runner. keycloak, api_key
	TokenType TokenType `json:"token_type"`
	// if the ID of the user is contained in the env setting
	Admin bool `json:"admin"`
	// if the token is associated with an app
	AppID string `json:"app_id"`
	// these are set by the keycloak user based on the token
	// if it's an app token - the keycloak user is loaded from the owner of the app
	// if it's a runner token - these values will be empty
	Type     OwnerType `json:"type"`
	Email    string    `json:"email"`
	Username string    `json:"username"`
	FullName string    `json:"full_name"`

	AuthProvider AuthProvider `json:"auth_provider"`

	Password           string `json:"-" gorm:"-"`           // Temporary field for password input, not persisted
	PasswordHash       []byte `json:"password_hash"`        // bcrypt hash of the password
	MustChangePassword bool   `json:"must_change_password"` // if the user must change their password

	SB          bool `json:"sb"`
	Deactivated bool `json:"deactivated"`
}

type UserSearchResponse struct {
	Users      []*User `json:"users"`
	TotalCount int64   `json:"total_count"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
}

type UserTokenUsageResponse struct {
	QuotasEnabled   bool    `json:"quotas_enabled"`
	MonthlyUsage    int     `json:"monthly_usage"`
	MonthlyLimit    int     `json:"monthly_limit"`
	IsProTier       bool    `json:"is_pro_tier"`
	UsagePercentage float64 `json:"usage_percentage"`
}

// CreateAccessGrantRequest - request to create an access grant for a team or user
type CreateAccessGrantRequest struct {
	UserReference string   `json:"user_reference"` // User ID or email
	TeamID        string   `json:"team_id"`        // Team ID
	Roles         []string `json:"roles"`          // Role names
}

// AccessGrant - grant access to a resource for a team or user. This allows users
// to share their application, knowledge, provider endpoint, etc with other users or teams.
type AccessGrant struct {
	ID             string    `json:"id" yaml:"id" gorm:"primaryKey"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ResourceID     string    `json:"resource_id" yaml:"resource_id"`         // App ID, Knowledge ID, etc
	OrganizationID string    `json:"organization_id" yaml:"organization_id"` // If granted to an organization
	TeamID         string    `json:"team_id" yaml:"team_id"`                 // If granted to a team
	UserID         string    `json:"user_id" yaml:"user_id"`                 // If granted to a user
	User           User      `json:"user" yaml:"user" gorm:"-"`              // Populated by the server if UserID is set
	Roles          []Role    `json:"roles,omitempty" yaml:"roles,omitempty" gorm:"-"`
}

// AccessGrantRoleBinding grants a role to the resource access binding
type AccessGrantRoleBinding struct {
	AccessGrantID  string    `json:"access_grant_id" yaml:"access_grant_id" gorm:"primaryKey"` //
	RoleID         string    `json:"role_id" yaml:"role_id" gorm:"primaryKey"`
	CreatedAt      time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" yaml:"updated_at"`
	OrganizationID string    `json:"organization_id" yaml:"organization_id" gorm:"index"`
}

// this lives in the database
// the ID is the keycloak user ID
// there might not be a record for every user
type UserMeta struct {
	ID     string     `json:"id"`
	Slug   string     `json:"slug" gorm:"uniqueIndex"` // URL-friendly username slug for GitHub-style URLs
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

type Config struct {
	Rules []Rule `json:"rules,omitempty" yaml:"rules,omitempty"`
}

type Rule struct {
	Resources []Resource `json:"resource,omitempty" yaml:"resource,omitempty"`
	Actions   []Action   `json:"actions,omitempty" yaml:"actions,omitempty"`
	Effect    Effect     `json:"effect,omitempty" yaml:"effect,omitempty"`
}

type Effect string

const (
	EffectAllow = Effect("allow")
	EffectDeny  = Effect("deny")
)

type Resource string

func (Resource) GormDataType() string {
	return "varchar(255)"
}

const (
	ResourceTeam                  Resource = "Team"
	ResourceOrganization          Resource = "Organization"
	ResourceRole                  Resource = "Role"
	ResourceMembership            Resource = "Membership"
	ResourceMembershipRoleBinding Resource = "MembershipRoleBinding"
	ResourceApplication           Resource = "Application"
	ResourceAccessGrants          Resource = "AccessGrants"
	ResourceKnowledge             Resource = "Knowledge"
	ResourceUser                  Resource = "User"
	ResourceAny                   Resource = "*"
	ResourceTypeDataset           Resource = "Dataset"
	ResourceProject               Resource = "Project"
	ResourceGitRepository         Resource = "GitRepository"
)

type Action string

const (
	ActionGet       Action = "Get"
	ActionList      Action = "List"
	ActionDelete    Action = "Delete"
	ActionUpdate    Action = "Update"
	ActionCreate    Action = "Create"
	ActionUseAction Action = "UseAction" // For example "use app"
)

var AvailableActions = map[Action]bool{
	ActionGet:       true,
	ActionList:      true,
	ActionCreate:    true,
	ActionDelete:    true,
	ActionUpdate:    true,
	ActionUseAction: true,
}

func (a Action) String() string {
	return string(a)
}

func ParseActions(actions []string) ([]Action, error) {
	var result []Action
	for _, action := range actions {
		a, err := ParseAction(action)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, nil

}

func ParseAction(a string) (Action, error) {
	_, ok := AvailableActions[Action(cases.Title(language.English).String(a))]
	if !ok {
		return Action(""), fmt.Errorf("action %s not found", a)
	}
	return Action(cases.Title(language.English).String(a)), nil
}

func (Config) GormDataType() string {
	return "json"
}

// GormDBDataType gorm db data type
func (Config) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Dialector.Name() {
	case "sqlite":
		return "JSON"
	case "mysql":
		return "JSON"
	case "postgres":
		return "JSONB"
	}
	return ""
}

func (m Config) Value() (driver.Value, error) {
	j, err := json.Marshal(m)
	return j, err
}

func (m *Config) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return errors.New("type assertion .([]byte]) failed")
	}
	var result Config
	if err := json.Unmarshal(source, &result); err != nil {
		return err
	}
	*m = result
	return nil
}

type CreateTeamRequest struct {
	Name           string `json:"name"`
	OrganizationID string `json:"organization_id"`
}

type UpdateTeamRequest struct {
	Name string `json:"name"`
}
