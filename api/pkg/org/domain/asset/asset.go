package asset

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

type ID = string
type Kind = string
type AuthType = string

const KindServer Kind = "server"

const (
	AuthSSHKey   AuthType = "ssh_key"
	AuthPassword AuthType = "password"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

type Server struct {
	Address             string   `json:"address"`
	Port                uint16   `json:"port"`
	User                string   `json:"user"`
	AuthType            AuthType `json:"auth_type"`
	PublicKey           string   `json:"public_key"`
	EncryptedPrivateKey string   `json:"encrypted_private_key"`
	EncryptedPassword   string   `json:"encrypted_password,omitempty"`
	HostKey             string   `json:"host_key,omitempty"`
}

type Config struct {
	Server *Server `json:"server,omitempty"`
}

type Asset struct {
	ID             ID        `json:"id" gorm:"primaryKey;type:text"`
	OrganizationID string    `json:"organization_id" gorm:"column:org_id;primaryKey;type:text;index;uniqueIndex:idx_asset_org_name,priority:1"`
	Name           string    `json:"name" gorm:"not null;uniqueIndex:idx_asset_org_name,priority:2"`
	Description    string    `json:"description,omitempty" gorm:"not null;default:''"`
	NotesForAgents string    `json:"notes_for_agents,omitempty" gorm:"not null;default:''"`
	Disabled       bool      `json:"disabled" gorm:"not null;default:false"`
	Kind           Kind      `json:"kind" gorm:"not null;index"`
	Config         Config    `json:"config" gorm:"serializer:json;type:jsonb;not null"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Link struct {
	OrganizationID string    `json:"organization_id" gorm:"column:org_id;primaryKey;type:text;index"`
	AssetID        ID        `json:"asset_id" gorm:"primaryKey;type:text;index"`
	AgentID        string    `json:"agent_id" gorm:"primaryKey;type:text;index"`
	CreatedAt      time.Time `json:"created_at"`
}

func (Asset) TableName() string { return "org_assets" }

func (Link) TableName() string { return "org_asset_links" }

func New(id ID, orgID, name, description, notesForAgents string, kind Kind, config Config, now time.Time) (Asset, error) {
	a := Asset{
		ID: id, OrganizationID: orgID, Name: strings.TrimSpace(name),
		Description: strings.TrimSpace(description), NotesForAgents: strings.TrimSpace(notesForAgents), Kind: kind, Config: config,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := a.Validate(); err != nil {
		return Asset{}, err
	}
	return a, nil
}

func (a Asset) Validate() error {
	if strings.TrimSpace(a.ID) == "" {
		return errors.New("asset id is empty")
	}
	if strings.TrimSpace(a.OrganizationID) == "" {
		return errors.New("asset orgID is empty")
	}
	if !namePattern.MatchString(a.Name) {
		return errors.New("asset name must be 1-63 lowercase letters, numbers, dots, underscores, or hyphens")
	}
	if a.CreatedAt.IsZero() || a.UpdatedAt.IsZero() {
		return errors.New("asset timestamp is zero")
	}
	switch a.Kind {
	case KindServer:
		if a.Config.Server == nil {
			return errors.New("server asset config is missing")
		}
		return validateServer(*a.Config.Server)
	default:
		return fmt.Errorf("unsupported asset kind %q", a.Kind)
	}
}

func validateServer(s Server) error {
	address := strings.TrimSpace(s.Address)
	if address == "" {
		return errors.New("server address is empty")
	}
	if strings.ContainsAny(address, " \t\r\n/@") || strings.Contains(address, "://") {
		return errors.New("server address must be a hostname or IP address without a scheme, path, or port")
	}
	if strings.Contains(address, ":") && net.ParseIP(address) == nil {
		return errors.New("server address contains an invalid IP address")
	}
	if s.Port == 0 {
		return errors.New("server port is zero")
	}
	if strings.TrimSpace(s.User) == "" || strings.ContainsAny(s.User, " \t\r\n@") {
		return errors.New("server user is invalid")
	}
	switch s.AuthType {
	case AuthSSHKey:
		if strings.TrimSpace(s.PublicKey) == "" {
			return errors.New("server public key is empty")
		}
		if strings.TrimSpace(s.EncryptedPrivateKey) == "" {
			return errors.New("server encrypted private key is empty")
		}
		if s.EncryptedPassword != "" {
			return errors.New("SSH-key server must not contain an encrypted password")
		}
	case AuthPassword:
		if strings.TrimSpace(s.EncryptedPassword) == "" {
			return errors.New("server encrypted password is empty")
		}
		if s.PublicKey != "" || s.EncryptedPrivateKey != "" {
			return errors.New("password server must not contain an SSH key pair")
		}
	default:
		return fmt.Errorf("unsupported server auth type %q", s.AuthType)
	}
	return nil
}

func NewLink(orgID string, assetID ID, agentID string, now time.Time) (Link, error) {
	l := Link{OrganizationID: orgID, AssetID: assetID, AgentID: agentID, CreatedAt: now}
	if strings.TrimSpace(orgID) == "" {
		return Link{}, errors.New("asset link orgID is empty")
	}
	if strings.TrimSpace(assetID) == "" {
		return Link{}, errors.New("asset link assetID is empty")
	}
	if strings.TrimSpace(agentID) == "" {
		return Link{}, errors.New("asset link agentID is empty")
	}
	if now.IsZero() {
		return Link{}, errors.New("asset link timestamp is zero")
	}
	return l, nil
}
