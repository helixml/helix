package runner

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/inhies/go-bytesize"
	"github.com/rs/zerolog/log"
)

type Options struct {
	ID       string
	APIHost  string
	APIToken string

	CacheDir string

	Config *config.RunnerConfig

	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/task/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/task
	TaskURL string
	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/session/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/session
	InitialSessionURL string

	// add this model to the global session query filter
	// so we only run a single model
	FilterModelName string

	// if we only want to run fine-tuning or inference
	// set this and it will be added to the global session filter
	FilterMode string

	// do we want to allow multiple models of the same type to run on this GPU?
	AllowMultipleCopies bool

	// how long to wait between loops for the controller
	// this will affect how often we ask for a global session
	GetTaskDelayMilliseconds int

	// how often to report our overal state to the api
	ReportStateDelaySeconds int

	// how many bytes of memory does our GPU have?
	// we report this back to the api when we ask
	// for the global next task (well, this minus the
	// currently running models)
	MemoryBytes uint64
	// if this is defined then we convert it usng
	// github.com/inhies/go-bytesize
	MemoryString string

	Labels map[string]string

	SchedulingDecisionBufferSize int
	JobHistoryBufferSize         int

	// used when we are developing platform code without a GPU
	// it will run local python scripts that fake the output
	MockRunner bool
	// if this is defined then we will throw an error for any jobs
	// the error will be the value of this string
	MockRunnerError string

	// how many seconds to delay the mock runner
	MockRunnerDelay int

	// development settings
	// never run more than this number of model instances
	MaxModelInstances int

	WebServer WebServer
}

type WebServer struct {
	Host string `envconfig:"SERVER_HOST" default:"127.0.0.1" description:"The host to bind the api server to."`
	Port int    `envconfig:"SERVER_PORT" default:"80" description:""`
}

type Runner struct {
	Ctx                   context.Context
	Options               Options
	httpClientOptions     system.ClientOptions
	websocketEventChannel chan *types.WebsocketEvent // how we write web sockets messages to the api server
	slots                 map[uuid.UUID]*Slot        // A map recording the slots running on this runner
	server                *HelixRunnerAPIServer
	pubsub                pubsub.PubSub
}

func NewRunner(
	ctx context.Context,
	options Options,
) (*Runner, error) {

	if options.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if options.APIHost == "" {
		return nil, fmt.Errorf("api host required")
	}
	if options.APIToken == "" {
		return nil, fmt.Errorf("api token is required")
	}

	// Remove trailing slash from ApiHost if present
	options.APIHost = strings.TrimSuffix(options.APIHost, "/")

	if options.MemoryString != "" {
		bytes, err := bytesize.Parse(options.MemoryString)
		if err != nil {
			return nil, err
		}
		log.Info().Msgf("Setting memoryBytes = %d", uint64(bytes))
		options.MemoryBytes = uint64(bytes)
	}
	if options.MemoryBytes == 0 {
		return nil, fmt.Errorf("memory is required")
	}

	server, err := NewHelixRunnerAPIServer(ctx, &options)
	if err != nil {
		return nil, err
	}

	// Build the URL to the nats server
	clientOptions := system.ClientOptions{
		Host:  options.APIHost,
		Token: options.APIToken,
	}
	// natsAPIPath := system.GetAPIPath(fmt.Sprintf("/runner/%s/ws", options.ID))
	natsServerURL := system.WSURL(clientOptions, "/test")
	log.Info().Msgf("Connecting to nats server: %s", natsServerURL)
	ps, err := pubsub.NewNatsClient(natsServerURL, options.APIToken)
	if err != nil {
		return nil, err
	}

	runner := &Runner{
		Ctx:     ctx,
		Options: options,
		httpClientOptions: system.ClientOptions{
			Host:  options.APIHost,
			Token: options.APIToken,
		},
		websocketEventChannel: make(chan *types.WebsocketEvent),
		slots:                 make(map[uuid.UUID]*Slot),
		server:                server,
		pubsub:                ps,
	}
	return runner, nil
}

func (r *Runner) Run(ctx context.Context) {
	log.Info().Msgf("Starting runner server on %s:%d", r.Options.WebServer.Host, r.Options.WebServer.Port)
	go func() {
		err := r.server.ListenAndServe(ctx, nil)
		if err != nil {
			panic(err)
		}
	}()

	log.Info().Str("runner_id", r.Options.ID).Msg("Starting NATS controller")
	go func() {
		serverURL := fmt.Sprintf("http://%s:%d", r.Options.WebServer.Host, r.Options.WebServer.Port)
		_, err := NewNatsController(ctx, &NatsControllerConfig{
			PS:        r.pubsub,
			ServerURL: serverURL,
			RunnerID:  r.Options.ID,
		})
		if err != nil {
			panic(err)
		}
		<-ctx.Done()
	}()

}
