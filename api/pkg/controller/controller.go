package controller

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

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
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog/log"
)

type Options struct {
	Config               *config.ServerConfig
	Store                store.Store
	PubSub               pubsub.PubSub
	Extractor            extract.Extractor
	RAG                  rag.RAG
	GPTScriptExecutor    gptscript.Executor
	Filestore            filestore.FileStore
	Janitor              *janitor.Janitor
	Notifier             notification.Notifier
	ProviderManager      manager.ProviderManager
	DataprepOpenAIClient openai.Client
	Scheduler            scheduler.Scheduler
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

	// the map of model instances that we have loaded
	// and are currently running
	activeRunners *xsync.MapOf[string, *types.RunnerState]

	// the current buffer of scheduling decisions
	schedulingDecisions []*types.GlobalSchedulingDecision

	scheduler scheduler.Scheduler
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
		activeRunners:       xsync.NewMapOf[string, *types.RunnerState](),
		schedulingDecisions: []*types.GlobalSchedulingDecision{},
		scheduler:           options.Scheduler,
	}

	toolsOpenAIClient, err := controller.getClient(ctx, options.Config.Inference.Provider)
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

// this should be run in a go-routine
func (c *Controller) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			err := c.run(c.Ctx)
			if err != nil {
				log.Error().Msgf("error in controller loop: %s", err.Error())
				debug.PrintStack()
			}
		}
	}
}

func (c *Controller) run(ctx context.Context) error {
	err := c.cleanOldRunnerMetrics(ctx)
	if err != nil {
		log.Error().Msgf("error in controller loop: %s", err.Error())
		debug.PrintStack()
	}
	return nil
}
