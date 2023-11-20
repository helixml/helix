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
	"time"

	"github.com/lukemarsden/helix/api/pkg/dataprep/text"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// set to false in production (will log messages to web UI)
const DEBUG = true

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

func (c *Controller) ReadRunnerWebsocketEvent(ctx context.Context, ev *types.WebsocketEvent) (*types.RunnerTaskResponse, error) {
	c.UserWebsocketEventChanWriter <- ev
	return nil, nil
}

func (c *Controller) HandleRunnerResponse(ctx context.Context, taskResponse *types.RunnerTaskResponse) (*types.RunnerTaskResponse, error) {
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

	if taskResponse.LoraDir != "" {
		targetInteraction.LoraDir = taskResponse.LoraDir
	}

	if taskResponse.Error != "" {
		targetInteraction.Error = taskResponse.Error
	}

	if taskResponse.Type == types.WorkerTaskResponseTypeResult && session.Mode == types.SessionModeFinetune && len(taskResponse.Files) > 0 {
		// we got some files back from a finetune
		// so let's hoist the session into inference mode but with the finetune file attached
		session.Mode = types.SessionModeInference
		session.LoraDir = taskResponse.Files[0]
		targetInteraction.LoraDir = taskResponse.Files[0]
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

func (c *Controller) CreateSession(ctx context.Context, req types.CreateSessionRequest) (*types.Session, error) {
	// the system interaction is the task we will run on a GPU and update in place
	systemInteraction := types.Interaction{
		ID:       system.GenerateUUID(),
		Created:  time.Now(),
		Creator:  types.CreatorTypeSystem,
		Message:  "",
		Files:    []string{},
		State:    types.InteractionStateWaiting,
		Finished: false,
		Metadata: map[string]string{},
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

	if req.ParentSession != "" {
		log.Debug().
			Msgf("🟢 new session: %+v", session)
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
		Creator:  types.CreatorTypeSystem,
		Message:  "",
		Files:    []string{},
		State:    types.InteractionStateWaiting,
		Finished: false,
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
		session, err = c.convertDocumentsToQuestions(session)
		if err != nil {
			return nil, err
		}
	}
	return session, nil
}

// return the JSON of some fine tune conversation data
func (c *Controller) ReadTextFineTuneQuestions(filepath string) ([]text.ShareGPTConversations, error) {
	reader, err := c.Options.Filestore.Download(c.Ctx, filepath)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var conversations []text.ShareGPTConversations
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		var conversation text.ShareGPTConversations
		err := json.Unmarshal([]byte(line), &conversation)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}

	return conversations, nil
}

func (c *Controller) WriteTextFineTuneQuestions(filepath string, data []text.ShareGPTConversations) error {
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

// once we've edited the JSONL file - we trigger the fine tuning by adding more interactions
func (c *Controller) BeginTextFineTune(session *types.Session) error {
	systemInteraction, err := model.GetSystemInteraction(session)
	if err != nil {
		return err
	}
	if len(systemInteraction.Files) == 0 {
		return fmt.Errorf("no files found")
	}
	filepath := systemInteraction.Files[0]
	if !strings.HasSuffix(filepath, ".jsonl") {
		return fmt.Errorf("file is not a jsonl file")
	}

	systemInteraction.Message = "completed document conversion"
	systemInteraction.Status = "all files converted to txt"
	systemInteraction.State = types.InteractionStateComplete
	systemInteraction.Finished = true

	finetuneUserInteraction := types.Interaction{
		ID:       system.GenerateUUID(),
		Created:  time.Now(),
		Creator:  types.CreatorTypeUser,
		Message:  "completed question & answer editing",
		Status:   "all question & answer pairs edited",
		Files:    systemInteraction.Files,
		State:    types.InteractionStateComplete,
		Finished: true,
		Metadata: map[string]string{},
	}

	finetuneSystemInteraction := types.Interaction{
		ID:       system.GenerateUUID(),
		Created:  time.Now(),
		Creator:  types.CreatorTypeSystem,
		Message:  "",
		Files:    []string{},
		State:    types.InteractionStateWaiting,
		Finished: false,
		Metadata: map[string]string{},
	}

	systemInteraction.Files = []string{}

	newInteractions := []types.Interaction{}
	for _, interaction := range session.Interactions {
		if interaction.ID == systemInteraction.ID {
			newInteractions = append(newInteractions, *systemInteraction)
		} else {
			newInteractions = append(newInteractions, interaction)
		}
	}
	newInteractions = append(newInteractions, finetuneUserInteraction, finetuneSystemInteraction)
	session.Interactions = newInteractions

	c.WriteSession(session)
	c.AddSessionToQueue(session)

	return nil
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

		// now we have added some text files let's update the user interaction
		userInteraction.Files = newFiles
		userInteraction.State = types.InteractionStateComplete
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

	dataprep, err := c.Options.DataPrepTextFactory(session)
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

	for i, chunk := range chunks {
		conversations, err := dataprep.ConvertChunk(chunk)
		if err != nil {
			return nil, err
		}
		allConversations = append(allConversations, conversations...)
		systemInteraction.Progress = int(float64(i+1) / float64(len(chunks)) * 100)
		systemInteraction.Status = fmt.Sprintf("converted %d of %d chunks into training data", i+1, len(chunks))
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
