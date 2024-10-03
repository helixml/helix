package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateApp(ctx context.Context, app *types.App) (*types.App, error) {
	if app.ID == "" {
		app.ID = system.GenerateAppID()
	}

	if app.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	app.Created = time.Now()

	setAppDefaults(app)
	sortAppTools(app)

	err := s.gdb.WithContext(ctx).Create(app).Error
	if err != nil {
		return nil, err
	}
	return s.GetApp(ctx, app.ID)
}

func sortAppTools(app *types.App) {
	for idx, assistant := range app.Config.Helix.Assistants {
		sort.SliceStable(assistant.Tools, func(i, j int) bool {
			return assistant.Tools[i].Name < assistant.Tools[j].Name
		})
		app.Config.Helix.Assistants[idx] = assistant
	}
}

func (s *PostgresStore) UpdateApp(ctx context.Context, app *types.App) (*types.App, error) {
	if app.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if app.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	app.Updated = time.Now()

	sortAppTools(app)

	err := s.gdb.WithContext(ctx).Save(&app).Error
	if err != nil {
		return nil, err
	}
	return s.GetApp(ctx, app.ID)
}

func (s *PostgresStore) GetApp(ctx context.Context, id string) (*types.App, error) {
	var tool types.App
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&tool).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	setAppDefaults(&tool)

	return &tool, nil
}

func (s *PostgresStore) ListApps(ctx context.Context, q *ListAppsQuery) ([]*types.App, error) {
	var tools []*types.App
	err := s.gdb.WithContext(ctx).Where(&types.App{
		Owner:     q.Owner,
		OwnerType: q.OwnerType,
		Global:    q.Global,
	}).Order("id DESC").Find(&tools).Error
	if err != nil {
		return nil, err
	}

	setAppDefaults(tools...)

	return tools, nil
}

func (s *PostgresStore) DeleteApp(ctx context.Context, id string) error {
	err := s.gdb.WithContext(ctx).Delete(&types.App{
		ID: id,
	}).Error
	if err != nil {
		return err
	}

	return nil
}

func setAppDefaults(apps ...*types.App) {
	for idx := range apps {
		app := apps[idx]
		if app.Config.Helix.Assistants == nil {
			app.Config.Helix.Assistants = []types.AssistantConfig{}
		}
	}
}
