package workersecret

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

var (
	ErrInvalidBinding = errors.New("invalid worker secret binding")
	secretNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
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
		return fmt.Errorf("%w: organization id and worker id are required", ErrInvalidBinding)
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		return fmt.Errorf("%w: secret name is required", ErrInvalidBinding)
	}
	if !secretNamePattern.MatchString(name) {
		return fmt.Errorf("%w: secret name must be a shell identifier", ErrInvalidBinding)
	}
	upperName := strings.ToUpper(name)
	if upperName == "USER_API_TOKEN" || strings.HasPrefix(upperName, "HELIX_") {
		return fmt.Errorf("%w: secret name is reserved for Helix bootstrap configuration", ErrInvalidBinding)
	}
	switch b.SourceKind {
	case SourceHelixSecret:
		if b.SecretID == "" || b.AccountID != "" || b.ExportKey != "" {
			return fmt.Errorf("%w: helix_secret requires only secret_id", ErrInvalidBinding)
		}
	case SourceConnectedAccount:
		if b.SecretID != "" || b.AccountID == "" || b.ExportKey == "" {
			return fmt.Errorf("%w: connected_account requires account_id and export_key", ErrInvalidBinding)
		}
	default:
		return fmt.Errorf("%w: invalid secret source kind", ErrInvalidBinding)
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
	ResourceID        string     `json:"resource_id,omitempty"`
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
	ResourceID   string     `json:"resource_id,omitempty"`
	AlreadyBound bool       `json:"already_bound"`
}
