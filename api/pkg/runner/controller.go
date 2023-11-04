package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/inhies/go-bytesize"
	"github.com/lukemarsden/helix/api/pkg/filestore"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/server"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type RunnerOptions struct {
	ID       string
	ApiHost  string
	ApiToken string

	// these URLs will have the instance ID appended by the model instance
	// e.g. http://localhost:8080/api/v1/worker/task/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/task
	TaskURL string
	// e.g. http://localhost:8080/api/v1/worker/response/:instanceid
	// we just pass http://localhost:8080/api/v1/worker/response
	ResponseURL string

	// how long without running a job before we close a model instance
	ModelInstanceTimeoutSeconds int
	// how many bytes of memory does our GPU have?
	// we report this back to the api when we ask
	// for the global next task (well, this minus the
	// currently running models)
	MemoryBytes uint64
	// if this is defined then we convert it usng
	// github.com/inhies/go-bytesize
	MemoryString string
}

type Runner struct {
	Ctx     context.Context
	Options RunnerOptions

	httpClientOptions server.ClientOptions

	modelMutex sync.RWMutex
	// the map of model instances that we have loaded
	// and are currently running
	activeModelInstances map[string]*ModelInstance

	// the lowest amount of memory that something can run with
	// if we have less than this amount of memory then there is
	// no point asking for more top level tasks
	// we get this on boot by asking the model package
	lowestMemoryRequirement uint64

	localQueue []*types.Session
}

func NewRunner(
	ctx context.Context,
	options RunnerOptions,
) (*Runner, error) {
	if options.ApiHost != "" {
		// these are only required if api-host is specified, we can also run in
		// a purely local mode
		if options.ID == "" {
			return nil, fmt.Errorf("id is required")
		}
		if options.ApiToken == "" {
			return nil, fmt.Errorf("api token is required")
		}
	}
	if options.MemoryString != "" {
		bytes, err := bytesize.Parse(options.MemoryString)
		if err != nil {
			return nil, err
		}
		options.MemoryBytes = uint64(bytes)
	}
	if options.MemoryBytes == 0 {
		return nil, fmt.Errorf("memory is required")
	}
	lowestMemoryRequirement, err := model.GetLowestMemoryRequirement()
	if err != nil {
		return nil, err
	}
	runner := &Runner{
		Ctx:                     ctx,
		Options:                 options,
		lowestMemoryRequirement: lowestMemoryRequirement,
		httpClientOptions: server.ClientOptions{
			Host:  options.ApiHost,
			Token: options.ApiToken,
		},
		activeModelInstances: map[string]*ModelInstance{},
	}
	return runner, nil
}

func modelInstanceMatchesSession(modelInstance *ModelInstance, session *types.Session) bool {
	return modelInstance.filter.Mode == session.Mode &&
		modelInstance.filter.Type == session.Type &&
		(modelInstance.filter.FinetuneFile == session.FinetuneFile ||
			(modelInstance.filter.FinetuneFile == "none" && session.FinetuneFile == ""))
}

func (r *Runner) AddToLocalQueue(ctx context.Context, session *types.Session) error {
	// iterate over model instances to see if one exists and if it doesn't, create it.
	// then add session to localQueue

	// Check if a model instance exists for the session's model ID
	found := false

	// loop over r.activeModelInstances, checking whether the filters on the
	// model instance match the session mode, type and finetune
	for _, modelInstance := range r.activeModelInstances {
		if modelInstanceMatchesSession(modelInstance, session) {
			// no need to create another one, because there's already one which will match the session
			log.Printf("🟠 Found modelInstance %+v which matches session %+v", modelInstance, session)
			found = true
			break
		}
	}
	if !found {
		// Create a new model instance because it doesn't exist
		log.Printf("🟠 No currently running modelInstance for session %+v, starting a new one", session)
		err := r.createModelInstance(ctx, session)
		if err != nil {
			return err
		}
	}

	// Add the session to the local queue
	r.localQueue = append(r.localQueue, session)
	return nil
}

// this should be run in a go-routine
func (r *Runner) StartLooping() {
	for {
		select {
		case <-r.Ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
			err := r.loop(r.Ctx)
			if err != nil {
				log.Error().Msgf("error in runner loop: %s", err.Error())
				debug.PrintStack()
			}
		}
	}
}

