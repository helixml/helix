// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"fmt"
	"log"

	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/types"
)

// set to false in production (will log messages to web UI)
const DEBUG = true

// we keep in memory queues of sessions that need to be processed
// agents will be reaching out over http to ask "got any jobs"
// and it would be nice to respond quickly - hence keeping this in memory
// but then you have restarts and shit to deal with, so every 10 seconds
// we clear the queues and reload them from the database
// the queues are pure backlog - nothing actually running lives in these queues
type sessionQueues struct {
	textInference  []*types.Session
	imageInference []*types.Session
	textFinetune   []*types.Session
	imageFinetune  []*types.Session
}

func newSessionQueues() *sessionQueues {
	return &sessionQueues{
		textInference:  []*types.Session{},
		imageInference: []*types.Session{},
		textFinetune:   []*types.Session{},
		imageFinetune:  []*types.Session{},
	}
}

// given a mode and type - pop the next session from the queue
func (c *Controller) PopSessionQueue(ctx context.Context, query types.SessionQuery) (*types.Session, error) {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	switch {
	case query.Mode == "Create" && query.Type == "Text":
		for i, session := range c.sessionQueues.textInference {
			if session.ModelName == query.ModelName {
				// remove the session from the queue
				c.sessionQueues.textInference = append(c.sessionQueues.textInference[:i], c.sessionQueues.textInference[i+1:]...)
				return session, nil
			}
		}
		return nil, nil
	case query.Mode == "Create" && query.Type == "Image":
		for i, session := range c.sessionQueues.imageInference {
			if session.ModelName == query.ModelName {
				// remove the session from the queue
				c.sessionQueues.imageInference = append(c.sessionQueues.imageInference[:i], c.sessionQueues.imageInference[i+1:]...)
				return session, nil
			}
		}
		return nil, nil
	case query.Mode == "Finetune" && query.Type == "Text":
		for i, session := range c.sessionQueues.textFinetune {
			if session.ModelName == query.ModelName {
				// remove the session from the queue
				c.sessionQueues.textFinetune = append(c.sessionQueues.textFinetune[:i], c.sessionQueues.textFinetune[i+1:]...)
				return session, nil
			}
		}
		return nil, nil
	case query.Mode == "Finetune" && query.Type == "Image":
		for i, session := range c.sessionQueues.imageFinetune {
			if session.ModelName == query.ModelName {
				// remove the session from the queue
				c.sessionQueues.imageFinetune = append(c.sessionQueues.imageFinetune[:i], c.sessionQueues.imageFinetune[i+1:]...)
				return session, nil
			}
		}
		return nil, nil
	}
	return nil, nil
}

func (c *Controller) PopSessionTask(ctx context.Context, query types.SessionQuery) (*types.WorkerTask, error) {
	session, err := c.PopSessionQueue(ctx, query)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}

	task := &types.WorkerTask{
		SessionID: session.ID,
		Mode:      session.Mode,
		Type:      session.Type,
		ModelName: session.ModelName,
	}

	switch {
	case query.Mode == "Create" && query.Type == "Text":
		var messages string
		for _, message := range session.Interactions.Messages {
			messages += message.Message + "\n"
		}
		task.Prompt = fmt.Sprintf("[INST]%s[/INST]", messages)
		return task, nil
	case query.Mode == "Create" && query.Type == "Image":
		return nil, nil
	case query.Mode == "Finetune" && query.Type == "Text":
		return nil, nil
	case query.Mode == "Finetune" && query.Type == "Image":
		return nil, nil
	}
	return nil, nil
}

