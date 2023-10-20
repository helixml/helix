// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"fmt"
	"log"

	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/types"
)

// set to false in production (will log messages to web UI)
const DEBUG = true

// the core function - decide which task to give to a worker
// TODO: keep track of the previous tasks run by this worker (and therefore we know which weights are loaded into RAM)
// try to send similar tasks to the same worker
func (c *Controller) ShiftSessionQueue(ctx context.Context, filter types.SessionFilter) (*types.Session, error) {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	// right now this is very dumb - it literally just returns the next thing and doesn't even care what type it is
	// TODO: get the worker auth system plugged in so we know who is asking for the task
	// and then we can keep track of the last thing they ran and pick better
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
		c.sessionQueue = append(c.sessionQueue[:i], c.sessionQueue[i+1:]...)
		return session, nil
	}

	return nil, nil
}

func (c *Controller) ConvertSessionToTask(ctx context.Context, session *types.Session) (*types.WorkerTask, error) {
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
	case session.Mode == "Create" && session.Type == "Text":
		model, err := model.GetLanguageModel(session.ModelName)
		if err != nil {
			return nil, err
		}
		prompt, err := model.GetPrompt(ctx, session)
		if err != nil {
			return nil, err
		}
		task.Prompt = prompt
		return task, nil
	case session.Mode == "Create" && session.Type == "Image":
		return nil, nil
	case session.Mode == "Finetune" && session.Type == "Text":
		return nil, nil
	case session.Mode == "Finetune" && session.Type == "Image":
		return nil, nil
	}
	return nil, nil
}

// add the given session onto the end of the queue
// unless it's already waiting and present in the queue
// in which case let's replace it at it's current position
func (c *Controller) PushSessionQueue(ctx context.Context, session *types.Session) error {
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
	return nil
}

func (c *Controller) AddActiveSession(ctx context.Context, session *types.Session) error {
	c.activeSessionMtx.Lock()
	defer c.activeSessionMtx.Unlock()
	c.activeSessions[session.ID] = session
	return nil
}

func (c *Controller) GetActiveSession(ctx context.Context, id string) (*types.Session, error) {
	c.activeSessionMtx.Lock()
	defer c.activeSessionMtx.Unlock()
	session, ok := c.activeSessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	return session, nil
}

func (c *Controller) RemoveActiveSession(ctx context.Context, id string) error {
	c.activeSessionMtx.Lock()
	defer c.activeSessionMtx.Unlock()
	if _, ok := c.activeSessions[id]; !ok {
		return fmt.Errorf("session not found")
	}
	delete(c.activeSessions, id)
	return nil
}

// if the action is "begin" - then we need to ceate a new textstream that is hooked up correctly
// then we stash that in a map
// if the action is "continue" - load the textstream and write to it
// if the action is "end" - unload the text stream
func (c *Controller) HandleWorkerResponse(ctx context.Context, taskResponse *types.WorkerTaskResponse) (*types.WorkerTaskResponse, error) {
	fmt.Printf("taskResponse --------------------------------------\n")
	fmt.Printf("%+v --------------------------------------\n", taskResponse)
	
	// session, err := c.Options.Store.GetSession(ctx, taskResponse.SessionID)
	// if err != nil {
	// 	return nil, err
	// }
	if taskResponse.Action == types.WorkerTaskResponseAction_Begin {
		return taskResponse, nil
	} else if taskResponse.Action == types.WorkerTaskResponseAction_Continue {
		return taskResponse, nil
	} else if taskResponse.Action == types.WorkerTaskResponseAction_End {
		return taskResponse, nil
	} else {
		return nil, nil
	}
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

		sessionQueue = append(sessionQueue, session)
	}

	// now we have the queue in oldest first order
	c.sessionQueue = sessionQueue
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
