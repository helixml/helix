// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"log"

	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

// load all jobs that are currently running and check if they are still running
func (c *Controller) triggerSessionTasks(ctx context.Context) error {
	log.Printf("I have a %+v", c.Options.Store)

	// can i send websockets?
	c.SessionUpdatesChan <- &types.Session{ID: "elephant"}

	return nil
}
