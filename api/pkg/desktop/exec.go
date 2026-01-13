package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ExecRequest represents a command execution request
type ExecRequest struct {
	Command    []string          `json:"command"`    // Command and arguments
	Background bool              `json:"background"` // Run in background (don't wait for completion)
	Timeout    int               `json:"timeout"`    // Timeout in seconds (default: 30, ignored if background)
	Env        map[string]string `json:"env"`        // Environment variables to set
}

// ExecResponse represents a command execution response
type ExecResponse struct {
	Success  bool   `json:"success"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code"`
	PID      int    `json:"pid,omitempty"` // Only set for background commands
}

// handleExec executes a command in the container
// This is used for benchmarking (e.g., starting vkcube) and debugging
//
// POST /exec
// Request body: {"command": ["vkcube"], "background": true}
// Response: {"success": true, "pid": 12345}
func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %s", err), http.StatusBadRequest)
		return
	}

	if len(req.Command) == 0 {
		http.Error(w, "command is required", http.StatusBadRequest)
		return
	}

	// Security: Only allow specific commands for safety
	allowedCommands := map[string]bool{
		"vkcube":       true,
		"glxgears":     true,
		"pkill":        true,
		"killall":      true,
		"ls":           true,
		"echo":         true,
		"weston-simple-egl": true,
	}

	cmdName := req.Command[0]
	if !allowedCommands[cmdName] {
		s.logger.Warn("exec: blocked disallowed command", "command", cmdName)
		http.Error(w, fmt.Sprintf("command not allowed: %s", cmdName), http.StatusForbidden)
		return
	}

	s.logger.Info("exec: executing command", "command", req.Command, "background", req.Background)

	var resp ExecResponse

	// Build environment with requested env vars
	var cmdEnv []string
	if len(req.Env) > 0 {
		// Start with current environment and add/override requested vars
		cmdEnv = os.Environ()
		for k, v := range req.Env {
			cmdEnv = append(cmdEnv, fmt.Sprintf("%s=%s", k, v))
		}
	}

	if req.Background {
		// Run in background - start and return PID
		cmd := exec.Command(req.Command[0], req.Command[1:]...)
		if cmdEnv != nil {
			cmd.Env = cmdEnv
		}
		if err := cmd.Start(); err != nil {
			resp.Success = false
			resp.Error = err.Error()
			resp.ExitCode = -1
		} else {
			resp.Success = true
			resp.PID = cmd.Process.Pid
			s.logger.Info("exec: started background command", "pid", resp.PID, "command", req.Command)
		}
	} else {
		// Run and wait for completion
		timeout := req.Timeout
		if timeout <= 0 {
			timeout = 30
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, req.Command[0], req.Command[1:]...)
		if cmdEnv != nil {
			cmd.Env = cmdEnv
		}
		output, err := cmd.CombinedOutput()

		resp.Output = strings.TrimSpace(string(output))
		if err != nil {
			resp.Success = false
			resp.Error = err.Error()
			if exitError, ok := err.(*exec.ExitError); ok {
				resp.ExitCode = exitError.ExitCode()
			} else {
				resp.ExitCode = -1
			}
		} else {
			resp.Success = true
			resp.ExitCode = 0
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
