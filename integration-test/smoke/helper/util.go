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
	TestPDFFilename   = "hr-guide.pdf"
	testFileDirectory = "integration-test/data/smoke"
)

func GetTestPDFFile() string {
	// If the BROWSER_URL environment variable is set, we are running in a container
	// and we need to use the container path.
	containerPath := path.Join("/", testFileDirectory, TestPDFFilename)
	if os.Getenv("BROWSER_URL") != "" {
		return containerPath
	}

	// Otherwise, we are running locally, so we need to use the local path.
	_, filename, _, _ := runtime.Caller(0)
	rootDir := path.Clean(path.Join(filepath.Dir(filename), "..", "..", ".."))
	// Build the full local path to the test file
	localPath := path.Join(rootDir, testFileDirectory, TestPDFFilename)
	return localPath
}

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

func SendMessage(t *testing.T, page *rod.Page, message string) {
	LogStep(t, "Looking for chat input textarea")
	textarea := page.MustElement("textarea")

	LogStep(t, fmt.Sprintf("Typing '%s' into chat input", message))
	textarea.MustWaitVisible().MustInput(message)

	LogStep(t, "Looking for send button")
	page.MustElementX("//div[@aria-label='Send Prompt']").MustWaitInteractable().MustClick()
}

func StartNewImageSession(t *testing.T, page *rod.Page) error {
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
