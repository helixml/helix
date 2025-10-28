package controller

import (
	"context"
	"fmt"
	"sync"

	"github.com/dgraph-io/ristretto/v2"
	agent "github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/agent/skill/mcp"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/gptscript"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/searxng"
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
	ProviderManager   manager.ProviderManager // OpenAI client provider
	// DataprepOpenAIClient openai.Client
	Scheduler        *scheduler.Scheduler
	RunnerController *scheduler.RunnerController
	OAuthManager     *oauth.Manager
	Browser          *browser.Browser
	SearchProvider   searxng.SearchProvider
	MCPClientGetter  mcp.ClientGetter
}

type Controller struct {
	Ctx          context.Context
	Options      Options
	ToolsPlanner tools.Planner

	providerManager manager.ProviderManager

	// dataprepOpenAIClient openai.Client

	newRagClient func(settings *types.RAGSettings) rag.RAG

	// keep a map of instantiated models so we can ask it about memory
	// the models package looks after instantiating this for us
	models map[string]model.Model

	// the current buffer of scheduling decisions
	schedulingDecisions []*types.GlobalSchedulingDecision

	scheduler *scheduler.Scheduler

	stepInfoEmitter agent.StepInfoEmitter

	// Keeping a map of trigger statuses for each app/trigger type,
	// this is done in-memory to ensure the status is always live
	triggerStatuses   map[TriggerStatusKey]types.TriggerStatus
	triggerStatusesMu *sync.RWMutex

	// Memory estimation service for calculating model memory requirements
	memoryEstimationService *MemoryEstimationService

	browserCache *ristretto.Cache[string, string]
}

type TriggerStatusKey struct {
	AppID string
	Type  types.TriggerType
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

	if options.MCPClientGetter == nil {
		// Initialize default client getter
		options.MCPClientGetter = &mcp.DefaultClientGetter{
			TLSSkipVerify: options.Config.Tools.TLSSkipVerify,
		}
	}

	models, err := model.GetModels()
	if err != nil {
		return nil, err
	}

	browserCache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create browser cache: %w", err)
	}

	controller := &Controller{
		Ctx:             ctx,
		Options:         options,
		providerManager: options.ProviderManager,
		models:          models,
		newRagClient: func(settings *types.RAGSettings) rag.RAG {
			return rag.NewLlamaindex(settings)
		},
		schedulingDecisions: []*types.GlobalSchedulingDecision{},
		scheduler:           options.Scheduler,
		stepInfoEmitter:     agent.NewPubSubStepInfoEmitter(options.PubSub, options.Store),
		triggerStatuses:     make(map[TriggerStatusKey]types.TriggerStatus),
		triggerStatusesMu:   &sync.RWMutex{},
		browserCache:        browserCache,
	}

	// Default provider
	toolsOpenAIClient, err := controller.getClient(ctx, "", "", options.Config.Inference.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get tools client: %v", err)
	}

	planner, err := tools.NewChainStrategy(options.Config, options.Store, options.GPTScriptExecutor, toolsOpenAIClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create tools planner: %v", err)
	}

	controller.ToolsPlanner = planner

	// Initialize OAuth manager and stores for the ChainStrategy planner
	tools.InitChainStrategyOAuth(planner, options.OAuthManager, options.Store, options.Store)

	// Initialize memory estimation service
	if options.RunnerController != nil {
		modelProvider := NewStoreModelProvider(options.Store)
		controller.memoryEstimationService = NewMemoryEstimationService(
			options.RunnerController, // Implements RunnerSender interface
			modelProvider,            // Wrapped store implementing ModelProvider interface
		)

		// Start background cache refresh
		// controller.memoryEstimationService.StartBackgroundCacheRefresh(ctx) // DISABLED FOR DEBUGGING
		// controller.memoryEstimationService.StartCacheCleanup(ctx) // DISABLED FOR DEBUGGING
	}

	return controller, nil
}

func (c *Controller) Initialize() error {
	return nil
}

// Close cleans up all resources used by the controller
func (c *Controller) Close() error {
	// Stop memory estimation service
	if c.memoryEstimationService != nil {
		c.memoryEstimationService.StopBackgroundCacheRefresh()
	}

	// Close browser if present
	if c.Options.Browser != nil {
		c.Options.Browser.Close()
	}

	// Close PubSub connections if present
	if c.Options.PubSub != nil {
		if natsClient, ok := c.Options.PubSub.(*pubsub.Nats); ok {
			natsClient.Close()
		}
	}

	// Close store connections if present
	if c.Options.Store != nil {
		if closer, ok := c.Options.Store.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				return fmt.Errorf("failed to close store: %w", err)
			}
		}
	}

	return nil
}

// GetMemoryEstimationService returns the memory estimation service
func (c *Controller) GetMemoryEstimationService() *MemoryEstimationService {
	return c.memoryEstimationService
}

func (c *Controller) SetTriggerStatus(appID string, triggerType types.TriggerType, status types.TriggerStatus) {
	c.triggerStatusesMu.Lock()
	defer c.triggerStatusesMu.Unlock()

	c.triggerStatuses[TriggerStatusKey{AppID: appID, Type: triggerType}] = status
}

func (c *Controller) GetTriggerStatus(appID string, triggerType types.TriggerType) (types.TriggerStatus, bool) {
	c.triggerStatusesMu.RLock()
	defer c.triggerStatusesMu.RUnlock()

	status, ok := c.triggerStatuses[TriggerStatusKey{AppID: appID, Type: triggerType}]
	return status, ok
}
