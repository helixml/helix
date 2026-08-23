package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/helixml/helix/api/pkg/org/domain/attachment"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
)

func stringCondition(opts []store.Option, field string) (string, bool) {
	for _, c := range store.Build(opts...).Conditions() {
		if c.Field == field && !c.In {
			return fmt.Sprint(c.Value), true
		}
	}
	return "", false
}

type triggersRepo struct {
	mu          sync.RWMutex
	rows        map[orgKey]trigger.Trigger
	attachments *attachmentsRepo
}

func (r *triggersRepo) Create(_ context.Context, t trigger.Trigger) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{t.OrganizationID, t.ID}
	if _, ok := r.rows[k]; ok {
		return fmt.Errorf("trigger %q %w", t.ID, store.ErrConflict)
	}
	for k2, x := range r.rows {
		if k2.OrgID == t.OrganizationID && x.Name == t.Name {
			return fmt.Errorf("trigger %q %w", t.Name, store.ErrConflict)
		}
	}
	r.rows[k] = t
	return nil
}
func (r *triggersRepo) Update(_ context.Context, t trigger.Trigger) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{t.OrganizationID, t.ID}
	old, ok := r.rows[k]
	if !ok {
		return fmt.Errorf("trigger %q: %w", t.ID, store.ErrNotFound)
	}
	for k2, x := range r.rows {
		if k2 != k && k2.OrgID == t.OrganizationID && x.Name == t.Name {
			return fmt.Errorf("trigger %q %w", t.Name, store.ErrConflict)
		}
	}
	old.Name = t.Name
	old.Kind = t.Kind
	old.Config = t.Config
	r.rows[k] = old
	return nil
}
func (r *triggersRepo) Delete(_ context.Context, orgID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{orgID, id}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("trigger %q: %w", id, store.ErrNotFound)
	}
	delete(r.rows, k)
	r.attachments.deleteForTrigger(orgID, id)
	return nil
}
func (r *triggersRepo) Find(_ context.Context, opts ...store.Option) ([]trigger.Trigger, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	org, _ := stringCondition(opts, "org_id")
	id, hasID := stringCondition(opts, "id")
	kind, hasKind := stringCondition(opts, "kind")
	out := []trigger.Trigger{}
	for k, t := range r.rows {
		if (org != "" && k.OrgID != org) || (hasID && t.ID != id) || (hasKind && string(t.Kind) != kind) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	q := store.Build(opts...)
	if q.Offset() < len(out) {
		out = out[q.Offset():]
	} else {
		out = nil
	}
	if q.Limit() > 0 && len(out) > q.Limit() {
		out = out[:q.Limit()]
	}
	return out, nil
}

type attachmentsRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]attachment.Attachment
}

func (r *attachmentsRepo) Create(_ context.Context, a attachment.Attachment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{a.OrganizationID, a.ID}
	if _, ok := r.rows[k]; ok {
		return fmt.Errorf("attachment %q %w", a.ID, store.ErrConflict)
	}
	for _, x := range r.rows {
		if x.OrganizationID == a.OrganizationID && x.WorkerID == a.WorkerID && x.Source == a.Source {
			return fmt.Errorf("worker source attachment %w", store.ErrConflict)
		}
	}
	r.rows[k] = a
	return nil
}
func (r *attachmentsRepo) Delete(_ context.Context, orgID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{orgID, id}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("attachment %q: %w", id, store.ErrNotFound)
	}
	delete(r.rows, k)
	return nil
}
func (r *attachmentsRepo) Find(_ context.Context, opts ...store.Option) ([]attachment.Attachment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	org, _ := stringCondition(opts, "org_id")
	id, hasID := stringCondition(opts, "id")
	worker, hasWorker := stringCondition(opts, "worker_id")
	trig, hasTrig := stringCondition(opts, "trigger_id")
	proc, hasProc := stringCondition(opts, "processor_id")
	output, hasOutput := stringCondition(opts, "output_id")
	out := []attachment.Attachment{}
	for k, a := range r.rows {
		if (org != "" && k.OrgID != org) || (hasID && a.ID != id) || (hasWorker && string(a.WorkerID) != worker) || (hasTrig && a.Source.TriggerID != trig) || (hasProc && string(a.Source.ProcessorID) != proc) || (hasOutput && a.Source.OutputID != output) {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r *attachmentsRepo) deleteForWorker(org, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, a := range r.rows {
		if a.OrganizationID == org && string(a.WorkerID) == id {
			delete(r.rows, k)
		}
	}
}
func (r *attachmentsRepo) deleteForTrigger(org, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, a := range r.rows {
		if a.OrganizationID == org && a.Source.TriggerID == id {
			delete(r.rows, k)
		}
	}
}
func (r *attachmentsRepo) deleteForProcessor(org, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, a := range r.rows {
		if a.OrganizationID == org && string(a.Source.ProcessorID) == id {
			delete(r.rows, k)
		}
	}
}
