package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

type assetsRepo struct {
	mu    sync.RWMutex
	rows  map[orgKey]asset.Asset
	links *assetLinksRepo
}

func (r *assetsRepo) Create(_ context.Context, a asset.Asset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{OrgID: a.OrganizationID, ID: a.ID}
	if _, ok := r.rows[k]; ok {
		return fmt.Errorf("asset %q in org %q: already exists", a.ID, a.OrganizationID)
	}
	for key, existing := range r.rows {
		if key.OrgID == a.OrganizationID && existing.Name == a.Name {
			return fmt.Errorf("an asset named %q in this org %w", a.Name, store.ErrConflict)
		}
	}
	r.rows[k] = a
	return nil
}

func (r *assetsRepo) Get(_ context.Context, orgID string, id asset.ID) (asset.Asset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a, ok := r.rows[orgKey{OrgID: orgID, ID: id}]; ok {
		return a, nil
	}
	return asset.Asset{}, fmt.Errorf("asset %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (r *assetsRepo) GetByName(_ context.Context, orgID, name string) (asset.Asset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for key, a := range r.rows {
		if key.OrgID == orgID && a.Name == name {
			return a, nil
		}
	}
	return asset.Asset{}, fmt.Errorf("asset %q in org %q: %w", name, orgID, store.ErrNotFound)
}

func (r *assetsRepo) List(_ context.Context, orgID string) ([]asset.Asset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]asset.Asset, 0)
	for key, a := range r.rows {
		if key.OrgID == orgID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *assetsRepo) Update(_ context.Context, a asset.Asset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := orgKey{OrgID: a.OrganizationID, ID: a.ID}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("asset %q in org %q: %w", a.ID, a.OrganizationID, store.ErrNotFound)
	}
	for key, existing := range r.rows {
		if key != k && key.OrgID == a.OrganizationID && existing.Name == a.Name {
			return fmt.Errorf("an asset named %q in this org %w", a.Name, store.ErrConflict)
		}
	}
	r.rows[k] = a
	return nil
}

func (r *assetsRepo) Delete(_ context.Context, orgID string, id asset.ID) error {
	r.mu.Lock()
	k := orgKey{OrgID: orgID, ID: id}
	if _, ok := r.rows[k]; !ok {
		r.mu.Unlock()
		return fmt.Errorf("asset %q in org %q: %w", id, orgID, store.ErrNotFound)
	}
	delete(r.rows, k)
	r.mu.Unlock()
	r.links.deleteForAsset(orgID, id)
	return nil
}

type assetLinkKey struct {
	orgID, assetID, agentID string
}

type assetLinksRepo struct {
	mu   sync.RWMutex
	rows map[assetLinkKey]asset.Link
}

func (r *assetLinksRepo) Create(_ context.Context, link asset.Link) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[assetLinkKey{link.OrganizationID, link.AssetID, link.AgentID}] = link
	return nil
}

func (r *assetLinksRepo) Delete(_ context.Context, orgID string, assetID asset.ID, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := assetLinkKey{orgID, assetID, agentID}
	if _, ok := r.rows[k]; !ok {
		return fmt.Errorf("asset link: %w", store.ErrNotFound)
	}
	delete(r.rows, k)
	return nil
}

func (r *assetLinksRepo) Find(_ context.Context, orgID string, assetID asset.ID, agentID string) (asset.Link, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if link, ok := r.rows[assetLinkKey{orgID, assetID, agentID}]; ok {
		return link, nil
	}
	return asset.Link{}, fmt.Errorf("asset link: %w", store.ErrNotFound)
}

func (r *assetLinksRepo) ListForAsset(_ context.Context, orgID string, assetID asset.ID) ([]asset.Link, error) {
	return r.list(func(k assetLinkKey) bool { return k.orgID == orgID && k.assetID == assetID }), nil
}

func (r *assetLinksRepo) ListForAgent(_ context.Context, orgID, agentID string) ([]asset.Link, error) {
	return r.list(func(k assetLinkKey) bool { return k.orgID == orgID && k.agentID == agentID }), nil
}

func (r *assetLinksRepo) list(match func(assetLinkKey) bool) []asset.Link {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]asset.Link, 0)
	for key, link := range r.rows {
		if match(key) {
			out = append(out, link)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AssetID != out[j].AssetID {
			return out[i].AssetID < out[j].AssetID
		}
		return out[i].AgentID < out[j].AgentID
	})
	return out
}

func (r *assetLinksRepo) deleteForAsset(orgID string, assetID asset.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.rows {
		if key.orgID == orgID && key.assetID == assetID {
			delete(r.rows, key)
		}
	}
}
