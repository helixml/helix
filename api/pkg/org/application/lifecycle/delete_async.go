package lifecycle

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

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
		return fmt.Errorf("node id is empty")
	}
	if s.Store == nil {
		return fmt.Errorf("lifecycle: store is nil")
	}
	if s.Deleting(orgID, id) {
		return nil
	}
	if _, err := s.Store.Nodes.Get(ctx, orgID, id); err != nil {
		return fmt.Errorf("get node %q: %w", id, err)
	}
	key := deleteKey{orgID: orgID, id: id}
	s.deleteMu.Lock()
	if _, ok := s.deleting[key]; ok {
		s.deleteMu.Unlock()
		return nil
	}
	if s.deleting == nil {
		s.deleting = map[deleteKey]struct{}{}
	}
	s.deleting[key] = struct{}{}
	s.deletes.Add(1)
	s.deleteMu.Unlock()

	go func() {
		defer s.deletes.Done()
		defer func() {
			s.deleteMu.Lock()
			delete(s.deleting, key)
			s.deleteMu.Unlock()
		}()
		if err := s.Delete(ctx, orgID, id); err != nil {
			s.logger().Error("background delete failed", "org", orgID, "node", id, "err", err)
			return
		}
		s.logger().Info("background delete finished", "org", orgID, "node", id)
	}()
	return nil
}

// Deleting reports whether a background delete of the Node is in flight.
// Read paths hide such Nodes and mutation paths refuse them.
func (s *Service) Deleting(orgID string, id orgchart.NodeID) bool {
	if s == nil {
		return false
	}
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	_, ok := s.deleting[deleteKey{orgID: orgID, id: id}]
	return ok
}
