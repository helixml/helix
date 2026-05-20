package domain

import (
	"errors"
	"time"
)

// Role is a job description. Owner-only: workers cannot edit their own
// Role. The owner edits Content via UpdateRole, and the new content
// fans out to every Worker filling a Position with this Role.
//
// Content is the canonical markdown the Worker reads on activation
// (it lands in role.md inside the Worker's Environment). Identity
// (name, voice, personality) is per-Worker, not per-Role.
type Role struct {
	ID        RoleID
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewRole validates and constructs a Role. Treat the returned value as immutable.
func NewRole(id RoleID, content string, now time.Time) (Role, error) {
	if id == "" {
		return Role{}, errors.New("role id is empty")
	}
	if content == "" {
		return Role{}, errors.New("role content is empty")
	}
	if now.IsZero() {
		return Role{}, errors.New("role timestamp is zero")
	}
	return Role{
		ID:        id,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}
