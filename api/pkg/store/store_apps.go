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

// RectifyApp handles the migration of app configurations from the old format (which used both
// Tools and specific fields like APIs, GPTScripts, Zapier) to a new canonical format where
// tools are only stored in their specific fields (APIs, GPTScripts, Zapier).
//
// This function:
//  1. Processes any tools found in the deprecated Tools field and converts them to their
//     appropriate specific fields (APIs, GPTScripts, Zapier)
//  2. Handles deduplication by name - if a tool already exists in a specific field
//     (e.g., in APIs), it won't be duplicated from the Tools field
//  3. Gives precedence to tools defined in their specific fields over those in the Tools field
//  4. Clears the Tools field after processing (as it's now deprecated)
//
// This allows us to handle old database records that might have tools defined in either or both places,
// while ensuring we move forward with a clean, consistent format where tools are only stored in
// their specific fields.
func RectifyApp(app *types.App) {
	for i := range app.Config.Helix.Assistants {
		assistant := &app.Config.Helix.Assistants[i]

		// Create maps to track existing tools by name
		existingAPIs := make(map[string]bool)
		existingGPTScripts := make(map[string]bool)
		existingZapier := make(map[string]bool)

		// First mark all existing non-Tools items
		for _, api := range assistant.APIs {
			existingAPIs[api.Name] = true
		}
		for _, script := range assistant.GPTScripts {
			existingGPTScripts[script.Name] = true
		}
		for _, zapier := range assistant.Zapier {
			existingZapier[zapier.Name] = true
		}

		// Convert tools to their appropriate fields
		// but only if they don't already exist in the non-Tools fields
		for _, tool := range assistant.Tools {
			switch tool.ToolType {
			case types.ToolTypeAPI:
				if !existingAPIs[tool.Name] && tool.Config.API != nil {
					assistant.APIs = append(assistant.APIs, types.AssistantAPI{
						Name:                    tool.Name,
						Description:             tool.Description,
						URL:                     tool.Config.API.URL,
						Schema:                  tool.Config.API.Schema,
						Headers:                 tool.Config.API.Headers,
						Query:                   tool.Config.API.Query,
						RequestPrepTemplate:     tool.Config.API.RequestPrepTemplate,
						ResponseSuccessTemplate: tool.Config.API.ResponseSuccessTemplate,
						ResponseErrorTemplate:   tool.Config.API.ResponseErrorTemplate,
					})
					existingAPIs[tool.Name] = true
				}
			case types.ToolTypeGPTScript:
				if !existingGPTScripts[tool.Name] && tool.Config.GPTScript != nil {
					assistant.GPTScripts = append(assistant.GPTScripts, types.AssistantGPTScript{
						Name:        tool.Name,
						Description: tool.Description,
						Content:     tool.Config.GPTScript.Script,
					})
					existingGPTScripts[tool.Name] = true
				}
			case types.ToolTypeZapier:
				if !existingZapier[tool.Name] && tool.Config.Zapier != nil {
					assistant.Zapier = append(assistant.Zapier, types.AssistantZapier{
						Name:          tool.Name,
						Description:   tool.Description,
						APIKey:        tool.Config.Zapier.APIKey,
						Model:         tool.Config.Zapier.Model,
						MaxIterations: tool.Config.Zapier.MaxIterations,
					})
					existingZapier[tool.Name] = true
				}
			}
		}

		// Clear the tools field as it's now deprecated
		assistant.Tools = nil
	}
}

func (s *PostgresStore) CreateApp(ctx context.Context, app *types.App) (*types.App, error) {
	if app.ID == "" {
		app.ID = system.GenerateAppID()
	}

	if app.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	app.Created = time.Now()

	setAppDefaults(app)
	RectifyApp(app)

	err := s.gdb.WithContext(ctx).Create(app).Error
	if err != nil {
		return nil, err
	}
	return s.GetApp(ctx, app.ID)
}

func (s *PostgresStore) UpdateApp(ctx context.Context, app *types.App) (*types.App, error) {
	if app.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if app.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	app.Updated = time.Now()

	RectifyApp(app)

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

	// Check if any tools need to be rectified
	hasTools := false
	for _, assistant := range app.Config.Helix.Assistants {
		if len(assistant.Tools) > 0 {
			hasTools = true
			break
		}
	}

	// If we found tools, rectify and save back to database
	if hasTools {
		RectifyApp(&app)
		err = s.gdb.WithContext(ctx).Save(&app).Error
		if err != nil {
			return nil, fmt.Errorf("error saving rectified app: %w", err)
		}
	}

	return &app, nil
}

// BACKWARD COMPATIBILITY ONLY: return an app with the apis, gptscripts, and zapier
// transformed into the deprecated Tools field
func (s *PostgresStore) GetAppWithTools(ctx context.Context, id string) (*types.App, error) {
	app, err := s.GetApp(ctx, id)
	if err != nil {
		return nil, err
	}

	// Convert each assistant's specific tool fields into the deprecated Tools field
	for i := range app.Config.Helix.Assistants {
		assistant := &app.Config.Helix.Assistants[i]
		var tools []*types.Tool

		// Convert APIs to Tools
		for _, api := range assistant.APIs {
			tools = append(tools, &types.Tool{
				Name:        api.Name,
				Description: api.Description,
				ToolType:    types.ToolTypeAPI,
				Config: types.ToolConfig{
					API: &types.ToolApiConfig{
						URL:                     api.URL,
						Schema:                  api.Schema,
						Headers:                 api.Headers,
						Query:                   api.Query,
						RequestPrepTemplate:     api.RequestPrepTemplate,
						ResponseSuccessTemplate: api.ResponseSuccessTemplate,
						ResponseErrorTemplate:   api.ResponseErrorTemplate,
					},
				},
			})
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
	err := s.gdb.WithContext(ctx).Where(&types.App{
		Owner:     q.Owner,
		OwnerType: q.OwnerType,
		Global:    q.Global,
	}).Order("id DESC").Find(&apps).Error
	if err != nil {
		return nil, err
	}

	setAppDefaults(apps...)

	// Check and rectify any apps that have tools
	for _, app := range apps {
		hasTools := false
		for _, assistant := range app.Config.Helix.Assistants {
			if len(assistant.Tools) > 0 {
				hasTools = true
				break
			}
		}

		if hasTools {
			RectifyApp(app)
			err = s.gdb.WithContext(ctx).Save(app).Error
			if err != nil {
				return nil, fmt.Errorf("error saving rectified app: %w", err)
			}
		}
	}

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
