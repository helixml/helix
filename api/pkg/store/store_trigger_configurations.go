package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateTriggerConfiguration(ctx context.Context, triggerConfig *types.TriggerConfiguration) (*types.TriggerConfiguration, error) {
	if triggerConfig.ID == "" {
		triggerConfig.ID = system.GenerateTriggerConfigurationID()
	}

	if triggerConfig.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	if triggerConfig.Name == "" {
		return nil, fmt.Errorf("name not specified")
	}

	if triggerConfig.AppID == "" {
		return nil, fmt.Errorf("app_id not specified")
	}

	var triggerType types.TriggerType

	switch {
	case triggerConfig.Trigger.Cron != nil:
		triggerType = types.TriggerTypeCron
	case triggerConfig.Trigger.AzureDevOps != nil:
		triggerType = types.TriggerTypeAzureDevOps
	case triggerConfig.Trigger.Slack != nil:
		triggerType = types.TriggerTypeSlack
	default:
		return nil, fmt.Errorf("trigger type not specified")
	}

	triggerConfig.Created = time.Now()
	triggerConfig.Updated = time.Now()
	triggerConfig.TriggerType = triggerType

	err := s.gdb.WithContext(ctx).Create(triggerConfig).Error
	if err != nil {
		return nil, err
	}
	return s.GetTriggerConfiguration(ctx, &GetTriggerConfigurationQuery{ID: triggerConfig.ID})
}

func (s *PostgresStore) UpdateTriggerConfiguration(ctx context.Context, triggerConfig *types.TriggerConfiguration) (*types.TriggerConfiguration, error) {
	if triggerConfig.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if triggerConfig.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	if triggerConfig.Name == "" {
		return nil, fmt.Errorf("name not specified")
	}

	if triggerConfig.AppID == "" {
		return nil, fmt.Errorf("app_id not specified")
	}

	triggerConfig.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Save(&triggerConfig).Error
	if err != nil {
		return nil, err
	}
	return s.GetTriggerConfiguration(ctx, &GetTriggerConfigurationQuery{ID: triggerConfig.ID})
}

func (s *PostgresStore) GetTriggerConfiguration(ctx context.Context, q *GetTriggerConfigurationQuery) (*types.TriggerConfiguration, error) {
	var triggerConfig types.TriggerConfiguration
	query := s.gdb.WithContext(ctx)

	if q.ID != "" {
		query = query.Where("id = ?", q.ID)
	}

	if q.Owner != "" {
		query = query.Where("owner = ?", q.Owner)
	}

	if q.OwnerType != "" {
		query = query.Where("owner_type = ?", q.OwnerType)
	}

	if q.OrganizationID != "" {
		query = query.Where("organization_id = ?", q.OrganizationID)
	}

	err := query.First(&triggerConfig).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &triggerConfig, nil
}

func (s *PostgresStore) ListTriggerConfigurations(ctx context.Context, q *ListTriggerConfigurationsQuery) ([]*types.TriggerConfiguration, error) {
	var triggerConfigs []*types.TriggerConfiguration
	query := s.gdb.WithContext(ctx)

	if q.Owner != "" {
		query = query.Where("owner = ?", q.Owner)
	}

	if q.OwnerType != "" {
		query = query.Where("owner_type = ?", q.OwnerType)
	}

	if q.OrganizationID != "" {
		query = query.Where("organization_id = ?", q.OrganizationID)
	}

	if q.AppID != "" {
		query = query.Where("app_id = ?", q.AppID)
	}

	if q.TriggerType != "" {
		query = query.Where("trigger_type = ?", q.TriggerType)
	}

	if q.Enabled {
		query = query.Where("enabled = ?", q.Enabled)
	}

	err := query.Order("enabled DESC, created DESC").Find(&triggerConfigs).Error
	if err != nil {
		return nil, err
	}
	return triggerConfigs, nil
}

func (s *PostgresStore) DeleteTriggerConfiguration(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.TriggerConfiguration{
		ID: id,
	}).Error
	if err != nil {
		return err
	}
	return nil
}
