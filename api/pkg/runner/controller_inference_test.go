package runner

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/stretchr/testify/assert"
)

// MockExecCmd is a mock implementation of ExecCmd for testing
type MockExecCmd struct {
	*gomock.Controller
	startFunc  func() error
	waitFunc   func() error
	stderrPipe func() (io.ReadCloser, error)
}

func NewMockExecCmd(ctrl *gomock.Controller) *MockExecCmd {
	return &MockExecCmd{Controller: ctrl}
}

func (m *MockExecCmd) Start() error {
	if m.startFunc != nil {
		return m.startFunc()
	}
	return nil
}

func (m *MockExecCmd) Wait() error {
	if m.waitFunc != nil {
		return m.waitFunc()
	}
	return nil
}

func (m *MockExecCmd) StderrPipe() (io.ReadCloser, error) {
	if m.stderrPipe != nil {
		return m.stderrPipe()
	}
	return nil, nil
}

// MockReadCloser is a mock implementation of io.ReadCloser
type MockReadCloser struct {
	*gomock.Controller
	readFunc  func(p []byte) (n int, err error)
	closeFunc func() error
}

func NewMockReadCloser(ctrl *gomock.Controller) *MockReadCloser {
	return &MockReadCloser{Controller: ctrl}
}

func (m *MockReadCloser) Read(p []byte) (n int, err error) {
	if m.readFunc != nil {
		return m.readFunc(p)
	}
	return 0, io.EOF
}

func (m *MockReadCloser) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestCreateInferenceModelInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	curShellCommander := ollamaCommander
	defer func() { ollamaCommander = curShellCommander }()
	curPortFinder := freePortFinder
	defer func() { freePortFinder = curPortFinder }()

	// Mock command to override the ollama command
	cmd := exec.Command("sleep", "999999")

	// Create a mock commander
	mockCommander := NewMockCommander(ctrl)
	mockCommander.EXPECT().LookPath("ollama").Return("ollama", nil)
	mockCommander.EXPECT().CommandContext(gomock.Any(), "ollama", "serve").Return(cmd)
	// Swap out the commander for the mock
	ollamaCommander = mockCommander

	// Create a mock free port finder
	mockFreePortFinder := NewMockFreePortFinder(ctrl)
	port, err := freeport.GetFreePort()
	assert.NoError(t, err)
	mockFreePortFinder.EXPECT().GetFreePort().Return(port, nil)
	// Swap out the free port finder for the mock
	freePortFinder = mockFreePortFinder

	// Add the runner options
	modelName := types.Model_Ollama_Llama3_8b

	ctx := context.Background()
	runner := &Runner{
		Ctx: context.Background(),
		Options: RunnerOptions{
			ID:          "test-id",
			ApiHost:     "http://localhost",
			ApiToken:    "test-token",
			MemoryBytes: 1024,
			Config: &config.RunnerConfig{
				Runtimes: config.Runtimes{
					Ollama: config.OllamaRuntimeConfig{
						Enabled:      true,
						WarmupModels: []string{string(modelName)},
						InstanceTTL:  1 * time.Millisecond,
					},
				},
			},
		},
		activeModelInstances: xsync.NewMapOf[string, ModelInstance](),
	}

	request := &types.RunnerLLMInferenceRequest{
		Request: &openai.ChatCompletionRequest{
			Model: string(modelName),
		},
	}

	// The code is expecting an ollama API to be running, so we need to start a mock http server
	// create a listener with the desired port.
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		log.Fatal(err)
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("ollama mock server hit")
		fmt.Println(r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	// NewUnstartedServer creates a listener. Close that listener and replace
	// with the one we created.
	ts.Listener.Close()
	ts.Listener = l

	// Start the server.
	ts.Start()
	// Stop the server on return from the function.
	defer ts.Close()

	err = runner.createInferenceModelInstance(ctx, request)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check that the cmd.Env contains expected env vars
	assert.Contains(t, cmd.Env, "OLLAMA_KEEP_ALIVE=-1")
	assert.Contains(t, cmd.Env, "HTTP_PROXY="+os.Getenv("HTTP_PROXY"))
	assert.Contains(t, cmd.Env, "HTTPS_PROXY="+os.Getenv("HTTPS_PROXY"))
	assert.Contains(t, cmd.Env, "OLLAMA_HOST="+fmt.Sprintf("0.0.0.0:%d", port))
	assert.Contains(t, cmd.Env, "OLLAMA_MODELS="+runner.Options.CacheDir)

	// Check if the model instance was stored
	var found bool
	runner.activeModelInstances.Range(func(key string, value ModelInstance) bool {
		found = true
		return false
	})

	if !found {
		t.Fatalf("expected model instance to be stored, but it was not")
	}

	// Wait while the model instance is in use
	var stale = false
	for !stale {
		runner.activeModelInstances.Range(func(key string, value ModelInstance) bool {
			stale = value.Stale()
			return false
		})
	}

	var pidStatusCode string
	pidStatusCode, _ = getPidStatus(cmd.Process.Pid)
	assert.Contains(t, pidStatusCode, "S")

	// We've set the model instance to be stale after 1ms, so it should kill
	aiModel, err := model.GetModel(modelName)
	assert.NoError(t, err)
	err = runner.checkForStaleModelInstances(ctx, aiModel, types.SessionModeInference)
	assert.NoError(t, err)

	for {
		pidStatusCode, _ = getPidStatus(cmd.Process.Pid)
		if pidStatusCode == "" {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	assert.Equal(t, pidStatusCode, "")
}
