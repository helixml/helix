package runner

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// a long running instance of a loaded into memory model
// that can run multiple session tasks sequentially
// we keep state of the active text stream (if the model supports it)
// and are in charge of sending updates out of the model to the api
// to update it's state
type ModelInstance struct {
	id string

	model  model.Model
	filter types.SessionFilter

	finishChan chan bool

	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/task/:instanceid
	taskURL string
	// e.g. http://localhost:8080/api/v1/worker/response/:instanceid
	responseURL string

	// we write responses to this function and they will be sent to the api
	responseHandler func(res *types.WorkerTaskResponse) error

	// we create a cancel context for the running process
	// which is derived from the main runner context
	ctx context.Context
	// the process running the python model will quit when we call this
	processCancelFunc context.CancelFunc

	// the session currently running on this model
	currentSession *types.Session

	// the currently active text stream if the model needs it
	// this is assigned by calling model.GetTextStream(mode) on the model
	// instance - this means models get to decide if/when they need text stream processing
	currentTextStream *model.TextStream

	// the function we call to stop the current text stream if we have one
	textStreamCancelFunc context.CancelFunc

	// the very first session that we will run - the precense of which caused
	// use to be instantiated - this will get switched to currentSession
	// as though it had been loaded from api as normal
	initialSession *types.Session

	// basically the timestamp of when the last job was finished
	lastJobCompletedTimestamp int64
}

func NewModelInstance(
	// this is the main runner CLI context
	modelName types.ModelName,
	mode types.SessionMode,
	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/task/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/task
	taskURL string,
	// e.g. http://localhost:8080/api/v1/worker/response/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/response
	responseURL string,

	responseHandler func(res *types.WorkerTaskResponse) error,
) (*ModelInstance, error) {
	modelInstance, err := model.GetModel(modelName)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	id := system.GenerateUUID()
	return &ModelInstance{
		id:              id,
		finishChan:      make(chan bool, 1),
		model:           modelInstance,
		responseHandler: responseHandler,
		taskURL:         fmt.Sprintf("%s/%s", taskURL, id),
		responseURL:     fmt.Sprintf("%s/%s", responseURL, id),
		filter: types.SessionFilter{
			ModelName: modelName,
			Mode:      mode,
		},
		ctx:               ctx,
		processCancelFunc: cancel,
	}, nil
}

func getLastInteractionID(session *types.Session) (string, error) {
	if len(session.Interactions) == 0 {
		return "", fmt.Errorf("session has no messages")
	}
	interaction := session.Interactions[len(session.Interactions)-1]
	if interaction.Creator != types.CreatorTypeUser {
		return "", fmt.Errorf("session does not have user interaction as last message")
	}
	return interaction.ID, nil
}

// this is the loading of a session onto a running model instance
// it gets the text stream setup if the model returns one
// and generally initializes a new task to be run on the model
// it also returns the task that will be fed down into the python code to execute
func (instance *ModelInstance) assignCurrentSession(session *types.Session) (*types.WorkerTask, error) {
	if instance.textStreamCancelFunc != nil {
		instance.textStreamCancelFunc()
		instance.textStreamCancelFunc = nil
	}
	instance.currentSession = session

	interactionID, err := getLastInteractionID(session)
	if err != nil {
		return nil, err
	}

	textStream, err := instance.model.GetTextStream(instance.filter.Mode)
	if err != nil {
		return nil, err
	}

	if textStream != nil {
		ctx, cancel := context.WithCancel(instance.ctx)
		instance.textStreamCancelFunc = cancel
		instance.currentTextStream = textStream

		go textStream.Start(ctx)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-textStream.Output:
					err := instance.responseHandler(&types.WorkerTaskResponse{
						Type:          types.WorkerTaskResponseTypeStream,
						SessionID:     instance.currentSession.ID,
						InteractionID: interactionID,
						Message:       textStream.Buffer,
					})
					if err != nil {
						log.Error().Msgf("Error sending WorkerTaskResponse: %s", err.Error())
					}
				}
			}
		}()
	}

	task, err := instance.model.GetTask(session)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (instance *ModelInstance) handleStream(ctx context.Context, taskResponse *types.WorkerTaskResponse) error {
	if instance.currentSession == nil {
		return fmt.Errorf("no current session")
	}
	if instance.currentTextStream == nil {
		return fmt.Errorf("no text stream to continue")
	}
	instance.currentTextStream.Write([]byte(taskResponse.Message))
	return nil
}

func (instance *ModelInstance) handleResult(ctx context.Context, taskResponse *types.WorkerTaskResponse) error {
	if instance.currentSession == nil {
		return fmt.Errorf("no current session")
	}
	if taskResponse.SessionID == "" {
		return fmt.Errorf("no session ID")
	}
	if taskResponse.SessionID != instance.currentSession.ID {
		return fmt.Errorf("session ID mismatch")
	}

	// reset the text stream if we have one
	if instance.currentTextStream != nil && instance.textStreamCancelFunc != nil {
		instance.textStreamCancelFunc()
		instance.textStreamCancelFunc = nil
		instance.currentTextStream = nil
	}

	// we update the timeout timestamp
	instance.lastJobCompletedTimestamp = time.Now().Unix()

	interactionID, err := getLastInteractionID(instance.currentSession)
	if err != nil {
		return err
	}

	// inject the interaction ID into the response
	// this means the python code never needs to worry about
	// feeding interaction ids back to us
	taskResponseCopy := *taskResponse
	taskResponseCopy.InteractionID = interactionID

	err = instance.responseHandler(&taskResponseCopy)
	if err != nil {
		return err
	}

	instance.currentSession = nil
	return nil
}

// run the model process
// we pass the instance context in so we can cancel it using our stopProcess function
func (instance *ModelInstance) startProcess() error {
	cmd, err := instance.model.GetCommand(instance.ctx, instance.filter.Mode, types.RunnerProcessConfig{
		InstanceID:  instance.id,
		TaskURL:     instance.taskURL,
		ResponseURL: instance.responseURL,
	})

	log.Info().
		Msgf("🟢 run model instance")
	spew.Dump(cmd.Dir)
	spew.Dump(cmd.Args)

	if err != nil {
		return err
	}
	if cmd == nil {
		return fmt.Errorf("no command to run")
	}
	go func(cmd *exec.Cmd) {
		if err := cmd.Start(); err != nil {
			log.Error().Msgf("Failed to start command: %v\n", err.Error())
			return
		}

		if err := cmd.Wait(); err != nil {
			log.Error().Msgf("Failed to wait for command: %v\n", err.Error())
		}

		log.Info().
			Msgf("🟢 stop model instance, exit code=%d", cmd.ProcessState.ExitCode())

		instance.finishChan <- true
	}(cmd)
	return nil
}

func (model *ModelInstance) stopProcess() error {
	if model.processCancelFunc == nil {
		return fmt.Errorf("no process to stop")
	}
	model.processCancelFunc()
	return nil
}
