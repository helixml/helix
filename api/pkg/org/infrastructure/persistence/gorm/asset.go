package gorm

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

type assetsRepo struct{ db *gorm.DB }

func newAssetsRepo(db *gorm.DB) *assetsRepo { return &assetsRepo{db: db} }

func (r *assetsRepo) Create(ctx context.Context, a asset.Asset) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(&a).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("an asset named %q in this org %w", a.Name, store.ErrConflict)
		}
		return fmt.Errorf("create asset: %w", err)
	}
	return nil
}

func (r *assetsRepo) Get(ctx context.Context, orgID string, id asset.ID) (asset.Asset, error) {
	var a asset.Asset
	if err := r.db.WithContext(ctx).Where("org_id = ? AND id = ?", orgID, id).First(&a).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return asset.Asset{}, fmt.Errorf("asset: %w", store.ErrNotFound)
		}
		return asset.Asset{}, fmt.Errorf("get asset: %w", err)
	}
	if err := a.Validate(); err != nil {
		return asset.Asset{}, fmt.Errorf("validate persisted asset: %w", err)
	}
	return a, nil
}

func (r *assetsRepo) GetByName(ctx context.Context, orgID, name string) (asset.Asset, error) {
	var a asset.Asset
	if err := r.db.WithContext(ctx).Where("org_id = ? AND name = ?", orgID, name).First(&a).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return asset.Asset{}, fmt.Errorf("asset: %w", store.ErrNotFound)
		}
		return asset.Asset{}, fmt.Errorf("get asset by name: %w", err)
	}
	if err := a.Validate(); err != nil {
		return asset.Asset{}, fmt.Errorf("validate persisted asset: %w", err)
	}
	return a, nil
}

func (r *assetsRepo) List(ctx context.Context, orgID string) ([]asset.Asset, error) {
	var assets []asset.Asset
	if err := r.db.WithContext(ctx).Where("org_id = ?", orgID).Order("name ASC").Find(&assets).Error; err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	for i := range assets {
		if err := assets[i].Validate(); err != nil {
			return nil, fmt.Errorf("validate persisted asset %q: %w", assets[i].ID, err)
		}
	}
	return assets, nil
}

func (r *assetsRepo) Update(ctx context.Context, a asset.Asset) error {
	if err := a.Validate(); err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&asset.Asset{}).
		Where("org_id = ? AND id = ?", a.OrganizationID, a.ID).
		Select("*").
		Omit("ID", "OrganizationID", "CreatedAt").
		Updates(&a)
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return fmt.Errorf("an asset named %q in this org %w", a.Name, store.ErrConflict)
		}
		return fmt.Errorf("update asset: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("asset: %w", store.ErrNotFound)
	}
	return nil
}

func (r *assetsRepo) Delete(ctx context.Context, orgID string, id asset.ID) error {
	res := r.db.WithContext(ctx).Where("org_id = ? AND id = ?", orgID, id).Delete(&asset.Asset{})
	if res.Error != nil {
		return fmt.Errorf("delete asset: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("asset: %w", store.ErrNotFound)
	}
	return nil
}

type assetLinksRepo struct{ db *gorm.DB }

func newAssetLinksRepo(db *gorm.DB) *assetLinksRepo { return &assetLinksRepo{db: db} }

func (r *assetLinksRepo) Create(ctx context.Context, link asset.Link) error {
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
		return fmt.Errorf("create asset link: %w", err)
	}
	return nil
}

func (r *assetLinksRepo) Delete(ctx context.Context, orgID string, assetID asset.ID, agentID string) error {
	res := r.db.WithContext(ctx).Where("org_id = ? AND asset_id = ? AND agent_id = ?", orgID, assetID, agentID).Delete(&asset.Link{})
	if res.Error != nil {
		return fmt.Errorf("delete asset link: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("asset link: %w", store.ErrNotFound)
	}
	return nil
}

func (r *assetLinksRepo) Find(ctx context.Context, orgID string, assetID asset.ID, agentID string) (asset.Link, error) {
	var link asset.Link
	err := r.db.WithContext(ctx).Where("org_id = ? AND asset_id = ? AND agent_id = ?", orgID, assetID, agentID).First(&link).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return asset.Link{}, fmt.Errorf("asset link: %w", store.ErrNotFound)
		}
		return asset.Link{}, fmt.Errorf("find asset link: %w", err)
	}
	return link, nil
}

func (r *assetLinksRepo) ListForAsset(ctx context.Context, orgID string, assetID asset.ID) ([]asset.Link, error) {
	return r.list(ctx, "asset_id = ?", orgID, assetID)
}

func (r *assetLinksRepo) ListForAgent(ctx context.Context, orgID, agentID string) ([]asset.Link, error) {
	return r.list(ctx, "agent_id = ?", orgID, agentID)
}

func (r *assetLinksRepo) list(ctx context.Context, condition string, orgID string, value any) ([]asset.Link, error) {
	var links []asset.Link
	if err := r.db.WithContext(ctx).Where("org_id = ?", orgID).Where(condition, value).Order("asset_id ASC, agent_id ASC").Find(&links).Error; err != nil {
		return nil, fmt.Errorf("list asset links: %w", err)
	}
	return links, nil
}
