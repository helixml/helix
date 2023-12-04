// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/lukemarsden/helix/api/pkg/dataprep/text"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// set to false in production (will log messages to web UI)
const DEBUG = true

func (c *Controller) CreateSession(ctx context.Context, req types.CreateSessionRequest) (*types.Session, error) {
	// the system interaction is the task we will run on a GPU and update in place
	systemInteraction := types.Interaction{
		ID:             system.GenerateUUID(),
		Created:        time.Now(),
		Updated:        time.Now(),
		Creator:        types.CreatorTypeSystem,
		Message:        "",
		Files:          []string{},
		State:          types.InteractionStateWaiting,
		Finished:       false,
		Metadata:       map[string]string{},
		DataPrepChunks: map[string][]types.DataPrepChunk{},
	}

	session := types.Session{
		ID:            req.SessionID,
		Name:          system.GenerateAmusingName(),
		ModelName:     req.ModelName,
		Type:          req.SessionType,
		Mode:          req.SessionMode,
		ParentSession: req.ParentSession,
		Owner:         req.Owner,
		OwnerType:     req.OwnerType,
		Created:       time.Now(),
		Updated:       time.Now(),
		Interactions: []types.Interaction{
			req.UserInteraction,
			systemInteraction,
		},
	}

	// create session in database
	sessionData, err := c.Options.Store.CreateSession(ctx, session)
	if err != nil {
		return nil, err
	}

	go c.SessionRunner(sessionData)

	return sessionData, nil
}

