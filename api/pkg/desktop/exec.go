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
		"vkcube":                    true,
		"glxgears":                  true,
		"pkill":                     true,
		"killall":                   true,
		"ls":                        true,
		"echo":                      true,
		"weston-simple-egl":         true,
		"claude":                    true,
		"cat":                       true,
		"test":                      true,
		"npm":                       true, // needed to upgrade claude CLI at login time
		"helix-claude-auth-wrapper": true, // runs claude auth login with stdout capture
		"git":                       true, // scoped further below: only identity writes via gitInvocationAllowed
	}

	cmdName := req.Command[0]
	if !allowedCommands[cmdName] {
		s.logger.Warn("exec: blocked disallowed command", "command", cmdName)
		http.Error(w, fmt.Sprintf("command not allowed: %s", cmdName), http.StatusForbidden)
		return
	}

	// Extra scoping for git: `git` is a dispatcher with ~150 subcommands, some
	// of which can exfiltrate secrets (e.g. via a custom credential.helper) or
	// make destructive filesystem changes. Only allow the identity-writing
	// subset used by the spec-approval flow.
	if cmdName == "git" && !gitInvocationAllowed(req.Command) {
		s.logger.Warn("exec: blocked git invocation", "args", req.Command)
		http.Error(w, "git invocation not allowed (only 'git config --global user.name|user.email <value>')", http.StatusForbidden)
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

// gitInvocationAllowed restricts `git` invocations to the identity-writing
// subset: `git config --global user.name <value>` or
// `git config --global user.email <value>`. Any other form is rejected.
//
// This exists because `git` as a dispatcher is extremely broad: allowing the
// binary unconditionally would permit arbitrary `git clone`, custom
// credential helpers, object fetches over HTTP, etc. from anyone able to
// reach the exec endpoint.
func gitInvocationAllowed(cmd []string) bool {
	if len(cmd) != 5 {
		return false
	}
	if cmd[0] != "git" || cmd[1] != "config" || cmd[2] != "--global" {
		return false
	}
	if cmd[3] != "user.name" && cmd[3] != "user.email" {
		return false
	}
	// Reject values that look like flags so we can't be tricked into
	// invoking `git config --global user.name --some-other-flag`.
	return !strings.HasPrefix(cmd[4], "-")
}
