package helper

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"golang.org/x/exp/rand" //nolint:staticcheck
)

func BrowseToAppsPage(t *testing.T, page *rod.Page) {
	LogStep(t, "Browsing to the apps page, this sometimes struggles a bit... if it keeps happening then we should just browse to the apps page directly")
	page.MustElementX(`//button[@aria-controls='menu-appbar']`).MustWaitVisible().MustClick()
	page.MustElementX(`//li[contains(text(), 'Your Apps')]`).MustWaitVisible().MustClick()
	page.MustElementX(`//*[@data-testid='DeveloperBoardIcon']`).MustWaitVisible() // Old session list is loaded when apps are loaded
}

func CreateNewApp(t *testing.T, page *rod.Page) {
	LogStep(t, "Creating a new app")
	page.MustElementX(`//*[@id="new-app-button"]`).MustWaitVisible().MustClick()
	random := rand.Intn(1000000)
	appName := "smoke-" + time.Now().Format("20060102150405") + "-" + strconv.Itoa(random)
	page.MustElementX(`//*[@id="app-name"]`).MustWaitVisible().MustInput(appName)
	SaveApp(t, page)
	LogStep(t, fmt.Sprintf("Created app: %s", page.MustInfo().URL))
}

func SaveApp(t *testing.T, _ *rod.Page) {
	LogStep(t, "No more save button!")
}

// This function checks to see if Helix has responded. It doesn't check the text.
func WaitForHelixResponse(ctx context.Context, t *testing.T, page *rod.Page) {
	LogStep(t, "Waiting for Helix to respond")
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			responses := page.MustElementsX("(//div[@class = 'interactionMessage'])")
			if len(responses) < 2 {
				// Must have the initial message and a response
				continue
			}
			lastMessage := responses[len(responses)-1].MustText()
			if len(lastMessage) < 1 {
				// Response must be at least 1 characters
				continue
			}
			LogStep(t, fmt.Sprintf("App responded with an answer: %s", lastMessage))
			return
		}
	}
}

func TestApp(ctx context.Context, t *testing.T, page *rod.Page, question string) {
	LogStep(t, "Typing question into the app")
	page.MustElementX(`//textarea[@id='textEntry']`).MustWaitVisible().MustInput(question)

	LogStep(t, "Clicking send button")
	page.MustElementX(`//button[@id='sendButton']`).MustWaitInteractable().MustClick()

	WaitForHelixResponse(ctx, t, page)
}