func (c *Controller) UpdateSession(ctx context.Context, req types.UpdateSessionRequest) (*types.Session, error) {
	systemInteraction := types.Interaction{
		ID:       system.GenerateUUID(),
		Created:  time.Now(),
		Updated:  time.Now(),
		Creator:  types.CreatorTypeSystem,
		Message:  "",
		Files:    []string{},
		State:    types.InteractionStateWaiting,
		Finished: false,
		Metadata: map[string]string{},
	}
	session, err := c.Options.Store.GetSession(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	session.Updated = time.Now()
	session.Interactions = append(session.Interactions, req.UserInteraction, systemInteraction)

	log.Debug().
		Msgf("🟢 update session: %+v", session)

	sessionData, err := c.Options.Store.UpdateSession(ctx, *session)
	if err != nil {
		return nil, err
	}

	go c.SessionRunner(sessionData)

	return sessionData, nil
}

// called once we've done the pre-processing for both create and update calls to sessions
func (c *Controller) SessionRunner(sessionData *types.Session) {
	// first we prepare the seession - which could mean whatever the model implementation wants
	// so we have to wait for that to complete before adding to the queue
	// the model can be adding subsequent child sessions to the queue
	// e.g. in the case of text fine tuning data prep - we need an LLM to convert
	// text into q&a pairs and we want to use our own mistral inference
	preparedSession, err := c.PrepareSession(sessionData)
	if err != nil {
		log.Error().Msgf("error preparing session: %s", err.Error())
		c.ErrorSession(sessionData, err)
		return
	}
	// it's ok if we did not get a session back here
	// it means there will be a later action that will add the session to the queue
	// in the case the user needs to edit some data before it can be run for example
	if preparedSession != nil {
		c.AddSessionToQueue(preparedSession)
	}
}

// this is called in a go routine from the main api handler
// this is blocking the session being added to the queue
// so we get the chance to do some async pre-processing
// before the session joins the queue
// in some cases - we need the user to interact with our pre-processing
// in this case - let's return nil here and let the user interaction add the session to the queue
// once they have completed their editing
// e.g. for text fine-tuning we need to prepare the input files
//   - convert pdf, docx, etc to txt
//   - chunk the text based on buffer and overflow config
//   - feed each chunk into an LLM implementation to extract q&a pairs
//   - append the q&a pairs to a jsonl file
//
// so - that is all auto handled by the system
// the user then needs to view and edit the resuting JSONL file in the browser
// so now we are in a state where the session is still preparing but we are waiting
// for the user - so, we return nil here with no error which
// TODO: this should be adding jobs to a queue
func (c *Controller) PrepareSession(session *types.Session) (*types.Session, error) {
	var err error
	// load the model
	// call it's
	// here we need to turn all of the uploaded files into text files
	// so we ping our handy python server that will do that for us
	if session.Type == types.SessionTypeText && session.Mode == types.SessionModeFinetune {
		session, err = c.convertDocumentsToText(session)
		if err != nil {
			return nil, err
		}
		session, err = c.convertChunksToQuestions(session)
		if err != nil {
			return nil, err
		}

		// we DON'T want the session in the queue yet
		// the user has to confirm the questions are correct
		// or there might have been errors that we want to give the user
		// a chance to decide what to do
		return nil, nil
	}
	return session, nil
}

// generic "update this session handler"
// this will emit a UserWebsocketEvent with a type of
// WebsocketEventSessionUpdate
func (c *Controller) WriteSession(session *types.Session) {
	log.Trace().
		Msgf("🔵 update session: %s %+v", session.ID, session)

	_, err := c.Options.Store.UpdateSession(context.Background(), *session)
	if err != nil {
		log.Printf("Error adding message: %s", err)
	}

	event := &types.WebsocketEvent{
		Type:      types.WebsocketEventSessionUpdate,
		SessionID: session.ID,
		Owner:     session.Owner,
		Session:   session,
	}

	c.UserWebsocketEventChanWriter <- event
}

func (c *Controller) WriteInteraction(session *types.Session, newInteraction *types.Interaction) *types.Session {
	newInteractions := []types.Interaction{}
	for _, interaction := range session.Interactions {
		if interaction.ID == newInteraction.ID {
			newInteractions = append(newInteractions, *newInteraction)
		} else {
			newInteractions = append(newInteractions, interaction)
		}
	}
	session.Interactions = newInteractions
	c.WriteSession(session)
	return session
}

func (c *Controller) BroadcastWebsocketEvent(ctx context.Context, ev *types.WebsocketEvent) error {
	c.UserWebsocketEventChanWriter <- ev
	return nil
}

func (c *Controller) BroadcastProgress(
	session *types.Session,
	progress int,
	status string,
) {
	ev := &types.WebsocketEvent{
		Type:      types.WebsocketEventWorkerTaskResponse,
		SessionID: session.ID,
		Owner:     session.Owner,
		WorkerTaskResponse: &types.RunnerTaskResponse{
			Type:      types.WorkerTaskResponseTypeProgress,
			SessionID: session.ID,
			Owner:     session.Owner,
			Progress:  progress,
			Status:    status,
		},
	}
	c.UserWebsocketEventChanWriter <- ev
}

func (c *Controller) ErrorSession(session *types.Session, sessionErr error) {
	userInteraction, err := model.GetUserInteraction(session)
	if err != nil {
		return
	}

	userInteraction.Finished = true
	userInteraction.State = types.InteractionStateComplete

	errorInteraction, err := model.GetSystemInteraction(session)
	if err != nil {
		return
	}
	errorInteraction.State = types.InteractionStateError
	errorInteraction.Completed = time.Now()
	errorInteraction.Error = sessionErr.Error()
	errorInteraction.Finished = true

	session = updateSessionInteractions(session, []types.Interaction{
		*userInteraction,
		*errorInteraction,
	})

	c.WriteSession(session)
}

// once we've edited the JSONL file - we trigger the fine tuning by adding more interactions
func (c *Controller) BeginTextFineTune(session *types.Session) error {
	systemInteraction, err := model.GetSystemInteraction(session)
	if err != nil {
		return err
	}

	systemInteraction.Message = "completed document conversion"
	systemInteraction.Status = "all files converted to txt"
	systemInteraction.State = types.InteractionStateWaiting
	systemInteraction.DataPrepStage = types.TextDataPrepStageComplete
	systemInteraction.Files = []string{}

	session = updateSessionInteractions(session, []types.Interaction{
		*systemInteraction,
	})

	c.WriteSession(session)
	c.AddSessionToQueue(session)

	return nil
}

// add the given session onto the end of the queue
// unless it's already waiting and present in the queue
// in which case let's replace it at it's current position
// we mark the session as "preparing" here to give text fine tuning
// a chance to sort itself out in the background
func (c *Controller) AddSessionToQueue(session *types.Session) {
	sessionSummary, err := model.GetSessionSummary(session)
	if err != nil {
		log.Error().Msgf("error getting session summary: %s", err.Error())
		return
	}

	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	existing := false
	newQueue := []*types.Session{}
	newSummaryQueue := []*types.SessionSummary{}
	for i, existingSession := range c.sessionQueue {
		if existingSession.ID == session.ID {
			// the session we are updating is already in the queue!
			newQueue = append(newQueue, session)
			newSummaryQueue = append(newSummaryQueue, sessionSummary)
			existing = true
		} else {
			// this is another session we just want to copy it over
			// we use the index to copy so it's the same for the summary and the actual session
			newQueue = append(newQueue, c.sessionQueue[i])
			newSummaryQueue = append(newSummaryQueue, c.sessionSummaryQueue[i])
		}
	}
	if !existing {
		// we did not find the session already in the queue
		newQueue = append(newQueue, session)
		newSummaryQueue = append(newSummaryQueue, sessionSummary)
	}

	c.sessionQueue = newQueue
	c.sessionSummaryQueue = newSummaryQueue
}

func (c *Controller) HandleRunnerResponse(ctx context.Context, taskResponse *types.RunnerTaskResponse) (*types.RunnerTaskResponse, error) {
	session, err := c.Options.Store.GetSession(ctx, taskResponse.SessionID)
	if err != nil {
		return nil, err
	}

	if session == nil {
		return nil, fmt.Errorf("session not found: %s", taskResponse.SessionID)
	}

	session, err = model.UpdateSystemInteraction(session, func(targetInteraction *types.Interaction) (*types.Interaction, error) {
		// mark the interaction as complete if we are a fully finished response
		if taskResponse.Type == types.WorkerTaskResponseTypeResult {
			targetInteraction.Finished = true
			targetInteraction.Completed = time.Now()
			targetInteraction.State = types.InteractionStateComplete
		}

		// update the message if we've been given one
		if taskResponse.Message != "" {
			if taskResponse.Type == types.WorkerTaskResponseTypeResult {
				targetInteraction.Message = taskResponse.Message
			} else if taskResponse.Type == types.WorkerTaskResponseTypeStream {
				targetInteraction.Message += taskResponse.Message
			}
		}

		if taskResponse.Progress != 0 {
			targetInteraction.Progress = taskResponse.Progress
		}

		if taskResponse.Status != "" {
			targetInteraction.Status = taskResponse.Status
		}

		// update the files if there are some
		if taskResponse.Files != nil {
			targetInteraction.Files = taskResponse.Files
		}

		if taskResponse.Error != "" {
			targetInteraction.Error = taskResponse.Error
		}

		if taskResponse.Type == types.WorkerTaskResponseTypeResult && session.Mode == types.SessionModeFinetune && taskResponse.LoraDir != "" {
			// we got some files back from a finetune
			// so let's hoist the session into inference mode but with the finetune file attached
			session.Mode = types.SessionModeInference
			session.LoraDir = taskResponse.LoraDir
			targetInteraction.LoraDir = taskResponse.LoraDir
		}

		return targetInteraction, nil
	})

	c.WriteSession(session)

	return taskResponse, nil
}

// return the JSON of some fine tune conversation data
func (c *Controller) ReadTextFineTuneQuestions(filepath string) ([]types.DataPrepTextQuestion, error) {
	reader, err := c.Options.Filestore.Download(c.Ctx, filepath)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var conversations []types.DataPrepTextQuestion
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		var conversation types.DataPrepTextQuestion
		err := json.Unmarshal([]byte(line), &conversation)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}

	return conversations, nil
}

