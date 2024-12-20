package main

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
)

// TestCase defines the structure for a single integration test
type TestCase struct {
	Name        string                   // Display name of the test
	Description string                   // Detailed description of test purpose
	Timeout     time.Duration            // Maximum time allowed for test execution
	Run         func(*rod.Browser) error // Function containing test logic
}

// TestSuite contains all smoke tests to be run
var TestSuite = []TestCase{
	HomepageLoadTest(),
	LoginFlowTest(),
	StartNewSessionTest(),
}

// HomepageLoadTest verifies that the homepage loads successfully
func HomepageLoadTest() TestCase {
	return TestCase{
		Name:        "Homepage Load",
		Description: "Verifies that the homepage loads successfully",
		Timeout:     10 * time.Second,
		Run: func(browser *rod.Browser) error {
			logStep("Navigating to homepage")
			page := browser.MustPage(getServerURL())

			logStep("Waiting for page load")
			page.MustWaitLoad()

			logStep("Verifying page loaded successfully")
			if !page.MustHas("body") {
				return fmt.Errorf("homepage failed to load properly")
			}
			return nil
		},
	}
}

// LoginFlowTest tests the login functionality
func LoginFlowTest() TestCase {
	return TestCase{
		Name:        "Login Flow",
		Description: "Tests the login functionality using credentials or saved cookies",
		Timeout:     10 * time.Second,
		Run:         performLogin,
	}
}

// StartNewSessionTest tests creating a new chat session
func StartNewSessionTest() TestCase {
	return TestCase{
		Name:        "Start New Session",
		Description: "Tests starting a new chat session after login",
		Timeout:     20 * time.Second,
		Run: func(browser *rod.Browser) error {
			if err := performLogin(browser); err != nil {
				return err
			}

			page := browser.MustPage(getServerURL())
			page.MustWaitStable()

			if err := startNewChat(page); err != nil {
				return err
			}

			if err := sendMessage(page); err != nil {
				return err
			}

			return nil
		},
	}
}

// startNewChat attempts to create a new chat session by clicking the New Session button
// Returns error if button is not found or if the operation fails
func startNewChat(page *rod.Page) error {
	logStep("Looking for New Session button")
	xpath := fmt.Sprintf(`//span[contains(text(), '%s')]`, "New Session")
	elements := page.MustElementsX(xpath)
	if len(elements) != 1 {
		return fmt.Errorf("new Session button not found")
	}

	logStep("Clicking New Session button")
	elements[0].MustClick()

	logStep("Waiting for page to stabilize")
	page.MustWaitStable()
	return nil
}

// sendMessage simulates sending a test message in the chat interface
// It types "hello helix", sends the message, and verifies the message appears
// Returns error if any step fails (input not found, send button not found, or message not displayed)
func sendMessage(page *rod.Page) error {
	logStep("Looking for chat input textarea")
	textarea := page.MustElement("textarea")

	logStep("Typing 'hello helix' into chat input")
	textarea.MustInput("hello helix")

	logStep("Looking for send button")
	sendButton := page.MustElement("#sendButton")
	sendButton.MustClick()

	logStep("Waiting for message to be sent")
	page.MustWaitStable()

	logStep("Looking for chat message")
	chatMessage := page.MustElement("div.interactionMessage")
	if chatMessage == nil {
		return fmt.Errorf("chat message not found")
	}

	return nil
}
