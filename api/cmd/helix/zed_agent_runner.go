package helix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

// ZedAgentRunner manages actual Zed process execution
type ZedAgentRunner struct {
	cfg              *config.ExternalAgentRunnerConfig
	activeSessions   map[string]*ZedProcess
	activeInstances  map[string]*ZedInstance
	mutex            sync.RWMutex
	wsPort           int
	zedBinaryPath    string
	workspaceBaseDir string
	nextDisplayNum   int
	nextRDPPort      int
}

// ZedProcess represents a running Zed process
type ZedProcess struct {
	SessionID    string
	InstanceID   string
	ThreadID     string
	UserID       string
	ProjectPath  string
	WorkDir      string
	Environment  []string
	Process      *exec.Cmd
	DisplayNum   int
	RDPPort      int
	StartTime    time.Time
	LastActivity time.Time
	Status       string
}

// ZedInstance represents a Zed instance with multiple threads
type ZedInstance struct {
	InstanceID   string
	SpecTaskID   string
	ProjectPath  string
	Threads      map[string]*ZedProcess
	Status       string
	CreatedAt    time.Time
	LastActivity time.Time
	GitRepoURL   string
}

// NewZedAgentRunner creates a new Zed agent runner
func NewZedAgentRunner(cfg *config.ExternalAgentRunnerConfig) *ZedAgentRunner {
	return &ZedAgentRunner{
		cfg:              cfg,
		activeSessions:   make(map[string]*ZedProcess),
		activeInstances:  make(map[string]*ZedInstance),
		zedBinaryPath:    getZedBinaryPath(),
		workspaceBaseDir: getWorkspaceBaseDir(),
		nextDisplayNum:   2,    // Start from :2 (supervisor uses :1)
		nextRDPPort:      5910, // Start from 5910
	}
}

// Run starts the Zed agent runner
func (r *ZedAgentRunner) Run(ctx context.Context) error {
	// Start WebSocket server for Zed agent communication
	err := r.startWebSocketServer(ctx)
	if err != nil {
		return fmt.Errorf("failed to start WebSocket server: %w", err)
	}

	log.Info().Msg("Zed agent runner started and listening for requests")

	// Wait for shutdown
	<-ctx.Done()

	// Cleanup all active sessions
	r.cleanup()

	return nil
}

// handleZedAgentRequest processes incoming Zed agent requests
func (r *ZedAgentRunner) handleZedAgentRequest(data []byte) error {
	var agent types.ZedAgent
	if err := json.Unmarshal(data, &agent); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal Zed agent request")
		return err
	}

	log.Info().
		Str("session_id", agent.SessionID).
		Str("instance_id", agent.InstanceID).
		Str("user_id", agent.UserID).
		Msg("Processing Zed agent request")

	ctx := context.Background() // Create context for Zed operations
	if agent.InstanceID != "" {
		return r.startZedInstance(ctx, &agent)
	} else {
		return r.startZedSession(ctx, &agent)
	}
}

// startZedInstance starts a new Zed instance or adds thread to existing instance
func (r *ZedAgentRunner) startZedInstance(ctx context.Context, agent *types.ZedAgent) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Get or create instance
	instance, exists := r.activeInstances[agent.InstanceID]
	if !exists {
		instance = &ZedInstance{
			InstanceID:   agent.InstanceID,
			SpecTaskID:   agent.SessionID,
			ProjectPath:  agent.ProjectPath,
			Threads:      make(map[string]*ZedProcess),
			Status:       "creating",
			CreatedAt:    time.Now(),
			LastActivity: time.Now(),
		}
		r.activeInstances[agent.InstanceID] = instance
	}

	// Create new thread in instance
	threadID := agent.ThreadID
	if threadID == "" {
		threadID = fmt.Sprintf("thread_%d", len(instance.Threads)+1)
	}

	process, err := r.createZedProcess(agent, instance)
	if err != nil {
		return fmt.Errorf("failed to create Zed process: %w", err)
	}

	instance.Threads[threadID] = process
	instance.LastActivity = time.Now()
	instance.Status = "active"

	log.Info().
		Str("instance_id", agent.InstanceID).
		Str("thread_id", threadID).
		Str("session_id", agent.SessionID).
		Msg("Created Zed thread in instance")

	return nil
}