func (c *Controller) WriteTextFineTuneQuestions(filepath string, data []types.DataPrepTextQuestion) error {
	jsonLines := []string{}

	for _, conversationEntry := range data {
		jsonLine, err := json.Marshal(conversationEntry)
		if err != nil {
			return err
		}
		jsonLines = append(jsonLines, string(jsonLine))
	}

	_, err := c.Options.Filestore.Upload(c.Ctx, filepath, strings.NewReader(strings.Join(jsonLines, "\n")))
	if err != nil {
		return err
	}

	return nil
}

type convertDocumentsToChunksRequest struct {
	URL string `json:"url"`
}

type convertDocumentsToChunksResponse struct {
	Text string `json:"text"`
}

type convertTextItem struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// in the case of a text fine tune - we need to convert all the documents first
// TODO: there is no rate limiting on this path
func (c *Controller) convertDocumentsToText(session *types.Session) (*types.Session, error) {
	userInteraction, err := model.GetUserInteraction(session)
	if err != nil {
		return nil, err
	}

	systemInteraction, err := model.GetSystemInteraction(session)
	if err != nil {
		return nil, err
	}

	filesToConvert := []string{}
	existingFileNames := map[string]bool{}

	for _, file := range userInteraction.Files {
		existingFileNames[file] = true
	}

	shouldConvertFile := func(filename string) bool {
		if strings.HasSuffix(filename, ".txt") {
			// it is already converted - nothing to do
			return false
		}

		filename = strings.TrimSuffix(filename, ".url")

		if strings.HasSuffix(filename, ".jsonl") {
			return false
		}

		// if strings.HasSuffix(filename, types.TEXT_DATA_PREP_QUESTIONS_FILE_SUFFIX) {
		// 	// we've already converted this file into q&a pairs
		// 	return false
		// }

		// check if we have already got the chunks for this file
		_, ok := existingFileNames[fmt.Sprintf("%s.txt", filename)]
		if ok {
			// we've already chunked this file into chunks
			return false
		}

		// check if we have already got the chunks for this file
		// _, ok = existingFileNames[fmt.Sprintf("%s%s", filename, types.TEXT_DATA_PREP_QUESTIONS_FILE_SUFFIX)]
		// if ok {
		// 	// we've already chunked this file into chunks
		// 	return false
		// }

		return true
	}

	for _, file := range userInteraction.Files {
		if shouldConvertFile(file) {
			filesToConvert = append(filesToConvert, file)
		}
	}

	runningFileList := copyFileList(userInteraction.Files)

	// get the progress bar to display
	initialMessage := fmt.Sprintf("downloading and extracting text from %d files", len(filesToConvert))
	systemInteraction.Status = initialMessage
	systemInteraction.Progress = 1
	systemInteraction.DataPrepStage = types.TextDataPrepStageExtractText
	systemInteraction.State = types.InteractionStateWaiting
	session = c.WriteInteraction(session, systemInteraction)

	c.BroadcastProgress(session, 1, initialMessage)

	var completedCounter int64

	err = system.ForEachConcurrently[string](
		filesToConvert,
		c.Options.DataPrepConcurrency,
		func(file string, i int) error {
			fileURL := ""
			filenameParts := strings.Split(file, ".")
			originalFile := file

			if filenameParts[len(filenameParts)-1] == "url" {
				// if the file itself ends with .url then it's a textfile
				// that has a URL we should download as the actual file
				fileURL, err = getFileContent(c.Ctx, c.Options.Filestore, file)
				if err != nil {
					return err
				}
				file = strings.TrimSuffix(file, ".url")
			} else {
				// otherwise it's a file already living in the file store
				// so
				fileObject, err := c.Options.Filestore.Get(c.Ctx, file)
				if err != nil {
					return err
				}
				fileURL = fileObject.URL
			}

			// for local development - the file server hostname will not resolve
			// from inside the unstructured container
			fileURL = strings.Replace(fileURL, "http://localhost", "http://api", 1)

			res, err := system.PostRequest[convertDocumentsToChunksRequest, convertDocumentsToChunksResponse](
				system.ClientOptions{},
				c.Options.TextExtractionURL,
				convertDocumentsToChunksRequest{
					URL: fileURL,
				},
			)
			if err != nil {
				return err
			}

			atomic.AddInt64(&completedCounter, 1)
			newFilepath := strings.TrimSuffix(file, path.Ext(file)) + ".txt"

			_, err = c.Options.Filestore.Upload(c.Ctx, newFilepath, strings.NewReader(res.Text))
			if err != nil {
				return err
			}

			percentConverted := int(float64(completedCounter) / float64(len(filesToConvert)) * 100)
			message := fmt.Sprintf("extracted text from %s - %d of %d files extracted", path.Base(file), completedCounter, len(filesToConvert))
			c.BroadcastProgress(session, percentConverted, message)
			systemInteraction.Status = message
			systemInteraction.Progress = percentConverted
			session = c.WriteInteraction(session, systemInteraction)

			runningFileList = injectFileToList(runningFileList, originalFile, newFilepath)
			userInteraction.Files = runningFileList
			session = c.WriteInteraction(session, userInteraction)

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	finishedMessage := fmt.Sprintf("extracted %d files", len(filesToConvert))

	c.BroadcastProgress(session, 100, finishedMessage)

	// now we have added some text files let's update the user interaction
	userInteraction.Files = runningFileList
	// userInteraction.State = types.InteractionStateComplete
	session = c.WriteInteraction(session, userInteraction)

	systemInteraction.Status = finishedMessage
	session = c.WriteInteraction(session, systemInteraction)

	// for cases where the text conversion is very fast, give the UI a chance to display the text stage
	time.Sleep(1 * time.Second)

	return session, nil
}

func (c *Controller) convertChunksToQuestions(session *types.Session) (*types.Session, error) {
	userInteraction, err := model.GetUserInteraction(session)
	if err != nil {
		return nil, err
	}

	systemInteraction, err := model.GetSystemInteraction(session)
	if err != nil {
		return nil, err
	}

	filesToConvert := []string{}

	shouldConvertFile := func(filename string) bool {
		return strings.HasSuffix(filename, ".txt")
	}

	for _, file := range userInteraction.Files {
		if shouldConvertFile(file) {
			filesToConvert = append(filesToConvert, file)
		}
	}

	dataprep, splitter, err := c.Options.DataPrepTextFactory(session)
	if err != nil {
		return nil, err
	}

	// add all the files to the splitter so we know what chunks we have
	for _, file := range filesToConvert {
		fileContent, err := getFileContent(c.Ctx, c.Options.Filestore, file)
		if err != nil {
			return nil, err
		}
		err = splitter.AddDocument(file, fileContent)
		if err != nil {
			return nil, err
		}
	}

	fmt.Printf("splitter.Chunks --------------------------------------\n")
	fmt.Printf("splitter.Chunks --------------------------------------\n")
	fmt.Printf("splitter.Chunks --------------------------------------\n")
	spew.Dump(splitter.Chunks)

	chunksToProcess := []*text.DataPrepTextSplitterChunk{}
	for _, chunk := range splitter.Chunks {
		if !hasProcessedQAChunk(systemInteraction, chunk.Filename, chunk.Index) {
			chunksToProcess = append(chunksToProcess, chunk)
		}
	}

	// get the progress bar to display
	initialMessage := fmt.Sprintf("converting %d text chunks to question answer pairs", len(chunksToProcess))
	systemInteraction.Status = initialMessage
	systemInteraction.Progress = 1
	systemInteraction.DataPrepStage = types.TextDataPrepStageConvertQuestions
	session = c.WriteInteraction(session, systemInteraction)
	c.BroadcastProgress(session, 1, initialMessage)

	var completedCounter int64
	var errorCounter int64
	var outerError error

	// we use this to only append questions to one file at a time
	var writeUpdatesMutex sync.Mutex

	runningFileList := copyFileList(userInteraction.Files)

	outerError = system.ForEachConcurrently[*text.DataPrepTextSplitterChunk](
		chunksToProcess,
		dataprep.GetConcurrency(),
		func(chunk *text.DataPrepTextSplitterChunk, i int) error {
			log.Info().Msgf("🔵 question conversion start %d of %d", i, len(chunksToProcess))
			questions, convertError := dataprep.ConvertChunk(chunk.Text, chunk.Index)

			// if this is set then we have a non GPT error and should just stop what we are doing
			if outerError != nil {
				return nil
			}

			// write the updates inside a mutex so we don't get a race
			err = func() error {
				writeUpdatesMutex.Lock()
				defer writeUpdatesMutex.Unlock()

				if convertError == nil {
					// if there is no JSONL file - make it appear
					if !hasQuestionsFile(userInteraction, chunk.Filename) {
						runningFileList = injectFileToList(runningFileList, chunk.Filename, getQuestionsFilename(chunk.Filename))
						userInteraction.Files = runningFileList

						// we want to write an empty file to the filestore here
						// because then appendQuestionsToFile doesn't need to deal with making it
						_, err = c.Options.Filestore.Upload(c.Ctx, getQuestionsFilename(chunk.Filename), strings.NewReader(""))
						if err != nil {
							log.Error().Msgf("error uploading file: %s", err.Error())
							return err
						}
					}
					innerErr := appendQuestionsToFile(c.Ctx, c.Options.Filestore, getQuestionsFilename(chunk.Filename), questions)
					if innerErr != nil {
						log.Error().Msgf("error adding questions to file: %s", innerErr.Error())
						return innerErr
					}
					atomic.AddInt64(&completedCounter, 1)
				} else {
					atomic.AddInt64(&errorCounter, 1)
				}

				// this marks the QA chunk as "done" - even with an error
				// we then give the user the choice to try again, abort or ignore the errors
				systemInteraction = updateProcessedQAChunk(systemInteraction, chunk.Filename, chunk.Index, len(questions), convertError)

				return nil
			}()

			if err != nil {
				return err
			}

			session = c.WriteInteraction(session, userInteraction)

			percentConverted := int((float64(completedCounter) + float64(errorCounter)) / float64(len(chunksToProcess)) * 100)
			message := fmt.Sprintf("%d total, %d converted and %d errors", len(chunksToProcess), completedCounter, errorCounter)
			c.BroadcastProgress(session, percentConverted, message)
			systemInteraction.Status = message
			systemInteraction.Progress = percentConverted
			session = c.WriteInteraction(session, systemInteraction)

			if convertError != nil {
				log.Error().Msgf("🔴 question conversion error %s", convertError.Error())
			} else {
				log.Info().Msgf("🟢 question conversion complete %d of %d", i, len(chunksToProcess))
			}

			return nil
		},
	)

	// if this error is hit - it means something has actually gone wrong rather than a data prep error
	// we catch the data prep errors and present them to the user once all processing is done
	if outerError != nil {
		return nil, outerError
	}

	finishedMessage := fmt.Sprintf("converted %d text chunks", len(chunksToProcess))

	c.BroadcastProgress(session, 100, finishedMessage)

	systemInteraction.Status = finishedMessage
	systemInteraction.DataPrepStage = types.TextDataPrepStageEditQuestions
	systemInteraction.Progress = 0
	systemInteraction.State = types.InteractionStateEditing
	session = c.WriteInteraction(session, systemInteraction)

	return session, nil
}
