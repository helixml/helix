package helper

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const (
	// This is the location inside the rod container
	TestPDFFile = "/integration-test/data/smoke/hr-guide.pdf"
)

func LogStep(t *testing.T, step string) {
	t.Logf("⏩ %s", step)
}

func LogAndFail(t *testing.T, message string) {
	t.Logf("❌ %s", message)
	t.FailNow()
}

func GetServerURL() string {
	url := os.Getenv("SERVER_URL")
	if url == "" {
		log.Fatal("SERVER_URL environment variable is not set")
	}
	return url
}

func GetHelixUser() string {
	user := os.Getenv("HELIX_USER")
	if user == "" {
		log.Fatal("HELIX_USER environment variable is not set")
	}
	return user
}

func GetHelixPassword() string {
	password := os.Getenv("HELIX_PASSWORD")
	if password == "" {
		log.Fatal("HELIX_PASSWORD environment variable is not set")
	}
	return password
}

func PerformLogin(t *testing.T, page *rod.Page) error {
	if err := loginWithCredentials(t, page); err != nil {
		return err
	}
	return verifyLogin(t, page)
}

func loginWithCredentials(t *testing.T, page *rod.Page) error {
	LogStep(t, "Looking for login button")
	loginBtn := page.MustElement("button[id='login-button']")
	err := loginBtn.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		LogStep(t, "Login button not found, must be already logged in")
		return nil
	}

	LogStep(t, "Waiting for login page to load")
	page.MustWaitLoad()

	LogStep(t, "Getting credentials from environment")
	username := GetHelixUser()
	password := GetHelixPassword()

	LogStep(t, "Filling in username and password")
	page.MustElement("input[type='text']").MustInput(username)
	page.MustElement("input[type='password']").MustInput(password)
	page.MustElement("input[type='submit']").MustClick()
	page.MustWaitStable()

	return nil
}

func verifyLogin(t *testing.T, page *rod.Page) error {
	LogStep(t, "Verifying login")
	username := GetHelixUser()
	xpath := fmt.Sprintf(`//span[contains(text(), '%s')]`, username)
	el := page.MustElementX(xpath)
	if el == nil {
		return fmt.Errorf("login failed - username not found")
	}
	return nil
}

func StartNewChat(t *testing.T, page *rod.Page) error {
	LogStep(t, "Looking for New Session button")
	xpath := fmt.Sprintf(`//span[contains(text(), '%s')]`, "New Session")
	elements := page.MustElementsX(xpath)
	if len(elements) != 1 {
		return fmt.Errorf("new Session button not found")
	}

	LogStep(t, "Clicking New Session button")
	elements[0].MustClick()

	LogStep(t, "Waiting for page to stabilize")
	page.MustWaitStable()
	return nil
}

func SendMessage(t *testing.T, page *rod.Page) error {
	LogStep(t, "Looking for chat input textarea")
	textarea := page.MustElement("textarea")

	LogStep(t, "Typing 'hello helix' into chat input")
	textarea.MustInput("hello helix")

	LogStep(t, "Looking for send button")
	sendButton := page.MustElement("#sendButton")
	sendButton.MustClick()

	LogStep(t, "Waiting for message to be sent")
	page.MustWaitStable()

	LogStep(t, "Looking for chat message")
	chatMessage := page.MustElement("div.interactionMessage")
	if chatMessage == nil {
		return fmt.Errorf("chat message not found")
	}

	return nil
}

func SetTestTimeout(t *testing.T, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel) // Register the cancel function to be called when the test finishes
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			LogAndFail(t, "Test timed out")
		}
	}()
	return ctx
}
