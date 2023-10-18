// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"log"

	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

// load all jobs that are currently running and check if they are still running
func (c *Controller) triggerSessionTasks(ctx context.Context) error {
	log.Printf("I have a %+v", c.Options.Store)

	// NB: for the demo, being serialized here is good: it means we'll only
	// spawn one GPU task at a time, and run less risk of GPU OOM. Later, we'll
	// need to figure out how to scale this, which is what the
	// Lilypad/Kubernetes schedulers are for

	st := c.Options.Store
	// fetch all sessions
	sessions, err := st.GetSessions(ctx, store.GetSessionsQuery{})
	if err != nil {
		return err
	}

	for _, session := range sessions {
		msgs := session.Interactions.Messages
		if len(msgs) == 0 {
			// should never happen, sessions are always initiated by the user
			// creating an initial message
			continue
		}
		latest := msgs[len(msgs)-1]
		if latest.User == "user" {
			// need to add a system response (computer always has the last word)
			msgs = append(msgs, types.UserMessage{
				User:     "system",
				Message:  "oh, hai there",
				Uploads:  []string{}, // cool, computer can create images here
				Finished: false,
			})
		}
		if latest.User == "system" {
			if len(latest.Message) < 100 {
				latest.Message += " - never gonna give you up"
			} else {
				latest.Finished = true
			}
			msgs[len(msgs)-1] = latest
		}
		session.Interactions.Messages = msgs
		// write it to the database. we'll inform any connected webuis over the
		// web interface as well
		s, err := st.UpdateSession(ctx, *session)
		if err != nil {
			return err
		}
		// can i send websockets?
		c.SessionUpdatesChan <- s
	}

	return nil
}
