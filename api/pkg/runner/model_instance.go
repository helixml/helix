package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	urllib "net/url"
	"os"
	"os/exec"
	"path"
	"syscall"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/server"
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

	runnerOptions     RunnerOptions
	httpClientOptions server.ClientOptions

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

	// the command we are currently executing
	currentCommand *exec.Cmd

	// the session that meant this model booted in the first place
	// used to know which lora type file we should download before
	// trying to start this model's python process
	initialSession *types.Session

	// the session currently running on this model
	currentSession *types.Session

	// if there is a value here - it will be fed into the running python
	// process next - it acts as a buffer for a session we want to run right away
	nextSession *types.Session

	// this is the session that we are preparing to run next
	// if there is a value here - then we return nil
	// because there is a task running (e.g. downloading files)
	// that we need to complete before we want this session to run
	queuedSession *types.Session

	// the currently active text stream if the model needs it
	// this is assigned by calling model.GetTextStream(mode) on the model
	// instance - this means models get to decide if/when they need text stream processing
	currentTextStream *model.TextStream

	// the timestamp of when this model instance either completed a job
	// or a new job was pulled and allocated
	// we use this timestamp to cleanup non-active model instances
	lastActivityTimestamp int64
}

func NewModelInstance(
	ctx context.Context,

	// the session that meant this model instance is instantiated
	session *types.Session,
	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/task/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/task
	taskURL string,
	// e.g. http://localhost:8080/api/v1/worker/response/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/response
	responseURL string,

	responseHandler func(res *types.WorkerTaskResponse) error,

	runnerOptions RunnerOptions,
) (*ModelInstance, error) {
	modelInstance, err := model.GetModel(session.ModelName)
	if err != nil {
		return nil, err
	}
	id := system.GenerateUUID()

	// if this is empty string then we need to hoist it to be types.FINETUNE_FILE_NONE
	// because then we are always specifically asking for a session that has no finetune file
	// if we left this blank we are saying "we don't care if it has one or not"
	useFinetuneFile := session.FinetuneFile

	if useFinetuneFile == "" {
		useFinetuneFile = types.FINETUNE_FILE_NONE
	}

	return &ModelInstance{
		id:              id,
		ctx:             ctx,
		finishChan:      make(chan bool, 1),
		model:           modelInstance,
		responseHandler: responseHandler,
		taskURL:         fmt.Sprintf("%s/%s", taskURL, id),
		responseURL:     fmt.Sprintf("%s/%s", responseURL, id),
		initialSession:  session,
		filter: types.SessionFilter{
			ModelName:    session.ModelName,
			Mode:         session.Mode,
			FinetuneFile: useFinetuneFile,
		},
		httpClientOptions: server.ClientOptions{
			Host:  runnerOptions.ApiHost,
			Token: runnerOptions.ApiToken,
		},
	}, nil
}

