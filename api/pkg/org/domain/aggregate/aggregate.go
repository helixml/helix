package aggregate

import (
	"errors"
	"time"
)

// Aggregate contains identity and tenancy state shared by org aggregates.
type Aggregate struct {
	ID             string
	OrganizationID string
	CreatedBy      string
	CreatedAt      time.Time
}

func New(id, orgID, createdBy string, createdAt time.Time) (Aggregate, error) {
	if id == "" {
		return Aggregate{}, errors.New("aggregate id is empty")
	}
	if orgID == "" {
		return Aggregate{}, errors.New("aggregate organization id is empty")
	}
	if createdAt.IsZero() {
		return Aggregate{}, errors.New("aggregate created at is zero")
	}
	return Aggregate{ID: id, OrganizationID: orgID, CreatedBy: createdBy, CreatedAt: createdAt.UTC()}, nil
}
