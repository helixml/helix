package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
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
	UploadPDFFileTest(),
	CreateRagAppTest(),
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
		Run: func(browser *rod.Browser) error {
			logStep("Launching browser")
			page := browser.MustPage(getServerURL())
			page.MustWaitLoad()
			if err := performLogin(page); err != nil {
				return err
			}
			return nil
		},
	}
}

// StartNewSessionTest tests creating a new chat session
func StartNewSessionTest() TestCase {
	return TestCase{
		Name:        "Start New Session",
		Description: "Tests starting a new chat session after login",
		Timeout:     20 * time.Second,
		Run: func(browser *rod.Browser) error {
			logStep("Launching browser")
			page := browser.MustPage(getServerURL())
			page.MustWaitLoad()
			if err := performLogin(page); err != nil {
				return err
			}

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

const (
	folderName = "smoke"
)

func UploadPDFFileTest() TestCase {
	return TestCase{
		Name:        "Upload PDF File",
		Description: "Tests uploading a PDF file",
		Timeout:     30 * time.Second,
		Run:         uploadPDFFile,
	}
}

func uploadPDFFile(browser *rod.Browser) error {
	logStep("Launching browser")
	page := browser.
		DefaultDevice(devices.LaptopWithHiDPIScreen.Landscape()).
		MustPage(getServerURL() + "/files")
	page.MustWaitLoad()
	if err := performLogin(page); err != nil {
		return err
	}

	// Create test folder
	xpath := fmt.Sprintf(`//button[contains(text(), '%s')]`, "Create Folder")
	elements := page.MustElementsX(xpath)
	if len(elements) != 1 {
		return fmt.Errorf("create folder button not found")
	}

	logStep("Clicking Create Folder button")
	elements[0].MustClick()

	logStep("Looking for chat input textarea")
	textarea := page.MustElement("input[type='text']")

	logStep("Typing folder name")
	textarea.MustInput(folderName)

	logStep("Clicking Submit button")
	sendButton := page.MustElement("#submitButton")
	sendButton.MustClick()

	logStep(fmt.Sprintf("clicking on the %s folder", folderName))
	folder := page.MustElementX(fmt.Sprintf(`//a[contains(text(), '%s')]`, folderName))
	folder.MustClick()

	logStep("waiting for page to stabilize")
	page.MustWaitStable()

	logStep("clicking on the upload file button")
	upload := page.MustElement("input[type='file']")
	upload.MustSetFiles("/Users/phil/code/helixml/helix/integration-test/data/smoke/hr-guide.pdf")
	page.MustReload()

	logStep("waiting for file to exist, then clicking on it")
	file := page.MustElementX(fmt.Sprintf(`//a[contains(text(), '%s')]`, "hr-guide.pdf"))
	file.MustClick()

	// Since files are downloaded in the context of the container, we can't get access to the file
	// to make sure it's downloaded. Assume it is.

	return nil
}

func CreateRagAppTest() TestCase {
	return TestCase{
		Name:        "Create RAG App",
		Description: "Tests creating a RAG app",
		Timeout:     120 * time.Second,
		Run:         createRagApp,
	}
}

func createRagApp(browser *rod.Browser) error {
	logStep("Launching browser")
	page := browser.
		DefaultDevice(devices.LaptopWithHiDPIScreen.Landscape()).
		MustPage(getServerURL())
	page.MustWaitLoad()
	if err := performLogin(page); err != nil {
		return err
	}

	logStep("Browsing to the apps page")
	page.MustElement("button[aria-controls='menu-appbar']").MustClick()
	page.MustElementX(`//li[contains(text(), 'Your Apps')]`).MustClick()

	logStep("Creating a new app")
	page.MustElement("#new-app-button").MustClick()
	page.MustWaitStable()

	logStep("Save initial app")
	page.MustElement("#app-name").MustInput(fmt.Sprintf("smoke-%d", time.Now().Unix()))
	page.MustElementX(`//button[text() = 'Save']`).MustClick()
	page.MustWaitStable()

	logStep("Adding knowledge")
	page.MustElementX(`//button[text() = 'Knowledge']`).MustClick()

	logStep("Adding knowledge source")
	page.MustElementX(`//button[text() = 'Add Knowledge Source']`).MustClick()
	page.MustElement(`input[value=filestore]`).MustClick()
	page.MustElement(`input[type=text]`).MustInput(folderName)
	page.MustElementX(`//button[text() = 'Add']`).MustClick()

	logStep("Save the app again")
	page.MustElementX(`//button[text() = 'Save']`).MustClick()
	page.MustWaitStable()

	logStep("clicking on the upload file button")
	upload := page.MustElement("input[type='file']")

	wait1 := page.MustWaitRequestIdle()
	upload.MustSetFiles("/integration-test/data/smoke/hr-guide.pdf")
	wait1()

	page.MustReload()

	logStep("Waiting for knowledge source to be ready")
	page.MustElementX(`//span[contains(text(), 'ready')]`)

	logStep("Testing the app")
	page.MustElement("#textEntry").MustInput("do you have a shoe policy")
	page.MustElement("#sendButton").MustClick()

	message := page.MustElement(".interactionMessage")
	if !strings.Contains(message.MustText(), "shoe policy") {
		return fmt.Errorf("app did not respond with the correct answer")
	}
	logStep("App responded with the correct answer")

	return nil
}