func (r *Runner) loop(ctx context.Context) error {
	// check for running model instances that have not seen a job in a while
	// and kill them if they are over the timeout
	// TODO: get the timeout to be configurable from the api and so dynamic
	// based on load
	err := r.checkForStaleModelInstances(ctx, time.Second*time.Duration(r.Options.ModelInstanceTimeoutSeconds))
	if err != nil {
		return err
	}

	// ask the api server if it currently has any work based on our free memory
	session, err := r.getNextGlobalSession(ctx)
	if err != nil {
		return err
	}

	if session != nil {
		log.Debug().
			Msgf("🔵 runner start model instance")
		err = r.createModelInstance(ctx, session)
		if err != nil {
			return err
		}
	}

	return nil
}

// loop over the active model instances and stop any that have not processed a job
// in the last timeout seconds
func (r *Runner) checkForStaleModelInstances(ctx context.Context, timeout time.Duration) error {
	r.modelMutex.Lock()
	defer r.modelMutex.Unlock()
	for _, activeModelInstance := range r.activeModelInstances {
		// this means we are booting so let's leave it alone to boot
		if activeModelInstance.lastActivityTimestamp == 0 {
			continue
		}
		if activeModelInstance.lastActivityTimestamp+int64(timeout.Seconds()) < time.Now().Unix() {
			log.Info().Msgf("Killing stale model instance %s", activeModelInstance.id)
			err := activeModelInstance.stopProcess()
			if err != nil {
				log.Error().Msgf("error stopping model instance %s: %s", activeModelInstance.id, err.Error())
				continue
			}
		}
	}
	return nil
}

// ask the master API if they have the next session for us
// we check with the various models and filter based on the currently free memory
// we pass that free memory back to the master API - it will filter out any tasks
// for models that would require more memory than we have available
func (r *Runner) getNextGlobalSession(ctx context.Context) (*types.Session, error) {
	if r.httpClientOptions.Host == "" {
		// we are in local only mode... the next session will be injected into
		// us rather than queried from the control server
		// TODO: it would be nice to still support memory limits and other
		// smarts for local tasks
		return nil, nil
	}
	freeMemory := r.getFreeMemory()

	if freeMemory < r.lowestMemoryRequirement {
		// we don't have enough memory to run anything
		// so we just wait for more memory to become available
		return nil, nil
	}

	queryParams := url.Values{}

	// this means "only give me sessions that will fit in this much RAM"
	queryParams.Add("memory", fmt.Sprintf("%d", freeMemory))

	// now let's loop over our running model instances and de-prioritise them
	// we might still get sessions of this type but only if there isn't another
	// type in the queue - this is to avoid running only one type of model
	// because of the random order the requests arrived in
	// (i.e. if we get 100 text inferences then the chance is we boot 100 model instances)
	// before trying to run another type of model

	for _, modelInstance := range r.activeModelInstances {
		queryParams.Add("reject", fmt.Sprintf("%s:%s", modelInstance.filter.ModelName, modelInstance.filter.Mode))
	}

	return r.getNextSession(ctx, queryParams)
}

