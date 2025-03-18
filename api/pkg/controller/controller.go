package controller

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/gptscript"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
)

type Options struct {
	Config            *config.ServerConfig
	Store             store.Store
	PubSub            pubsub.PubSub
	Extractor         extract.Extractor
	RAG               rag.RAG
	GPTScriptExecutor gptscript.Executor
	Filestore         filestore.FileStore
	Janitor           *janitor.Janitor
	Notifier          notification.Notifier
	// OpenAIClient         openai.Client
	ProviderManager      manager.ProviderManager
	DataprepOpenAIClient openai.Client
	Scheduler            *scheduler.Scheduler
	RunnerController     *scheduler.RunnerController
}

type Controller struct {
	Ctx          context.Context
	Options      Options
	ToolsPlanner tools.Planner

	providerManager manager.ProviderManager

	dataprepOpenAIClient openai.Client

	newRagClient func(settings *types.RAGSettings) rag.RAG

	// keep a map of instantiated models so we can ask it about memory
	// the models package looks after instantiating this for us
	models map[string]model.Model

	// the current buffer of scheduling decisions
	schedulingDecisions []*types.GlobalSchedulingDecision

	scheduler *scheduler.Scheduler
}

func NewController(
	ctx context.Context,
	options Options,
) (*Controller, error) {
	if options.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if options.Filestore == nil {
		return nil, fmt.Errorf("filestore is required")
	}
	if options.Extractor == nil {
		return nil, fmt.Errorf("text extractor is required")
	}
	if options.Janitor == nil {
		return nil, fmt.Errorf("janitor is required")
	}
	if options.ProviderManager == nil {
		return nil, fmt.Errorf("provider manager is required")
	}

	models, err := model.GetModels()
	if err != nil {
		return nil, err
	}

	controller := &Controller{
		Ctx:                  ctx,
		Options:              options,
		providerManager:      options.ProviderManager,
		dataprepOpenAIClient: options.DataprepOpenAIClient,
		models:               models,
		newRagClient: func(settings *types.RAGSettings) rag.RAG {
			return rag.NewLlamaindex(settings)
		},
		schedulingDecisions: []*types.GlobalSchedulingDecision{},
		scheduler:           options.Scheduler,
	}

	// Default provider
	toolsOpenAIClient, err := controller.getClient(ctx, "", options.Config.Inference.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get tools client: %v", err)
	}

	planner, err := tools.NewChainStrategy(options.Config, options.Store, options.GPTScriptExecutor, toolsOpenAIClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create tools planner: %v", err)
	}

	controller.ToolsPlanner = planner

	return controller, nil
}

func (c *Controller) Initialize() error {
	return nil
}

// AuthorizeUserToApp checks if a user has access to an app
// This is a controller-level method that mirrors the server's authorizeUserToApp function
func (c *Controller) AuthorizeUserToApp(ctx context.Context, user *types.User, app *types.App) error {
	// Check direct ownership
	if user.ID == app.Owner {
		return nil
	}

	// Check if app is global
	if app.Global {
		return nil
	}

	// Organization apps require membership checks
	if app.OrganizationID != "" {
		// Check organization membership
		orgMembership, err := c.Options.Store.GetOrganizationMembership(ctx, &store.GetOrganizationMembershipQuery{
			OrganizationID: app.OrganizationID,
			UserID:         user.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to get organization membership: %w", err)
		}

		// Organization owners always have access
		if orgMembership.Role == types.OrganizationRoleOwner {
			return nil
		}

		// Check access grants
		grants, err := c.Options.Store.ListAccessGrants(ctx, &store.ListAccessGrantsQuery{
			OrganizationID: app.OrganizationID,
			UserID:         user.ID,
			ResourceType:   types.ResourceApplication,
			ResourceID:     app.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to list access grants: %w", err)
		}

		// If user has any grants, allow access
		if len(grants) > 0 {
			return nil
		}

		// Check team-based access
		teams, err := c.Options.Store.ListTeams(ctx, &store.ListTeamsQuery{
			OrganizationID: app.OrganizationID,
			UserID:         user.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to list teams: %w", err)
		}

		var teamIDs []string
		for _, team := range teams {
			teamIDs = append(teamIDs, team.ID)
		}

		if len(teamIDs) > 0 {
			teamGrants, err := c.Options.Store.ListAccessGrants(ctx, &store.ListAccessGrantsQuery{
				OrganizationID: app.OrganizationID,
				ResourceType:   types.ResourceApplication,
				ResourceID:     app.ID,
				TeamIDs:        teamIDs,
			})
			if err != nil {
				return fmt.Errorf("failed to list team access grants: %w", err)
			}

			if len(teamGrants) > 0 {
				return nil
			}
		}
	}

	return fmt.Errorf("you do not have access to the app with the id: %s", app.ID)
}