// startZedSession starts a single Zed session
func (r *ZedAgentRunner) startZedSession(ctx context.Context, agent *types.ZedAgent) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	process, err := r.createZedProcess(agent, nil)
	if err != nil {
		return fmt.Errorf("failed to create Zed process: %w", err)
	}

	r.activeSessions[agent.SessionID] = process

	log.Info().
		Str("session_id", agent.SessionID).
		Str("user_id", agent.UserID).
		Msg("Started single Zed session")

	return nil
}

// createZedProcess creates and starts a Zed process
func (r *ZedAgentRunner) createZedProcess(agent *types.ZedAgent, instance *ZedInstance) (*ZedProcess, error) {
	// Allocate display and RDP port
	displayNum := r.nextDisplayNum
	r.nextDisplayNum++
	rdpPort := r.nextRDPPort
	r.nextRDPPort++

	// Setup workspace directory
	workDir := agent.WorkDir
	if workDir == "" {
		if instance != nil {
			workDir = filepath.Join(r.workspaceBaseDir, instance.InstanceID)
		} else {
			workDir = filepath.Join(r.workspaceBaseDir, agent.SessionID)
		}
	}

	// Create workspace directory
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Setup environment variables
	env := os.Environ()
	env = append(env, fmt.Sprintf("DISPLAY=:%d", displayNum))
	env = append(env, "HOME=/home/zed")
	env = append(env, "USER=zed")

	// Add custom environment variables from agent
	env = append(env, agent.Env...)

	// Handle git repository cloning if specified
	gitRepoURL := ""
	helixAPIKey := ""
	for _, envVar := range agent.Env {
		if strings.HasPrefix(envVar, "GIT_REPO_URL=") {
			gitRepoURL = strings.TrimPrefix(envVar, "GIT_REPO_URL=")
		}
		if strings.HasPrefix(envVar, "HELIX_API_KEY=") {
			helixAPIKey = strings.TrimPrefix(envVar, "HELIX_API_KEY=")
		}
	}

	// Clone git repository if specified
	if gitRepoURL != "" {
		log.Info().
			Str("git_repo_url", gitRepoURL).
			Str("work_dir", workDir).
			Msg("Cloning git repository for Zed agent")

		if err := r.cloneGitRepository(gitRepoURL, helixAPIKey, workDir); err != nil {
			log.Error().Err(err).Msg("Failed to clone git repository")
			// Continue anyway - agent can work without repository
		}
	}

	// Setup Zed command
	cmd := exec.Command(r.zedBinaryPath, workDir)
	cmd.Env = env
	cmd.Dir = workDir

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Zed process: %w", err)
	}

	process := &ZedProcess{
		SessionID:    agent.SessionID,
		InstanceID:   agent.InstanceID,
		ThreadID:     agent.ThreadID,
		UserID:       agent.UserID,
		ProjectPath:  agent.ProjectPath,
		WorkDir:      workDir,
		Environment:  env,
		Process:      cmd,
		DisplayNum:   displayNum,
		RDPPort:      rdpPort,
		StartTime:    time.Now(),
		LastActivity: time.Now(),
		Status:       "running",
	}

	// Monitor process in background
	go r.monitorProcess(process)

	log.Info().
		Str("session_id", agent.SessionID).
		Str("work_dir", workDir).
		Int("display_num", displayNum).
		Int("rdp_port", rdpPort).
		Int("pid", cmd.Process.Pid).
		Msg("Started Zed process")

	return process, nil
}

// monitorProcess monitors a Zed process and handles cleanup
func (r *ZedAgentRunner) monitorProcess(process *ZedProcess) {
	// Wait for process to complete
	err := process.Process.Wait()

	log.Info().
		Str("session_id", process.SessionID).
		Err(err).
		Msg("Zed process completed")

	// Remove from active sessions
	r.mutex.Lock()
	delete(r.activeSessions, process.SessionID)

	// If part of instance, remove from instance threads
	if process.InstanceID != "" {
		if instance, exists := r.activeInstances[process.InstanceID]; exists {
			delete(instance.Threads, process.ThreadID)
			if len(instance.Threads) == 0 {
				delete(r.activeInstances, process.InstanceID)
			}
		}
	}
	r.mutex.Unlock()
}