// we have popped the next session from the master API
// let's create a model for it
// this means instantiating the model instance and then starting it
// because this model consumes memory it means the global next job filter
// will take into account the fact this model is running
// and will add the de-prioritise filter to the next request
// so that we get a different job type
func (r *Runner) createModelInstance(ctx context.Context, session *types.Session) error {
	r.modelMutex.Lock()
	defer r.modelMutex.Unlock()

	modelInstance, err := NewModelInstance(
		r.Ctx,
		session,
		r.Options.TaskURL,
		r.Options.ResponseURL,

		// this function will convert any files it sees locally into an upload
		// to the api server filestore - all files will be written to the filestore
		// under a session sub path - you can include tar files and they will untarred at the other end
		// into the filestore
		// TODO: support the tar feature above
		func(res *types.WorkerTaskResponse) error {
			return r.uploadWorkerResponse(res, session)
		},
		r.Options,
	)
	if err != nil {
		return err
	}

	// belt and braces in remote case and reject jobs that won't fit in local case
	modelMem := modelInstance.model.GetMemoryRequirements(session.Mode)
	freeMem := r.getFreeMemory()
	if modelMem > freeMem {
		// refuse to start or record the model instance, it will just get GC'd at this point
		return fmt.Errorf("cannot fit model requiring gpu memory %d into available gpu memory %d", modelMem, freeMem)
	}

	log.Debug().
		Msgf("🔵 runner started model instance: %s", modelInstance.id)

	// every session is queued - it gives the session a chance to download files
	// and do any other prep it needs to do before being passed into the Python process
	// over http - this is run for EVERY session not just the first one (look in getNextTask)
	// this will run in a go-routine internally
	go modelInstance.queueSession(session)

	// now we block on starting the process
	// for inference on a lora type file, this will need to download the lora file
	// BEFORE we start the process -these files are part of the weights loaded into memory
	err = modelInstance.startProcess(session)
	if err != nil {
		return err
	}
	r.activeModelInstances[modelInstance.id] = modelInstance
	go func() {
		<-modelInstance.finishChan
		r.modelMutex.Lock()
		defer r.modelMutex.Unlock()
		log.Debug().
			Msgf("🔵 runner stop model instance: %s", modelInstance.id)
		delete(r.activeModelInstances, modelInstance.id)
	}()
	return nil
}

// given a running model instance id
// get the next session that it should run
// this is either the nextSession property on the instance
// or it's what is returned by the master API server (if anything)
// this function being called means "I am ready for more work"
// because the child processes are blocking - the child will not be
// asking for more work until it's ready to accept and run it
func (r *Runner) getNextTask(ctx context.Context, instanceID string) (*types.WorkerTask, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instanceid is required")
	}
	modelInstance, ok := r.activeModelInstances[instanceID]
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	var session *types.Session

	foundLocalQueuedSession := false
	for i, sess := range r.localQueue {
		if modelInstanceMatchesSession(modelInstance, sess) {
			foundLocalQueuedSession = true
			// remove it from the local queue
			r.localQueue = append(r.localQueue[:i], r.localQueue[i+1:]...)
			session = sess
			break
		}

	}
	// as the first check, we need to ask if there's a session in localQueue
	// that matches this model instance. if there is, we've got a local
	// session and it takes precedence over remote work

	// if there is, call modelInstance.queueSession on it

	if foundLocalQueuedSession {
		// queue it, and fall thru below to assign
		go modelInstance.queueSession(session)
	} else if modelInstance.nextSession != nil {
		// if there is a session in the nextSession cache then we return it immediately
		session = modelInstance.nextSession
		modelInstance.nextSession = nil
	} else if modelInstance.queuedSession != nil {
		// if there is a session in the queuedSession cache then we are waiting for
		// a task to complete before we want to actually run the session
	} else {
		// ask the upstream api server if there is another task
		// if there is - then assign it to the queuedSession
		// and call "pre"
		if r.httpClientOptions.Host != "" {
			queryParams := url.Values{}

			queryParams.Add("model_name", string(modelInstance.filter.ModelName))
			queryParams.Add("mode", string(modelInstance.filter.Mode))
			queryParams.Add("finetune_file", string(modelInstance.filter.FinetuneFile))

			apiSession, err := r.getNextSession(ctx, queryParams)
			if err != nil {
				return nil, err
			}

			if apiSession != nil {
				go modelInstance.queueSession(apiSession)
			}
		}
	}

	// we don't have any work for this model instance
	if session == nil {
		return nil, fmt.Errorf("no session found")
	}

	task, err := modelInstance.assignSessionTask(ctx, session)
	if err != nil {
		return nil, err
	}

	return task, nil
}

