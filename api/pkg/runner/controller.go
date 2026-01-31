package runner

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/inhies/go-bytesize"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

type Options struct {
	ID       string
	APIHost  string
	APIToken string

	CacheDir string

	Config *config.RunnerConfig

	// add this model to the global session query filter
	// so we only run a single model
	FilterModelName string

	// if we only want to run fine-tuning or inference
	// set this and it will be added to the global session filter
	FilterMode string

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

	// When set to true, allows running without a GPU for development/CI purposes
	// Will use system memory for scheduling
	// Should not be used in production as CPU inference will be very slow
	DevelopmentCPUOnly bool `envconfig:"DEVELOPMENT_CPU_ONLY" default:"false"`

	// development settings
	// never run more than this number of model instances
	// MaxModelInstances int

	WebServer WebServer

	MaxPullConcurrency int `envconfig:"RUNNER_MAX_PULL_CONCURRENCY" default:"4"`
}

type WebServer struct {
	Host string `envconfig:"SERVER_HOST" default:"127.0.0.1" description:"The host to bind the api server to."`
	Port int    `envconfig:"SERVER_PORT" default:"8080" description:"The port to bind the api server to."`
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

	server, err := NewHelixRunnerAPIServer(ctx, &options)
	if err != nil {
		return nil, err
	}

	natsServerURL := system.WSURL(system.ClientOptions{
		Host:  options.APIHost,
		Token: options.APIToken,
	}, system.GetAPIPath("/runner/"+options.ID+"/ws"))
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
	pool := pool.New().WithErrors()

	log.Info().Msgf("Starting runner server on %s:%d", r.Options.WebServer.Host, r.Options.WebServer.Port)

	pool.Go(func() error {
		err := r.server.ListenAndServe(ctx, nil)
		if err != nil {
			return err
		}
		return nil
	})

	log.Info().Str("runner_id", r.Options.ID).Msg("Starting NATS controller")

	pool.Go(func() error {
		serverURL := fmt.Sprintf("http://%s:%d", r.Options.WebServer.Host, r.Options.WebServer.Port)

		// Add retry logic for connecting to NATS
		maxRetries := 10
		backoff := 2 * time.Second
		var err error

		for i := 0; i < maxRetries; i++ {
			log.Info().
				Str("runner_id", r.Options.ID).
				Int("attempt", i+1).
				Int("max_retries", maxRetries).
				Msg("Attempting to connect to NATS")

			_, err = NewNatsController(ctx, &NatsControllerConfig{
				PS:        r.pubsub,
				ServerURL: serverURL,
				RunnerID:  r.Options.ID,
			})

			if err == nil {
				log.Info().
					Str("runner_id", r.Options.ID).
					Msg("Successfully connected to NATS")
				break
			}

			log.Warn().
				Err(err).
				Str("runner_id", r.Options.ID).
				Int("attempt", i+1).
				Int("max_retries", maxRetries).
				Dur("retry_after", backoff).
				Msg("Failed to connect to NATS, retrying...")

			select {
			case <-ctx.Done():
				log.Warn().Msg("Context cancelled while retrying NATS connection")
				return fmt.Errorf("context cancelled while retrying NATS connection")
			case <-time.After(backoff):
				// Exponential backoff with a maximum of 30 seconds
				backoff = time.Duration(math.Min(float64(backoff)*1.5, 30*float64(time.Second)))
			}
		}

		if err != nil {
			log.Error().
				Err(err).
				Str("runner_id", r.Options.ID).
				Int("max_retries", maxRetries).
				Msg("Failed to connect to NATS after multiple attempts")
			return fmt.Errorf("failed to connect to NATS after %d attempts: %v", maxRetries, err)
		}

		return nil
	})

	// Skip model reconciler in mock runner mode
	if !r.Options.MockRunner {
		pool.Go(func() error {
			log.Info().Msg("starting helix model reconciler")
			err := r.startHelixModelReconciler(ctx)
			if err != nil {
				log.Error().Err(err).Msg("error starting helix model reconciler")
			}
			return err
		})
	} else {
		log.Info().Msg("skipping helix model reconciler in mock runner mode")
	}

	err := pool.Wait()
	if err != nil {
		log.Error().Err(err).Msg("error running runner")
	}
}
