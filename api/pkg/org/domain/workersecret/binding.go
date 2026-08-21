package workersecret

import (
	"errors"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

type SourceKind string

const (
	SourceHelixSecret      SourceKind = "helix_secret"
	SourceConnectedAccount SourceKind = "connected_account"
)

type Binding struct {
	OrganizationID    string
	WorkerID          orgchart.NodeID
	Name              string
	Description       string
	Usage             string
	ContentType       string
	SuggestedFilename string
	SourceKind        SourceKind
	SecretID          string
	AccountID         string
	ExportKey         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (b Binding) Validate() error {
	if strings.TrimSpace(b.OrganizationID) == "" || strings.TrimSpace(string(b.WorkerID)) == "" {
		return errors.New("organization id and worker id are required")
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		return errors.New("secret name is required")
	}
	if name == "USER_API_TOKEN" || strings.HasPrefix(name, "HELIX_") {
		return errors.New("secret name is reserved for Helix bootstrap configuration")
	}
	switch b.SourceKind {
	case SourceHelixSecret:
		if b.SecretID == "" || b.AccountID != "" || b.ExportKey != "" {
			return errors.New("helix_secret requires only secret_id")
		}
	case SourceConnectedAccount:
		if b.SecretID != "" || b.AccountID == "" || b.ExportKey == "" {
			return errors.New("connected_account requires account_id and export_key")
		}
	default:
		return errors.New("invalid secret source kind")
	}
	return nil
}

type Descriptor struct {
	Name              string     `json:"name"`
	Description       string     `json:"description,omitempty"`
	Usage             string     `json:"usage,omitempty"`
	ContentType       string     `json:"content_type,omitempty"`
	SuggestedFilename string     `json:"suggested_filename,omitempty"`
	Available         bool       `json:"available"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

type Resolved struct {
	Descriptor
	Value string `json:"value"`
}

type AvailableSource struct {
	Group        string     `json:"group"`
	Label        string     `json:"label"`
	SourceKind   SourceKind `json:"source_kind"`
	SecretID     string     `json:"secret_id,omitempty"`
	AccountID    string     `json:"account_id,omitempty"`
	ExportKey    string     `json:"export_key,omitempty"`
	ProposedName string     `json:"proposed_name"`
	Usage        string     `json:"usage,omitempty"`
	AlreadyBound bool       `json:"already_bound"`
}
