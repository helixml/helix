package helper

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/stretchr/testify/require"
)

const (
	// This is the location inside the rod container
	TestPDFFile = "/integration-test/data/smoke/hr-guide.pdf"
)

func LogStep(t *testing.T, step string) {
	_, file, line, _ := runtime.Caller(1) // Get caller info, skip 1 frame to get the caller rather than this function
	timestamp := time.Now().Format("15:04:05.000")
	t.Logf("[%s] ⏩ %s (at %s:%d)", timestamp, step, filepath.Base(file), line)
}

func LogAndFail(t *testing.T, message string) {
	t.Logf("❌ %s", message)
	t.FailNow()
}

func LogAndPass(t *testing.T, message string) {
	t.Logf("✅ %s", message)
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
	page.MustElement("button[id='login-button']").MustClick()

	LogStep(t, "Getting credentials from environment")
	username := GetHelixUser()
	password := GetHelixPassword()

	LogStep(t, "Filling in username and password")
	page.MustElementX("//input[@type='text']").MustWaitVisible().MustInput(username)
	page.MustElementX("//input[@type='password']").MustWaitVisible().MustInput(password)
	page.MustElementX("//input[@type='submit']").MustWaitVisible().MustClick()

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

func StartNewChat(t *testing.T, page *rod.Page) {
	LogStep(t, "Clicking New Session button")
	page.MustElementX(`//span[contains(text(), 'New Session')]`).MustWaitVisible().MustClick()
}

func SendMessage(t *testing.T, page *rod.Page, message string) {
	LogStep(t, "Looking for chat input textarea")
	textarea := page.MustElement("textarea")

	LogStep(t, fmt.Sprintf("Typing '%s' into chat input", message))
	textarea.MustWaitVisible().MustInput(message)

	LogStep(t, "Looking for send button")
	page.MustElement("#sendButton").MustWaitInteractable().MustClick()
}

func StartNewImageSession(t *testing.T, page *rod.Page) error {
	LogStep(t, "Creating new session")
	page.MustElementX(`//span[contains(text(), 'New Session')]`).MustClick()

	LogStep(t, "Selecting Image mode")
	page.MustElementX(`//button[contains(text(), 'TEXT')]`).MustClick()

	// Button must now say "IMAGE"
	page.MustElementX(`//button[contains(text(), 'IMAGE')]`)

	return nil
}

func CreateContext(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel) // Register the cancel function to be called when the test finishes
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			LogAndFail(t, "Test timed out")
		}
	}()
	return ctx
}

func GetFirstAPIKey(t *testing.T, page *rod.Page) string {
	LogStep(t, "Getting API key")
	page.MustNavigate(fmt.Sprintf("%s/account", GetServerURL()))
	apiKey := page.MustElementX("(//p[contains(text(),'hl-')])[1]").MustText()
	require.NotEmpty(t, apiKey, "API key should be set")
	return apiKey
}

func DownloadFile(t *testing.T, url string, dir string) string {
	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err)
	LogStep(t, fmt.Sprintf("Downloading %s", url))
	downloadCmd := exec.Command("curl", "-sL", "-O", url)
	downloadCmd.Dir = dir
	output, err := downloadCmd.CombinedOutput()
	require.NoError(t, err, "Failed to download %s: %s", path.Base(url), string(output))
	return path.Join(dir, path.Base(url))
}

func DownloadRepository(t *testing.T, dir string) string {
	url := "https://github.com/helixml/helix/archive/refs/heads/main.zip"
	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err)
	LogStep(t, fmt.Sprintf("Downloading %s", url))
	downloadCmd := exec.Command("curl", "-sL", "-o", path.Join(dir, "helix-main.zip"), url)
	output, err := downloadCmd.CombinedOutput()
	require.NoError(t, err, "Failed to download %s: %s", path.Base(url), string(output))

	unzipCmd := exec.Command("unzip", "-q", path.Join(dir, "helix-main.zip"), "-d", dir)
	output, err = unzipCmd.CombinedOutput()
	require.NoError(t, err, "Failed to unzip %s: %s", path.Base(url), string(output))

	return path.Join(dir, "helix-main")
}
