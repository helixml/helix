package helper

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"golang.org/x/exp/rand"
)

func BrowseToAppsPage(t *testing.T, page *rod.Page) {
	LogStep(t, "Browsing to the apps page")
	page.MustElement("button[aria-controls='menu-appbar']").MustClick()
	page.MustElementX(`//li[contains(text(), 'Your Apps')]`).MustClick()
}

func CreateNewApp(t *testing.T, page *rod.Page) {
	LogStep(t, "Creating a new app")
	page.MustElement("#new-app-button").MustClick()
	page.MustWaitStable()
	random := rand.Intn(1000000)
	appName := "smoke-" + time.Now().Format("20060102150405") + "-" + strconv.Itoa(random)
	page.MustElement("#app-name").MustInput(appName)
	page.MustWaitStable()
	SaveApp(t, page)
}

func SaveApp(t *testing.T, page *rod.Page) {
	LogStep(t, "Saving app")
	page.MustElementX(`//button[text() = 'Save']`).MustClick()
	page.MustWaitStable()
}

// This function checks to see if Helix has responded. It doesn't check the text.
func WaitForHelixResponse(t *testing.T, page *rod.Page) {
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			LogAndFail(t, "App did not respond with an answer")
		case <-ticker.C:
			responses := page.MustElementsX("(//div[@class = 'interactionMessage'])")
			if len(responses) < 2 {
				// Must have the initial message and a response
				continue
			}
			lastMessage := responses[len(responses)-1].MustText()
			if len(lastMessage) < 10 {
				// Response must be at least 10 characters
				continue
			}
			LogStep(t, fmt.Sprintf("App responded with an answer: %s", lastMessage))
			return
		}
	}
}
