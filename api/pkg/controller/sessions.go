// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/lukemarsden/helix/api/pkg/dataprep/text"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// set to false in production (will log messages to web UI)
const DEBUG = true

// this function expects the sessionQueueMtx to be locked when it is run
func (c *Controller) getMatchingSessionFilterIndex(ctx context.Context, filter types.SessionFilter) int {
	for i, session := range c.sessionQueue {
		if filter.Mode != "" && session.Mode != filter.Mode {
			continue
		}
		if filter.Type != "" && session.Type != filter.Type {
			continue
		}
		if filter.ModelName != "" && session.ModelName != filter.ModelName {
			continue
		}

		if filter.FinetuneFile == types.FINETUNE_FILE_NONE {
			// the filter is NONE - we cannot have a finetune file
			if session.FinetuneFile != "" {
				continue
			}
		} else if filter.FinetuneFile != "" {
			// the filter is a SPECIFIC file - we must have that file
			if session.FinetuneFile != filter.FinetuneFile {
				continue
			}
		} else if filter.FinetuneFile == "" {
			// the filter is ANY file - so anything goes
		}

		// we are asking for sessions that will fit in an amount of RAM
		// so we need to ask the associated model instance what the memory
		// requirements are for this session
		if filter.Memory > 0 {
			model, ok := c.models[session.ModelName]
			if !ok {
				continue
			}
			if model.GetMemoryRequirements(session.Mode) > filter.Memory {
				continue
			}
		}

		// look to see if we have any rejection matches that we should not include
		reject := false
		for _, rejectEntry := range filter.Reject {
			if rejectEntry.ModelName == session.ModelName && rejectEntry.Mode == session.Mode {
				reject = true
			}
		}
		if reject {
			continue
		}

		// if we've made it this far we've got a session!
		return i
	}

	return -1
}

// load the session queues from the database in case of restart
func (c *Controller) loadSessionQueues(ctx context.Context) error {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	sessionQueue := []*types.Session{}

	st := c.Options.Store

	// fetch all sessions - this is in DESC order so we need to reverse the array
	sessions, err := st.GetSessions(ctx, store.GetSessionsQuery{})
	if err != nil {
		return err
	}

	for i := len(sessions) - 1; i >= 0; i-- {
		session := sessions[i]

		interactions := session.Interactions
		if interactions == nil || len(interactions) == 0 {
			// should never happen, sessions are always initiated by the user
			// creating an initial message
			continue
		}

		latest := interactions[len(interactions)-1]
		if latest.Creator == types.CreatorTypeSystem {
			// we've already given a response, don't need to do anything
			continue
		}

		if latest.Runner != "" {
			// this session is already being worked on
			continue
		}

		sessionQueue = append(sessionQueue, session)
	}

	// now we have the queue in oldest first order
	c.sessionQueue = sessionQueue
	return nil
}

// the core function - decide which task to give to a worker
// TODO: keep track of the previous tasks run by this worker (and therefore we know which weights are loaded into RAM)
// try to send similar tasks to the same worker
func (c *Controller) ShiftSessionQueue(ctx context.Context, filter types.SessionFilter, runnerID string) (*types.Session, error) {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	sessionIndex := c.getMatchingSessionFilterIndex(ctx, filter)

	if sessionIndex >= 0 {
		session := c.sessionQueue[sessionIndex]

		log.Debug().
			Msgf("🔵 scheduler hit query: %+v", filter)
		log.Debug().
			Msgf("🔵 scheduler hit session: %+v", session)

		c.sessionQueue = append(c.sessionQueue[:sessionIndex], c.sessionQueue[sessionIndex+1:]...)

		if len(session.Interactions) == 0 {
			return nil, fmt.Errorf("no interactions found")
		}

		return session, nil
	}

	return nil, nil
}

func (c *Controller) RemoveSessionFromQueue(ctx context.Context, id string) error {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	sessionQueue := []*types.Session{}

	for _, session := range c.sessionQueue {
		if session.ID == id {
			continue
		}
		sessionQueue = append(sessionQueue, session)
	}

	c.sessionQueue = sessionQueue

	return nil
}