// cleanup stops all active Zed processes
func (r *ZedAgentRunner) cleanup() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	log.Info().Msg("Cleaning up active Zed processes")

	// Stop all single sessions
	for sessionID, process := range r.activeSessions {
		log.Info().Str("session_id", sessionID).Msg("Stopping Zed session")
		if process.Process != nil && process.Process.Process != nil {
			process.Process.Process.Kill()
		}
	}

	// Stop all instance threads
	for instanceID, instance := range r.activeInstances {
		log.Info().Str("instance_id", instanceID).Msg("Stopping Zed instance")
		for threadID, process := range instance.Threads {
			log.Info().Str("thread_id", threadID).Msg("Stopping Zed thread")
			if process.Process != nil && process.Process.Process != nil {
				process.Process.Process.Kill()
			}
		}
	}
}

// Helper functions
func getZedBinaryPath() string {
	if path := os.Getenv("ZED_BINARY"); path != "" {
		return path
	}
	return "/usr/local/bin/zed"
}

func getWorkspaceBaseDir() string {
	if dir := os.Getenv("WORKSPACE_DIR"); dir != "" {
		return dir
	}
	return "/tmp/zed-workspaces"
}

// cloneGitRepository clones a git repository with authentication
func (r *ZedAgentRunner) cloneGitRepository(gitRepoURL, apiKey, workDir string) error {
	// Create workspace directory
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Build authenticated clone URL
	cloneURL := gitRepoURL
	if apiKey != "" {
		// Add API key authentication to URL
		cloneURL = strings.Replace(gitRepoURL, "://", fmt.Sprintf("://api:%s@", apiKey), 1)
	}

	log.Info().
		Str("repo_url", gitRepoURL).
		Str("work_dir", workDir).
		Msg("Cloning git repository")

	// Execute git clone
	cmd := exec.Command("git", "clone", cloneURL, workDir)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // Disable interactive prompts
		"GIT_ASKPASS=true",      // Use URL-based authentication
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().
			Err(err).
			Str("output", string(output)).
			Str("clone_url", gitRepoURL).
			Msg("Git clone failed")
		return fmt.Errorf("git clone failed: %w", err)
	}

	log.Info().
		Str("work_dir", workDir).
		Msg("Git repository cloned successfully")

	// Set proper ownership
	if err := r.setWorkspaceOwnership(workDir); err != nil {
		log.Warn().Err(err).Msg("Failed to set workspace ownership")
	}

	return nil
}

// setWorkspaceOwnership ensures the workspace is owned by the zed user
func (r *ZedAgentRunner) setWorkspaceOwnership(workDir string) error {
	cmd := exec.Command("chown", "-R", "zed:zed", workDir)
	return cmd.Run()
}

// startWebSocketServer starts the WebSocket server for Zed communication
func (r *ZedAgentRunner) startWebSocketServer(ctx context.Context) error {
	// Use existing external WebSocket thread sync on port 3030
	// The Zed binary will connect to this for coordination

	log.Info().
		Int("port", 3030).
		Msg("WebSocket server for Zed sync running on port 3030")

	return nil
}

func NewZedAgentRunnerCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:     "zed-agent-runner",
		Short:   "Start the Helix Zed agent runner.",
		Long:    "Start the Helix Zed agent runner that processes Zed editor tasks. Replaces gptscript runner entirely.",
		Example: "helix zed-agent-runner",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := zedAgentRunner(cmd)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					log.Info().Msg("Zed agent runner stopped")
					return nil
				}
				return err
			}
			return nil
		},
	}

	return runCmd
}

func zedAgentRunner(_ *cobra.Command) error {
	cfg, err := config.LoadExternalAgentRunnerConfig()
	if err != nil {
		log.Error().Err(err).Msg("failed to load Zed agent runner config")
		return err
	}

	log.Info().
		Str("api_host", cfg.APIHost).
		Str("runner_id", cfg.RunnerID).
		Int("concurrency", cfg.Concurrency).
		Int("max_tasks", cfg.MaxTasks).
		Msg("starting Zed agent runner")

	// Initialize Zed agent runner
	runner := NewZedAgentRunner(&cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Info().
			Str("signal", sig.String()).
			Msg("received shutdown signal, stopping Zed agent runner")
		cancel()
	}()

	// Start the runner
	err = runner.Run(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// Graceful shutdown
			log.Info().Msg("Zed agent runner gracefully shut down")
			return nil
		} else {
			log.Error().Err(err).Msg("Zed agent runner stopped with error")
			return err
		}
	}

	log.Info().Msg("Zed agent runner stopped successfully")
	return nil
}