func (r *Runner) handleTaskResponse(ctx context.Context, instanceID string, taskResponse *types.WorkerTaskResponse) (*types.WorkerTaskResponse, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instanceid is required")
	}
	if taskResponse == nil {
		return nil, fmt.Errorf("task response is required")
	}
	modelInstance, ok := r.activeModelInstances[instanceID]
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}
	switch {
	case taskResponse.Type == types.WorkerTaskResponseTypeStream:
		err := modelInstance.handleStream(ctx, taskResponse)
		if err != nil {
			log.Error().Msgf("error handling stream: %s", err.Error())
			return nil, err
		}
	case taskResponse.Type == types.WorkerTaskResponseTypeProgress:
		err := modelInstance.handleProgress(ctx, taskResponse)
		if err != nil {
			log.Error().Msgf("error handling progress: %s", err.Error())
			return nil, err
		}
	case taskResponse.Type == types.WorkerTaskResponseTypeResult:
		err := modelInstance.handleResult(ctx, taskResponse)
		if err != nil {
			log.Error().Msgf("error handling job result: %s", err.Error())
			return nil, err
		}
	}

	return taskResponse, nil
}

func (r *Runner) getNextSession(ctx context.Context, queryParams url.Values) (*types.Session, error) {
	parsedURL, err := url.Parse(server.URL(r.httpClientOptions, fmt.Sprintf("/runner/%s/nextsession", r.Options.ID)))
	if err != nil {
		return nil, err
	}
	parsedURL.RawQuery = queryParams.Encode()

	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", r.httpClientOptions.Token))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, nil
	}

	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, resp.Body)
	if err != nil {
		return nil, err
	}

	var session *types.Session
	err = json.Unmarshal(buffer.Bytes(), &session)
	if err != nil {
		return nil, err
	}

	return session, nil
}

func (r *Runner) getUsedMemory() uint64 {
	r.modelMutex.RLock()
	defer r.modelMutex.RUnlock()
	memoryUsed := uint64(0)
	for _, modelInstance := range r.activeModelInstances {
		memoryUsed += modelInstance.model.GetMemoryRequirements(modelInstance.filter.Mode)
	}
	return memoryUsed
}

func (r *Runner) getFreeMemory() uint64 {
	return r.Options.MemoryBytes - r.getUsedMemory()
}

func (r *Runner) uploadWorkerResponse(res *types.WorkerTaskResponse, session *types.Session) error {
	if r.httpClientOptions.Host == "" {
		// no upstream server configured, skip uploading
		return nil
	}
	if len(res.Files) > 0 {
		// create a new multipart form
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		// loop over each file and add it to the form
		for _, filepath := range res.Files {
			file, err := os.Open(filepath)
			if err != nil {
				return err
			}
			defer file.Close()

			// create a new form field for the file
			part, err := writer.CreateFormFile("files", filepath)
			if err != nil {
				return err
			}

			// copy the file contents into the form field
			_, err = io.Copy(part, file)
			if err != nil {
				return err
			}
		}

		// close the multipart form
		err := writer.Close()
		if err != nil {
			return err
		}

		url := server.URL(r.httpClientOptions, fmt.Sprintf("/runner/%s/session/%s/upload", r.Options.ID, session.ID))

		log.Debug().Msgf("🟠 upload files %s", url)

		// create a new POST request with the multipart form as the body
		req, err := http.NewRequest("POST", url, body)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		server.AddHeadersVanilla(req, r.httpClientOptions.Token)

		// send the request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// handle the response
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		var data []filestore.FileStoreItem
		resultBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		// parse body as json into result
		err = json.Unmarshal(resultBody, &data)
		if err != nil {
			return err
		}

		mappedFiles := []string{}

		for _, fileItem := range data {
			mappedFiles = append(mappedFiles, fileItem.Path)
		}

		res.Files = mappedFiles
	}

	log.Debug().Msgf("🟠 Sending task response %s", session.ID)
	spew.Dump(res)

	// this function will write any task responses back to the api server for it to process
	// we will only hear WorkerTaskResponseTypeStreamContinue and WorkerTaskResponseTypeResult
	// and both of these will have an interaction ID and the full, latest copy of the text
	// the job of the api server is to ensure the existence of the instance (create or update)
	// and replace it's message property - this is the text streaming case
	// if the model does not return a text stream - then all we will hear is a WorkerTaskResponseTypeResult
	// and the api server is just appending to the session
	_, err := server.PostRequest[*types.WorkerTaskResponse, *types.WorkerTaskResponse](
		r.httpClientOptions,
		fmt.Sprintf("/runner/%s/response", r.Options.ID),
		res,
	)
	if err != nil {
		return err
	}
	return nil
}