// generic "update this session handler"
// this will emit a UserWebsocketEvent with a type of
// WebsocketEventSessionUpdate
func (c *Controller) WriteSession(session *types.Session) {
	log.Debug().
		Msgf("🔵 update session: %s %+v", session.ID, session)

	_, err := c.Options.Store.UpdateSession(context.Background(), *session)
	if err != nil {
		log.Printf("Error adding message: %s", err)
	}

	event := &types.WebsocketEvent{
		Type:      types.WebsocketEventSessionUpdate,
		SessionID: session.ID,
		Session:   session,
	}

	c.UserWebsocketEventChan <- event
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

func (c *Controller) ErrorSession(session *types.Session, sessionErr error) {
	userInteraction, err := model.GetUserInteraction(session)
	if err != nil {
		return
	}

	userInteraction.Finished = true
	userInteraction.State = types.InteractionStateReady

	errorInteraction, err := model.GetSystemInteraction(session)
	if err != nil {
		return
	}
	errorInteraction.State = types.InteractionStateError
	errorInteraction.Error = sessionErr.Error()
	errorInteraction.Finished = true

	newInteractions := []types.Interaction{}
	for _, interaction := range session.Interactions {
		if interaction.ID == errorInteraction.ID {
			newInteractions = append(newInteractions, *errorInteraction)
		} else if interaction.ID == userInteraction.ID {
			newInteractions = append(newInteractions, *userInteraction)
		} else {
			newInteractions = append(newInteractions, interaction)
		}
	}

	session.Interactions = newInteractions

	c.WriteSession(session)
}

// add the given session onto the end of the queue
// unless it's already waiting and present in the queue
// in which case let's replace it at it's current position
// we mark the session as "preparing" here to give text fine tuning
// a chance to sort itself out in the background
func (c *Controller) AddSessionToQueue(session *types.Session) {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	existing := false
	newQueue := []*types.Session{}
	for _, existingSession := range c.sessionQueue {
		if existingSession.ID == session.ID {
			newQueue = append(newQueue, session)
			existing = true
		} else {
			newQueue = append(newQueue, existingSession)
		}
	}
	if !existing {
		newQueue = append(newQueue, session)
	}

	c.sessionQueue = newQueue
}

func (c *Controller) HandleRunnerResponse(ctx context.Context, taskResponse *types.WorkerTaskResponse) (*types.WorkerTaskResponse, error) {
	session, err := c.Options.Store.GetSession(ctx, taskResponse.SessionID)
	if err != nil {
		return nil, err
	}

	if session == nil {
		return nil, fmt.Errorf("session not found: %s", taskResponse.SessionID)
	}

	// let's see if we are updating an existing interaction
	// or appending a new one
	targetInteraction, err := model.GetSystemInteraction(session)
	if err != nil {
		return nil, err
	}

	if targetInteraction == nil {
		return nil, fmt.Errorf("interaction not found: %s", taskResponse.SessionID)
	}

	if targetInteraction.Creator == types.CreatorTypeUser {
		return nil, fmt.Errorf("interaction is not a system interaction cannot update: %s -> %s", taskResponse.SessionID, targetInteraction.ID)
	}

	// mark the interaction as complete if we are a fully finished response
	if taskResponse.Type == types.WorkerTaskResponseTypeResult {
		targetInteraction.Finished = true
		targetInteraction.State = types.InteractionStateReady
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

	if taskResponse.Type == types.WorkerTaskResponseTypeResult && session.Mode == types.SessionModeFinetune && len(taskResponse.Files) > 0 {
		// we got some files back from a finetune
		// so let's hoist the session into inference mode but with the finetune file attached
		session.Mode = types.SessionModeInference
		session.FinetuneFile = taskResponse.Files[0]
		targetInteraction.FinetuneFile = taskResponse.Files[0]
	}

	newInteractions := []types.Interaction{}
	for _, interaction := range session.Interactions {
		if interaction.ID == targetInteraction.ID {
			newInteractions = append(newInteractions, *targetInteraction)
		} else {
			newInteractions = append(newInteractions, interaction)
		}
	}

	session.Interactions = newInteractions

	c.WriteSession(session)

	return taskResponse, nil
}

// this is called in a go routine from the main api handler
// it needs to first prepare the session
// and then it can add it to the queue for backend processing
func (c *Controller) PrepareSession(session *types.Session) (*types.Session, error) {
	// here we need to turn all of the uploaded files into text files
	// so we ping our handy python server that will do that for us
	if session.Type == types.SessionTypeText && session.Mode == types.SessionModeFinetune {
		session, err := c.convertDocumentsToText(session)
		if err != nil {
			return nil, err
		}
		session, err = c.convertDocumentsToQuestions(session)
		if err != nil {
			return nil, err
		}
	}
	return session, nil
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

	if userInteraction.State != types.InteractionStateWaiting {
		return session, nil
	}

	newFiles := []string{}
	for _, file := range userInteraction.Files {

		newFiles = append(newFiles, file)

		// if file is not a text file
		// then we need to convert it
		if !strings.HasSuffix(file, ".txt") {
			systemInteraction.Status = fmt.Sprintf("converting file: %s", path.Base(file))
			c.WriteInteraction(session, systemInteraction)

			log.Debug().
				Msgf("🔵 converting file: %s", file)
			reader, err := c.Options.Filestore.Download(c.Ctx, file)
			if err != nil {
				return nil, err
			}

			client := newRetryClient()

			req, err := createMultipartRequest(c.Options.TextExtractionURL, "documents", path.Base(file), reader)
			if err != nil {
				return nil, fmt.Errorf("Error creating request: %v\n", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}

			var results []convertTextItem

			err = json.Unmarshal(body, &results)
			if err != nil {
				return nil, err
			}

			if len(results) == 0 {
				return nil, fmt.Errorf("no results found")
			}

			newFilepath := strings.TrimSuffix(file, path.Ext(file)) + ".txt"

			_, err = c.Options.Filestore.Upload(c.Ctx, newFilepath, strings.NewReader(results[0].Content))
			if err != nil {
				return nil, err
			}

			newFiles = append(newFiles, newFilepath)
		}

		userInteraction.Files = newFiles

		// now we have added some text files let's update the user interaction
		session = c.WriteInteraction(session, userInteraction)

		systemInteraction.Status = fmt.Sprintf("all files converted to txt")
		session = c.WriteInteraction(session, systemInteraction)
	}

	return session, nil
}

func (c *Controller) convertDocumentsToQuestions(session *types.Session) (*types.Session, error) {
	userInteraction, err := model.GetUserInteraction(session)
	if err != nil {
		return nil, err
	}

	systemInteraction, err := model.GetSystemInteraction(session)
	if err != nil {
		return nil, err
	}

	if userInteraction.State != types.InteractionStateWaiting {
		return session, nil
	}

	dataprep, err := c.Options.DataPrepTextFactory()
	if err != nil {
		return nil, err
	}

	for _, file := range userInteraction.Files {
		// if file is not a text file
		// then we need to convert it
		if !strings.HasSuffix(file, ".txt") {
			continue
		}

		systemInteraction.Status = fmt.Sprintf("adding document to training data: %s", path.Base(file))
		c.WriteInteraction(session, systemInteraction)

		reader, err := c.Options.Filestore.Download(c.Ctx, file)
		if err != nil {
			return nil, err
		}

		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, reader)
		if err != nil {
			return nil, err
		}

		err = dataprep.AddDocument(buf.String())
		if err != nil {
			return nil, err
		}
	}

	finetuneDataSet := path.Join(path.Dir(userInteraction.Files[0]), "finetune_dataset.jsonl")

	chunks, err := dataprep.GetChunks()
	if err != nil {
		return nil, err
	}

	allConversations := []text.DataPrepTextConversation{}

	systemInteraction.Progress = 1
	systemInteraction.Status = fmt.Sprintf("converted 0 of %d chunks into training data", len(chunks))
	c.WriteInteraction(session, systemInteraction)

	for index, chunk := range chunks {
		conversations, err := dataprep.ConvertChunk(chunk)
		if err != nil {
			return nil, err
		}
		allConversations = append(allConversations, conversations...)
		systemInteraction.Progress = int(float64(index+1) / float64(len(chunks)) * 100)
		systemInteraction.Status = fmt.Sprintf("converted %d of %d chunks into training data", index+1, len(chunks))
		c.WriteInteraction(session, systemInteraction)
	}

	// now we have allConversations we convert into jsonL data

	jsonLines := []string{}

	for _, conversationEntry := range allConversations {
		jsonLine, err := json.Marshal(text.ConvertConversation(conversationEntry))
		if err != nil {
			return nil, err
		}
		jsonLines = append(jsonLines, string(jsonLine))
	}

	_, err = c.Options.Filestore.Upload(c.Ctx, finetuneDataSet, strings.NewReader(strings.Join(jsonLines, "\n")))
	if err != nil {
		return nil, err
	}

	// by this stage we need to have generated a jsonl file of the text
	systemInteraction.Files = []string{finetuneDataSet}
	systemInteraction.Status = fmt.Sprintf("all files converted to txt - please edit and save the file to start training")
	systemInteraction.Progress = 0
	systemInteraction.State = types.InteractionStateEditing
	session = c.WriteInteraction(session, systemInteraction)

	// we return nil here because we want the user to edit the JSONL
	// file and we will handle adding the session to the queue ourselves
	return nil, nil
}

// return the JSON of some fine tune conversation data
func (c *Controller) ReadTextFineTuneQuestions(filepath string) ([]text.ShareGPTConversation, error) {
	reader, err := c.Options.Filestore.Download(c.Ctx, filepath)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var conversations []text.ShareGPTConversation
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		var conversation text.ShareGPTConversation
		err := json.Unmarshal([]byte(line), &conversation)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}

	return conversations, nil
}

func (c *Controller) SaveTextFineTuneQuestions(filepath string, data []text.ShareGPTConversation) error {
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
