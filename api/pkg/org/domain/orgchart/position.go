package orgchart

import "errors"

// Position is a concrete slot in the org chart, instantiating a Role.
// ParentID is nil for the root position.
type Position struct {
	ID             PositionID
	OrganizationID string
	RoleID         RoleID
	ParentID       *PositionID
}

// NewPosition validates and constructs a Position. orgID is required
// — every Position is scoped to a helix.Organization via the
// composite (id, org_id) PK. Pass parentID = nil for the root
// position.
func NewPosition(id PositionID, roleID RoleID, parentID *PositionID, orgID string) (Position, error) {
	if id == "" {
		return Position{}, errors.New("position id is empty")
	}
	if roleID == "" {
		return Position{}, errors.New("position role id is empty")
	}
	if orgID == "" {
		return Position{}, errors.New("position orgID is empty")
	}
	var parent *PositionID
	if parentID != nil {
		if *parentID == "" {
			return Position{}, errors.New("parent position id is empty")
		}
		if *parentID == id {
			return Position{}, errors.New("position cannot be its own parent")
		}
		p := *parentID
		parent = &p
	}
	return Position{ID: id, OrganizationID: orgID, RoleID: roleID, ParentID: parent}, nil
}

// IsRoot reports whether the position has no parent.
func (p Position) IsRoot() bool { return p.ParentID == nil }
