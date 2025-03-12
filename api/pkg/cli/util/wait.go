package util

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

// WaitForKnowledgeReady waits for all knowledge associated with an app to be in the Ready state.
// It polls the API every 2 seconds to check the status of all knowledge sources.
//
// Parameters:
// - ctx: Parent context for the operation
// - apiClient: Helix API client
// - appID: ID of the app whose knowledge sources to check
// - timeout: Maximum duration to wait for knowledge sources to be ready
//
// Returns:
// - error if any knowledge source fails to index, reaches an error state, or if the timeout expires
//
// The function provides status updates when knowledge sources change state and
// immediately returns an error if any source enters the error state.
func WaitForKnowledgeReady(ctx context.Context, apiClient client.Client, appID string, timeout time.Duration) error {
	fmt.Printf("Waiting for knowledge to be ready for app %s...\n", appID)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	knowledgeFilter := &client.KnowledgeFilter{
		AppID: appID,
	}

	// Poll until all knowledge is ready
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	printedStatus := make(map[string]types.KnowledgeState)
	progressCounter := 0

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for knowledge to be ready")
		case <-ticker.C:
			// Print a progress indicator
			progressCounter++
			if progressCounter%5 == 0 {
				fmt.Print(".")
				if progressCounter%50 == 0 {
					fmt.Println() // New line every 50 dots
				}
			}

			knowledge, err := apiClient.ListKnowledge(ctx, knowledgeFilter)
			if err != nil {
				fmt.Println() // End the dots line if we're erroring out
				return fmt.Errorf("failed to list knowledge: %w", err)
			}

			// If no knowledge sources, we're done
			if len(knowledge) == 0 {
				fmt.Println("\nNo knowledge sources found, continuing...")
				return nil
			}

			// Check status of all knowledge
			allReady := true

			for _, k := range knowledge {
				// Print status changes
				if printedStatus[k.ID] != k.State {
					fmt.Printf("Knowledge '%s' status: %s\n", k.Name, k.State)
					printedStatus[k.ID] = k.State
				}

				switch k.State {
				case types.KnowledgeStateError:
					return fmt.Errorf("knowledge '%s' failed with error: %s", k.Name, k.Message)
				case types.KnowledgeStateReady:
					continue
				case types.KnowledgeStatePreparing:
					// Knowledge in "Preparing" state is waiting for explicit user action
					// We should warn about this but not consider it an error
					fmt.Printf("Warning: Knowledge '%s' is in 'preparing' state and waiting for explicit completion.\n", k.Name)
					fmt.Printf("To complete preparation, call: apiClient.CompleteKnowledgePreparation(ctx, \"%s\")\n", k.ID)
					allReady = false
				default:
					allReady = false
				}
			}

			if allReady {
				fmt.Println("All knowledge sources are ready!")
				return nil
			}
		}
	}
}
