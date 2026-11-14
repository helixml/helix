package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// ErrDeleteInProgress is returned by Delete when another delete of the same
// Node is already running.
var ErrDeleteInProgress = errors.New("bot is already being deleted")

type deleteKey struct {
	orgID string
	id    orgchart.NodeID
}

// StartDelete runs Delete in the background so callers do not wait for the
// sandbox teardown, which can take minutes. It fails synchronously only when
// the Node does not exist. Starting a delete that is already in flight is a
// no-op. A failed background delete leaves the Node in place (Delete is
// retry-safe), logs the error, and clears the in-flight marker so the Node is
// visible and deletable again.
func (s *Service) StartDelete(ctx context.Context, orgID string, id orgchart.NodeID) error {
	if id == "" {
		return errors.New("node id is empty")
	}
	if s.Store == nil {
		return errors.New("lifecycle: store is nil")
	}
	if s.Deleting(orgID, id) {
		return nil
	}
	if _, err := s.Store.Nodes.Get(ctx, orgID, id); err != nil {
		return fmt.Errorf("get node %q: %w", id, err)
	}
	if !s.claimDelete(orgID, id) {
		return nil
	}
	s.deletes.Add(1)
	go func() {
		defer s.deletes.Done()
		defer s.releaseDelete(orgID, id)
		if err := s.deleteDetached(ctx, orgID, id); err != nil {
			s.logger().Error("background delete failed", "org", orgID, "node", id, "err", err)
			return
		}
		s.logger().Info("background delete finished", "org", orgID, "node", id)
	}()
	return nil
}

// Deleting reports whether a delete of the Node is in flight. Read paths hide
// such Nodes and mutation paths refuse them.
func (s *Service) Deleting(orgID string, id orgchart.NodeID) bool {
	if s == nil {
		return false
	}
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	_, ok := s.deleting[deleteKey{orgID: orgID, id: id}]
	return ok
}

// claimDelete marks the Node as being deleted. It returns false when another
// delete already holds the claim.
func (s *Service) claimDelete(orgID string, id orgchart.NodeID) bool {
	key := deleteKey{orgID: orgID, id: id}
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	if _, ok := s.deleting[key]; ok {
		return false
	}
	if s.deleting == nil {
		s.deleting = map[deleteKey]struct{}{}
	}
	s.deleting[key] = struct{}{}
	return true
}

func (s *Service) releaseDelete(orgID string, id orgchart.NodeID) {
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	delete(s.deleting, deleteKey{orgID: orgID, id: id})
}