// this is the loading of a session onto a running model instance
// it gets the text stream setup if the model returns one
// and generally initializes a new task to be run on the model
// it also returns the task that will be fed down into the python code to execute
func (instance *ModelInstance) assignSessionTask(ctx context.Context, session *types.Session) (*types.WorkerTask, error) {
	if instance.currentTextStream != nil {
		instance.currentTextStream.Close(ctx)
		instance.currentTextStream = nil
	}

	// mark the instance as active so it doesn't get cleaned up
	instance.lastActivityTimestamp = time.Now().Unix()
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
		instance.currentTextStream = textStream

		go textStream.Start(ctx)
		go func() {
			for {
				select {
				case <-textStream.Closed:
					return
				case msg := <-textStream.Output:
					err := instance.responseHandler(&types.WorkerTaskResponse{
						Type:          types.WorkerTaskResponseTypeStream,
						SessionID:     instance.currentSession.ID,
						InteractionID: interactionID,
						Message:       msg,
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
	task.SessionID = session.ID
	return task, nil
}

// to queue a session means to put it into a buffer and wait for the Python process to boot up and then "pull" it
func (instance *ModelInstance) queueSession(session *types.Session) {
	instance.queuedSession = session
	instance.nextSession = nil

	log.Debug().
		Msgf("🔵 runner prepare session: %s", session.ID)

	preparedSession, err := instance.downloadInteractionFiles(session)

	if err != nil {
		log.Error().Msgf("error preparing session: %s", err.Error())
		instance.queuedSession = nil
		instance.nextSession = nil
		instance.errorSession(session, err)
		return
	}

	log.Debug().
		Msgf("🔵 runner assign next session: %s", preparedSession.ID)

	instance.queuedSession = nil
	instance.nextSession = preparedSession

}

func (instance *ModelInstance) downloadFinetuneFile(session *types.Session) (*types.Session, error) {
	if session.FinetuneFile == "" {
		return session, nil
	}
	downloadFolder := path.Join(os.TempDir(), "helix", "downloads", session.ID, "finetune_file")
	if err := os.MkdirAll(downloadFolder, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	// filename := path.Base(session.FinetuneFile)
	// url := server.URL(instance.httpClientOptions, fmt.Sprintf("/runner/%s/session/%s/download", instance.runnerOptions.ID, session.ID))

	// urlValues := urllib.Values{}
	// urlValues.Add("path", session.FinetuneFile)

	// fullURL := fmt.Sprintf("%s?%s", url, urlValues.Encode())

	// log.Debug().
	// 	Msgf("🔵 runner downloading interaction file: %s", fullURL)

	return session, nil
}

func (instance *ModelInstance) downloadInteractionFiles(session *types.Session) (*types.Session, error) {
	interaction, err := model.GetUserInteraction(session)
	if err != nil {
		return nil, err
	}

	downloadFolder := path.Join(os.TempDir(), "helix", "downloads", session.ID, interaction.ID)
	if err := os.MkdirAll(downloadFolder, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	remappedFilepaths := []string{}

	for _, filepath := range interaction.Files {
		filename := path.Base(filepath)
		url := server.URL(instance.httpClientOptions, fmt.Sprintf("/runner/%s/session/%s/download", instance.runnerOptions.ID, session.ID))
		urlValues := urllib.Values{}
		urlValues.Add("path", filepath)

		fullURL := fmt.Sprintf("%s?%s", url, urlValues.Encode())

		log.Debug().
			Msgf("🔵 runner downloading interaction file: %s", fullURL)

		req, err := http.NewRequest("GET", fullURL, nil)
		if err != nil {
			return nil, err
		}
		server.AddHeadersVanilla(req, instance.httpClientOptions.Token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code for file download: %d %s", resp.StatusCode, fullURL)
		}

		file, err := os.Create(path.Join(downloadFolder, filename))
		if err != nil {
			return nil, err
		}
		defer file.Close()

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return nil, err
		}

		remappedFilepaths = append(remappedFilepaths, path.Join(downloadFolder, filename))

		log.Debug().
			Msgf("🔵 runner downloaded interaction file: %s -> %s", fullURL, filename)
	}

	interaction.Files = remappedFilepaths

	newInteractions := []types.Interaction{}

	for _, existingInteraction := range session.Interactions {
		if existingInteraction.ID == interaction.ID {
			newInteractions = append(newInteractions, *interaction)
		} else {
			newInteractions = append(newInteractions, existingInteraction)
		}
	}

	session.Interactions = newInteractions

	return session, nil
}

func (instance *ModelInstance) errorSession(session *types.Session, err error) {
	interactionID, getInteractionErr := getLastInteractionID(session)
	if getInteractionErr != nil {
		log.Error().Msgf("Error reporting error to api: %v\n", getInteractionErr.Error())
		return
	}

	apiUpdateErr := instance.responseHandler(&types.WorkerTaskResponse{
		Type:          types.WorkerTaskResponseTypeResult,
		SessionID:     session.ID,
		InteractionID: interactionID,
		Error:         err.Error(),
	})

	if apiUpdateErr != nil {
		log.Error().Msgf("Error reporting error to api: %v\n", apiUpdateErr.Error())
	}
}

func (instance *ModelInstance) handleStream(ctx context.Context, taskResponse *types.WorkerTaskResponse) error {
	if instance.currentSession == nil {
		return fmt.Errorf("no current session")
	}
	if taskResponse.SessionID == "" {
		return fmt.Errorf("no session ID")
	}
	if taskResponse.SessionID != instance.currentSession.ID {
		return fmt.Errorf("session ID mismatch")
	}
	if instance.currentTextStream == nil {
		return fmt.Errorf("no text stream to continue")
	}
	instance.lastActivityTimestamp = time.Now().Unix()
	instance.currentTextStream.Write([]byte(taskResponse.Message))
	return nil
}

func (instance *ModelInstance) handleProgress(ctx context.Context, taskResponse *types.WorkerTaskResponse) error {
	if instance.currentSession == nil {
		return fmt.Errorf("no current session")
	}
	if taskResponse.SessionID == "" {
		return fmt.Errorf("no session ID")
	}
	if taskResponse.SessionID != instance.currentSession.ID {
		return fmt.Errorf("session ID mismatch")
	}
	instance.lastActivityTimestamp = time.Now().Unix()
	interactionID, err := getLastInteractionID(instance.currentSession)
	if err != nil {
		return err
	}
	taskResponseCopy := *taskResponse
	taskResponseCopy.InteractionID = interactionID
	err = instance.responseHandler(&taskResponseCopy)
	if err != nil {
		return err
	}
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
	if instance.currentTextStream != nil {
		instance.currentTextStream.Close(ctx)
		instance.currentTextStream = nil
	}

	// we update the timeout timestamp
	instance.lastActivityTimestamp = time.Now().Unix()

	interactionID, err := getLastInteractionID(instance.currentSession)
	if err != nil {
		return err
	}

	// inject the interaction ID into the response
	// this means the python code never needs to worry about
	// feeding interaction ids back to us
	taskResponseCopy := *taskResponse
	taskResponseCopy.InteractionID = interactionID

	// now we pass the response through the model handler
	// this gives each model a chance to process the result
	// for example, the SDXL model will upload the files to the filestore
	// and turn them into full URLs that can be displayed in the UI

	err = instance.responseHandler(&taskResponseCopy)
	if err != nil {
		return err
	}

	instance.currentSession = nil
	return nil
}

// run the model process
// we pass the instance context in so we can cancel it using our stopProcess function
func (instance *ModelInstance) startProcess(session *types.Session) error {
	// download lora file

	cmd, err := instance.model.GetCommand(instance.ctx, instance.filter, types.RunnerProcessConfig{
		InstanceID:  instance.id,
		TaskURL:     instance.taskURL,
		ResponseURL: instance.responseURL,
	})
	if err != nil {
		return err
	}
	if cmd == nil {
		return fmt.Errorf("no command to run")
	}

	log.Info().
		Msgf("🟢 run model instance")
	spew.Dump(cmd.Dir)
	spew.Dump(cmd.Args)

	instance.currentCommand = cmd

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	// Create buffers to store stdout and stderr
	var stdoutBuf, stderrBuf bytes.Buffer

	// Start a go routine to copy the contents of the stdout pipe to the buffer
	go func() {
		_, err := io.Copy(io.MultiWriter(os.Stdout, &stdoutBuf), stdoutPipe)
		if err != nil {
			log.Error().Msgf("Error copying stdout: %v", err)
		}
	}()

	// Start a go routine to copy the contents of the stderr pipe to the buffer
	go func() {
		_, err := io.Copy(io.MultiWriter(os.Stderr, &stderrBuf), stderrPipe)
		if err != nil {
			log.Error().Msgf("Error copying stderr: %v", err)
		}
	}()

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		log.Error().Msgf("Failed to start command: %v\n", err.Error())
		return err
	}

	go func(cmd *exec.Cmd) {
		if err = cmd.Wait(); err != nil {
			log.Error().Msgf("Command ended with an error: %v\n", err.Error())

			// we are currently running a session and we got an error from the Python process
			// this normally means that a job caused an error so let's tell the api
			// that this interaction has it's Error field set
			if instance.currentSession != nil {
				interactionID, getInteractionErr := getLastInteractionID(instance.currentSession)
				if getInteractionErr != nil {
					log.Error().Msgf("Error reporting error to api: %v\n", getInteractionErr.Error())
					return
				}
				apiUpdateErr := instance.responseHandler(&types.WorkerTaskResponse{
					Type:          types.WorkerTaskResponseTypeResult,
					SessionID:     instance.currentSession.ID,
					InteractionID: interactionID,
					Error:         stderrBuf.String(),
				})
				if apiUpdateErr != nil {
					log.Error().Msgf("Error reporting error to api: %v\n", apiUpdateErr.Error())
				}
			}
		}

		log.Info().
			Msgf("🟢 stop model instance, exit code=%d", cmd.ProcessState.ExitCode())

		instance.finishChan <- true
	}(cmd)
	return nil
}

func (instance *ModelInstance) stopProcess() error {
	if instance.currentCommand == nil {
		return fmt.Errorf("no process to stop")
	}
	log.Info().Msgf("🟢 stop model process")
	if err := syscall.Kill(-instance.currentCommand.Process.Pid, syscall.SIGKILL); err != nil {
		log.Error().Msgf("error stopping model process: %s", err.Error())
		return err
	}
	log.Info().Msgf("🟢 stopped model process")
	return nil
}
