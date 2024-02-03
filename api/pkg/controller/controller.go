package controller

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/dataprep/text"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog/log"
)

type ControllerOptions struct {
	Store                  store.Store
	Planner                tools.Planner
	Filestore              filestore.FileStore
	FilestorePresignSecret string
	Janitor                *janitor.Janitor
	DataPrepTextFactory    func(session *types.Session) (text.DataPrepTextQuestionGenerator, *text.DataPrepTextSplitter, error)
	// this is an "env" prefix like "dev"
	// the user prefix is handled inside the controller
	// (see getFilestorePath)
	FilePrefixGlobal string
	// this is a golang template that is used to prefix the user
	// path in the filestore - it is passed Owner and OwnerType values
	// write me an example FilePrefixUser as a go template
	// e.g. "users/{{.Owner}}"
	FilePrefixUser string

	FilePrefixSessions string
	// a static path used to denote what sub-folder job results live in
	FilePrefixResults string

	// the URL we post documents to so we can get the text back from them
	TextExtractionURL string

	// how many scheduler decisions to buffer before we start dropping them
	SchedulingDecisionBufferSize int

	Notifier notification.Notifier
}

type Controller struct {
	Ctx     context.Context
	Options ControllerOptions

	// this is used to WRITE events to browsers
	UserWebsocketEventChanWriter chan *types.WebsocketEvent

	// this is used to READ events from runners
	RunnerWebsocketEventChanReader chan *types.WebsocketEvent

	// the backlog of sessions that need a GPU
	sessionQueue []*types.Session
	// we keep this managed to avoid having to lock the queue mutex
	// whilst we calculate all the summaries
	sessionSummaryQueue []*types.SessionSummary
	sessionQueueMtx     sync.Mutex

	// keep a map of instantiated models so we can ask it about memory
	// the models package looks after instantiating this for us
	models map[types.ModelName]model.Model

	// the map of model instances that we have loaded
	// and are currently running
	activeRunners *xsync.MapOf[string, *types.RunnerState]

	// the current buffer of scheduling decisions
	schedulingDecisions []*types.GlobalSchedulingDecision
}

func NewController(
	ctx context.Context,
	options ControllerOptions,
) (*Controller, error) {
	if options.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if options.DataPrepTextFactory == nil {
		return nil, fmt.Errorf("data prep text factory is required")
	}
	if options.Filestore == nil {
		return nil, fmt.Errorf("filestore is required")
	}
	if options.TextExtractionURL == "" {
		return nil, fmt.Errorf("text extraction URL is required")
	}
	if options.Janitor == nil {
		return nil, fmt.Errorf("janitor is required")
	}
	models, err := model.GetModels()
	if err != nil {
		return nil, err
	}

	controller := &Controller{
		Ctx:                            ctx,
		Options:                        options,
		UserWebsocketEventChanWriter:   make(chan *types.WebsocketEvent),
		RunnerWebsocketEventChanReader: make(chan *types.WebsocketEvent),
		sessionQueue:                   []*types.Session{},
		sessionSummaryQueue:            []*types.SessionSummary{},
		models:                         models,
		activeRunners:                  xsync.NewMapOf[string, *types.RunnerState](),
		schedulingDecisions:            []*types.GlobalSchedulingDecision{},
	}
	return controller, nil
}

func (c *Controller) Initialize() error {

	// here we are reading *types.WebsocketEvent from the runner websocket server
	// it's the runners way of saying "here is an update"
	// it is used for "stream" and "progress" events
	// the "result" event is posted to the API (to ensure finality)
	go func() {
		for {
			select {
			case <-c.Ctx.Done():
				return
			case event := <-c.RunnerWebsocketEventChanReader:
				log.Trace().Msgf("Runner websocket event: %+v", *event)
				err := c.BroadcastWebsocketEvent(context.Background(), event)
				if err != nil {
					log.Error().Msgf("Error handling runner websocket event: %s", err.Error())
				}
			}
		}
	}()

	// load the session queue from the database to survive restarts
	err := c.loadSessionQueues(c.Ctx)
	if err != nil {
		return err
	}
	return nil
}

// this should be run in a go-routine
func (c *Controller) StartLooping() {
	for {
		select {
		case <-c.Ctx.Done():
			return
		case <-time.After(10 * time.Second):
			err := c.loop(c.Ctx)
			if err != nil {
				log.Error().Msgf("error in controller loop: %s", err.Error())
				debug.PrintStack()
			}
		}
	}
}

func (c *Controller) loop(ctx context.Context) error {
	err := c.cleanOldRunnerMetrics(ctx)
	if err != nil {
		log.Error().Msgf("error in controller loop: %s", err.Error())
		debug.PrintStack()
	}
	return nil
}
