// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"fmt"
	"log"

	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

// set to false in production (will log messages to web UI)
const DEBUG = true

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
				DebugStream:  debugStream,
				OutputStream: outputStream,
				FinishChan:   finishChan,
			}
			go llm.Mistral_7B_Instruct_v0_1(ctx)

		case session.Mode == "Create" && session.Type == "Image":
			// session for image generation
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

			t2i_ft := FinetuneTextToImage{}
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
