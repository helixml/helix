package helper

import (
	"fmt"
	"strconv"
	"strings"
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

func WaitForHelixResponse(t *testing.T, page *rod.Page, expectedResponse string) {
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			message := page.MustElementX("(//div[@class = 'interactionMessage'])[last()]").MustText()
			LogAndFail(t, fmt.Sprintf("Incorrect response from app, expected '%s', got: %s", expectedResponse, message))
		case <-ticker.C:
			message := page.MustElementX("(//div[@class = 'interactionMessage'])[last()]").MustText()
			if strings.Contains(strings.ToLower(message), strings.ToLower(expectedResponse)) {
				LogStep(t, "App responded with the correct answer")
				return
			}
		}
	}
}