func insertSessionIntoQueue(sessions []*types.Session, session *types.Session) []*types.Session {
	existing := false
	newQueue := []*types.Session{}
	for _, existingSession := range sessions {
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

	return newQueue
}

func (c *Controller) PushSessionQueue(ctx context.Context, session *types.Session) error {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	switch {
	case session.Mode == "Create" && session.Type == "Text":
		c.sessionQueues.textInference = insertSessionIntoQueue(c.sessionQueues.textInference, session)
	case session.Mode == "Create" && session.Type == "Image":
		c.sessionQueues.imageInference = insertSessionIntoQueue(c.sessionQueues.imageInference, session)
	case session.Mode == "Finetune" && session.Type == "Text":
		c.sessionQueues.textFinetune = insertSessionIntoQueue(c.sessionQueues.textFinetune, session)
	case session.Mode == "Finetune" && session.Type == "Image":
		c.sessionQueues.imageFinetune = insertSessionIntoQueue(c.sessionQueues.imageFinetune, session)
	}
	return nil
}

// reload the current session queues from the database
// this is called on startup and every 10 seconds
func (c *Controller) loadSessionQueues(ctx context.Context) error {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	queues := newSessionQueues()

	st := c.Options.Store

	// fetch all sessions
	// NOTE: this will fetch in DESC order so the latest is first
	// if we want to do a FIFO queue then we need to pick from the end
	sessions, err := st.GetSessions(ctx, store.GetSessionsQuery{})
	if err != nil {
		return err
	}

	for i := len(sessions) - 1; i >= 0; i-- {
		session := sessions[i]

		msgs := session.Interactions.Messages
		if len(msgs) == 0 {
			// should never happen, sessions are always initiated by the user
			// creating an initial message
			continue
		}

		latest := msgs[len(msgs)-1]
		if latest.User == "system" {
			// we've already given a response, don't need to do anything
			continue
		}

		switch {
		case session.Mode == "Create" && session.Type == "Text":
			queues.textInference = append(queues.textInference, session)
		case session.Mode == "Create" && session.Type == "Image":
			queues.imageInference = append(queues.imageInference, session)
		case session.Mode == "Finetune" && session.Type == "Text":
			queues.textFinetune = append(queues.textFinetune, session)
		case session.Mode == "Finetune" && session.Type == "Image":
			queues.imageFinetune = append(queues.imageFinetune, session)
		}
	}

	c.sessionQueues = queues
	return nil
}

// load all jobs that are currently running and check if they are still running
func (c *Controller) triggerSessionTasks(ctx context.Context) error {
	log.Println("Starting triggerSessionTasks")
	// NB: for the demo, being serialized here is good: it means we'll only
	// spawn one GPU task at a time, and run less risk of GPU OOM. Later, we'll
	// need to figure out how to scale/parallelize this, which is what the
	// Lilypad/Kubernetes schedulers are for

	st := c.Options.Store
	// fetch all sessions
	sessions, err := st.GetSessions(ctx, store.GetSessionsQuery{})
	if err != nil {
		return err
	}

	for _, session := range sessions {

		st := c.Options.Store

		msgs := session.Interactions.Messages
		if len(msgs) == 0 {
			// should never happen, sessions are always initiated by the user
			// creating an initial message
			continue
		}

		latest := msgs[len(msgs)-1]
		if latest.User == "system" {
			// we've already given a response, don't need to do anything
			continue
		}

		// if we didn't continue here, we need to read from the various channels
		// until we read from the FinishChan
		debugStream := make(chan string)
		outputStream := make(chan string)
		finishChan := make(chan error)

		switch {
		case session.Mode == "Create" && session.Type == "Text":
			// session for text generation
			llm := LanguageModel{
				Interactions: session.Interactions,
				DebugStream:  debugStream,
				OutputStream: outputStream,
				FinishChan:   finishChan,
			}
			go llm.Mistral_7B_Instruct_v0_1(ctx)

		case session.Mode == "Create" && session.Type == "Image":
			// session for image generation

			// TODO: set Prompt, etc, from interactions

			t2i := TextToImage{
				DebugStream:  debugStream,
				OutputStream: outputStream,
				FinishChan:   finishChan,
			}
			go t2i.SDXL_1_0_Base(ctx)

		case session.Mode == "Finetune" && session.Type == "Text":
			// session for text finetuning

			// TODO: we might want to check that we have the QA correctly edited
			// and have the user click an explicit "Start" button before
			// proceeding here

			// TODO: set InputDataset

			llm_ft := FinetuneLanguageModel{
				DebugStream:  debugStream,
				OutputStream: outputStream,
				FinishChan:   finishChan,
			}
			go llm_ft.Mistral_7B_Instruct_v0_1(ctx)

		case session.Mode == "Finetune" && session.Type == "Image":
			// session for image finetuning

			// TODO: we might want to check that we have the image labels
			// correctly added and have the user click an explicit "Start"
			// button (an interaction type) before proceeding here

			// TODO: set InputPath, OutputPath

			t2i_ft := FinetuneTextToImage{
				DebugStream:  debugStream,
				OutputStream: outputStream,
				FinishChan:   finishChan,
			}
			go t2i_ft.SDXL_1_0_Base_Finetune(ctx)

		default:
			return fmt.Errorf("invalid mode or session type")
		}

		firstMessage := true
		addMessage := func(msg string, finished bool) {
			// need to add a system response (computer always has the last word)
			if firstMessage {
				msgs = append(msgs, types.UserMessage{
					User:     "system",
					Message:  msg,
					Uploads:  []string{}, // cool, computer can create images here
					Finished: finished,
				})
				firstMessage = false
			} else {
				latest := msgs[len(msgs)-1]
				latest.Message += msg
				if finished {
					latest.Finished = true
				}
				msgs[len(msgs)-1] = latest
			}
			session.Interactions.Messages = msgs

			// write it to the database. we'll inform any connected webuis over the
			// web interface as well
			s, err := st.UpdateSession(ctx, *session)
			if err != nil {
				log.Printf("Error adding message: %s", err)
			}
			// can i send websockets?
			c.SessionUpdatesChan <- s
		}

		// TODO: handle images coming out of the models
		for {
			select {
			case debugMsg := <-debugStream:
				fmt.Println("Debug message:", debugMsg)
				if DEBUG {
					addMessage(debugMsg, false)
				}
			case outputMsg := <-outputStream:
				fmt.Println("Output message:", outputMsg)
				addMessage(outputMsg, false)

			case err := <-finishChan:
				fmt.Println("Finish chan:", err)
				if err != nil {
					fmt.Println("Error:", err)
					addMessage("\nError: "+err.Error(), true)
				} else {
					fmt.Println("Finished successfully")
					addMessage("", true)
				}
				// maybe factor the main body of this loop into a separate fn so
				// we can just return instead of using goto, heh
				goto nextSession
			}
		}
	nextSession:
	}
	return nil
}
