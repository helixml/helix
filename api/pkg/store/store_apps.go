package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/getkin/kin-openapi/openapi3"
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

	spew.Dump(app)

	sortAppTools(app)

	err := s.gdb.WithContext(ctx).Save(&app).Error
	if err != nil {
		return nil, err
	}
	return s.GetApp(ctx, app.ID)
}

func (s *PostgresStore) GetApp(ctx context.Context, id string) (*types.App, error) {
	var app types.App
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&app).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	setAppDefaults(&app)

	return &app, nil
}

// XXX Copy paste to avoid import cycle
func GetActionsFromSchema(spec string) ([]*types.ToolAPIAction, error) {
	loader := openapi3.NewLoader()

	schema, err := loader.LoadFromData([]byte(spec))
	if err != nil {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}

	var actions []*types.ToolAPIAction

	for path, pathItem := range schema.Paths.Map() {

		for method, operation := range pathItem.Operations() {
			description := operation.Summary
			if description == "" {
				description = operation.Description
			}

			if operation.OperationID == "" {
				return nil, fmt.Errorf("operationId is missing for all %s %s", method, path)
			}

			actions = append(actions, &types.ToolAPIAction{
				Name:        operation.OperationID,
				Description: description,
				Path:        path,
				Method:      method,
			})
		}
	}

	return actions, nil
}

// ConvertAPIToTool converts an AssistantAPI to a Tool
func ConvertAPIToTool(api types.AssistantAPI) (*types.Tool, error) {
	t := &types.Tool{
		Name:        api.Name,
		Description: api.Description,
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:                     api.URL,
				Schema:                  api.Schema,
				Headers:                 api.Headers,
				Query:                   api.Query,
				RequestPrepTemplate:     api.RequestPrepTemplate,
				ResponseSuccessTemplate: api.ResponseSuccessTemplate,
				ResponseErrorTemplate:   api.ResponseErrorTemplate,
			},
		},
	}

	actions, err := GetActionsFromSchema(api.Schema)
	if err != nil {
		return nil, fmt.Errorf("error getting actions from schema: %w", err)
	}
	t.Config.API.Actions = actions

	return t, nil
}

// BACKWARD COMPATIBILITY ONLY: return an app with the apis, gptscripts, and zapier
// transformed into the deprecated (or at least internal) Tools field
func (s *PostgresStore) GetAppWithTools(ctx context.Context, id string) (*types.App, error) {
	app, err := s.GetApp(ctx, id)
	if err != nil {
		return nil, err
	}
	return ParseAppTools(app)
}

func ParseAppTools(app *types.App) (*types.App, error) {
	// Convert each assistant's specific tool fields into the deprecated Tools field
	for i := range app.Config.Helix.Assistants {
		assistant := &app.Config.Helix.Assistants[i]
		var tools []*types.Tool

		// Convert APIs to Tools
		for _, api := range assistant.APIs {
			t, err := ConvertAPIToTool(api)
			if err != nil {
				return nil, err
			}
			tools = append(tools, t)
		}

		// Convert Zapier to Tools
		for _, zapier := range assistant.Zapier {
			tools = append(tools, &types.Tool{
				Name:        zapier.Name,
				Description: zapier.Description,
				ToolType:    types.ToolTypeZapier,
				Config: types.ToolConfig{
					Zapier: &types.ToolZapierConfig{
						APIKey:        zapier.APIKey,
						Model:         zapier.Model,
						MaxIterations: zapier.MaxIterations,
					},
				},
			})
		}

		// Convert GPTScripts to Tools
		for _, script := range assistant.GPTScripts {
			tools = append(tools, &types.Tool{
				Name:        script.Name,
				Description: script.Description,
				ToolType:    types.ToolTypeGPTScript,
				Config: types.ToolConfig{
					GPTScript: &types.ToolGPTScriptConfig{
						Script: script.Content,
					},
				},
			})
		}

		assistant.Tools = tools
	}

	return app, nil
}

func (s *PostgresStore) ListApps(ctx context.Context, q *ListAppsQuery) ([]*types.App, error) {
	var apps []*types.App

	// Build the query conditionally based on the query parameters
	query := s.gdb.WithContext(ctx)

	// Add owner and owner type conditions if provided
	if q.Owner != "" {
		query = query.Where("owner = ?", q.Owner)
	}

	if q.OwnerType != "" {
		query = query.Where("owner_type = ?", q.OwnerType)
	}

	// Handle global flag
	if q.Global {
		query = query.Where("global = ?", q.Global)
	}

	// Handle organization_id based on specific conditions
	if q.OrganizationID != "" {
		query = query.Where("organization_id = ?", q.OrganizationID)
	} else {
		query = query.Where("organization_id IS NULL OR organization_id = ''")
	}

	// Execute the query
	err := query.Order("id DESC").Find(&apps).Error
	if err != nil {
		return nil, err
	}

	setAppDefaults(apps...)

	return apps, nil
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
