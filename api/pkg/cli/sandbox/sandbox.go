// Package sandbox is the helix CLI surface for the user-facing Sandboxes API.
//
// Auth comes from $HELIX_API_KEY; the API URL from $HELIX_URL (default
// http://localhost:8080). The org is resolved either via --org (name or id) or
// $HELIX_ORG, falling back to the user's first org.
package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sandbox",
		Short:   "Manage user-facing sandboxes (Sandboxes API)",
		Aliases: []string{"sandboxes", "sbx"},
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newRuntimesCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newExecCmd())
	cmd.AddCommand(newCommandsCmd())
	cmd.AddCommand(newLogsCmd())
	cmd.AddCommand(newKillCmd())
	cmd.AddCommand(newTerminalCmd())
	cmd.AddCommand(newWaitCmd())
	cmd.AddCommand(newReadCmd())
	cmd.AddCommand(newWriteCmd())
	cmd.AddCommand(newLsCmd())
	return cmd
}

func newRuntimesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "runtimes",
		Short: "List sandbox runtimes configured on the server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			runtimes, err := c.ListSandboxRuntimes(ctx)
			if err != nil {
				return err
			}
			for _, r := range runtimes {
				fmt.Println(r)
			}
			return nil
		},
	}
}

func newClient() (*client.HelixClient, error) {
	url := os.Getenv("HELIX_URL")
	if url == "" {
		url = "http://localhost:8080"
	}
	apiKey := os.Getenv("HELIX_API_KEY")
	if apiKey == "" {
		return nil, errors.New("HELIX_API_KEY is not set")
	}
	return client.NewClient(url, apiKey, false)
}

// resolveOrg picks the org id. Order:
//  1. --org flag (id or name)
//  2. $HELIX_ORG (id or name)
//  3. user's first org
func resolveOrg(ctx context.Context, c *client.HelixClient, orgFlag string) (string, error) {
	pick := orgFlag
	if pick == "" {
		pick = os.Getenv("HELIX_ORG")
	}
	if pick != "" && strings.HasPrefix(pick, system.OrganizationPrefix) {
		return pick, nil
	}
	orgs, err := c.ListOrganizations(ctx)
	if err != nil {
		return "", err
	}
	if pick == "" {
		if len(orgs) == 0 {
			return "", errors.New("no organizations available; create one in the UI first")
		}
		return orgs[0].ID, nil
	}
	for _, o := range orgs {
		if o.Name == pick || o.ID == pick {
			return o.ID, nil
		}
	}
	return "", fmt.Errorf("organization %q not found", pick)
}

// ---------- list ----------

func newListCmd() *cobra.Command {
	var (
		orgFlag     string
		projectFlag string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sandboxes in the organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			var filter *client.SandboxListFilter
			if projectFlag != "" {
				filter = &client.SandboxListFilter{ProjectID: projectFlag}
			}
			resp, err := c.ListSandboxes(ctx, orgID, filter)
			if err != nil {
				return err
			}
			if len(resp.Sandboxes) == 0 {
				fmt.Println("No sandboxes.")
				return nil
			}
			fmt.Printf("%-32s  %-16s  %-30s  %-12s  %-10s  %-20s  %s\n", "ID", "RUNTIME", "IMAGE", "STATUS", "AGE", "PROJECT", "NAME")
			for _, sb := range resp.Sandboxes {
				age := time.Since(sb.CreatedAt).Round(time.Second)
				project := sb.ProjectID
				if project == "" {
					project = "-"
				}
				image := sb.Image
				if image == "" {
					image = "-"
				}
				fmt.Printf("%-32s  %-16s  %-30s  %-12s  %-10s  %-20s  %s\n", sb.ID, sb.Runtime, image, sb.Status, age, project, sb.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (default: $HELIX_ORG or first org)")
	cmd.Flags().StringVar(&projectFlag, "project", "", "Filter to a single project ID")
	return cmd
}

// ---------- create ----------

func newCreateCmd() *cobra.Command {
	var (
		orgFlag     string
		projectFlag string
		name        string
		runtime     string
		image       string
		ttl         int
		wait        bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a sandbox",
		Long: `Create a new sandbox.

By default the server's configured default runtime is used (typically
headless-ubuntu). Use --runtime to pick another configured runtime — see the
list with 'helix sandbox runtimes'. Use --image to pin a custom Docker image
(only honoured when the server has HELIX_SANDBOX_ALLOW_CUSTOM_IMAGE=true).
--runtime and --image are mutually exclusive.

Pass --project to associate the sandbox with a project (optional).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			sb, err := c.CreateSandbox(ctx, orgID, &types.CreateSandboxRequest{
				Name:           name,
				Runtime:        types.SandboxRuntime(runtime),
				Image:          image,
				TimeoutSeconds: ttl,
				ProjectID:      projectFlag,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Created %s (runtime=%s, image=%s, status=%s)\n", sb.ID, sb.Runtime, sb.Image, sb.Status)
			if wait {
				return waitForRunning(ctx, c, orgID, sb.ID, 90*time.Second)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name")
	cmd.Flags().StringVar(&projectFlag, "project", "", "Optional project ID to associate the sandbox with")
	cmd.Flags().StringVar(&name, "name", "", "Display name")
	cmd.Flags().StringVar(&runtime, "runtime", "", "Configured runtime name (e.g. headless-ubuntu, node22). Empty = server default.")
	cmd.Flags().StringVar(&image, "image", "", "Custom Docker image (requires HELIX_SANDBOX_ALLOW_CUSTOM_IMAGE=true on the server)")
	cmd.Flags().IntVar(&ttl, "ttl", 600, "Lifetime in seconds")
	cmd.Flags().BoolVar(&wait, "wait", true, "Wait for status=running")
	return cmd
}

// ---------- get ----------

func newGetCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "get <sandbox-id>",
		Short: "Show a sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			sb, err := c.GetSandbox(ctx, orgID, args[0])
			if err != nil {
				return err
			}
			out, _ := json.MarshalIndent(sb, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name")
	return cmd
}

// ---------- delete ----------

func newDeleteCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:     "delete <sandbox-id>",
		Aliases: []string{"rm"},
		Short:   "Delete a sandbox",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			if err := c.DeleteSandbox(ctx, orgID, args[0]); err != nil {
				return err
			}
			fmt.Printf("Deleted %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name")
	return cmd
}

// ---------- wait ----------

func newWaitCmd() *cobra.Command {
	var (
		orgFlag string
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "wait <sandbox-id>",
		Short: "Block until the sandbox is running",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			return waitForRunning(ctx, c, orgID, args[0], timeout)
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name")
	cmd.Flags().DurationVar(&timeout, "timeout", 90*time.Second, "Max time to wait")
	return cmd
}

func waitForRunning(ctx context.Context, c *client.HelixClient, orgID, sbID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		sb, err := c.GetSandbox(ctx, orgID, sbID)
		if err != nil {
			return err
		}
		switch sb.Status {
		case types.SandboxStatusRunning:
			fmt.Printf("Sandbox %s is running.\n", sb.ID)
			return nil
		case types.SandboxStatusFailed:
			return fmt.Errorf("sandbox failed: %s", sb.StatusMessage)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for sandbox %s (last status=%s)", sb.ID, sb.Status)
		}
		time.Sleep(time.Second)
	}
}

// ---------- exec ----------

func newExecCmd() *cobra.Command {
	var (
		orgFlag  string
		cwd      string
		ttl      int
		detached bool
	)
	cmd := &cobra.Command{
		Use:   "exec <sandbox-id> -- <cmd> [args...]",
		Short: "Run a command in the sandbox",
		Long: `Run a command inside the sandbox.

By default this is synchronous: the command runs to completion and stdout/stderr
are printed. Use --detached to fire-and-forget — the command keeps running and
the new command id is printed; fetch its output later with:

  helix sandbox logs <sandbox-id> <cmd-id>`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			sbID := args[0]
			cmdArgs := args[1:]
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			resp, err := c.RunSandboxCommand(ctx, orgID, sbID, &types.RunSandboxCommandRequest{
				Cmd:            cmdArgs[0],
				Args:           cmdArgs[1:],
				Cwd:            cwd,
				TimeoutSeconds: ttl,
				Detached:       detached,
			})
			if err != nil {
				return err
			}
			if detached {
				fmt.Println(resp.ID)
				return nil
			}
			if resp.Stdout != "" {
				os.Stdout.WriteString(resp.Stdout)
			}
			if resp.Stderr != "" {
				os.Stderr.WriteString(resp.Stderr)
			}
			if resp.ExitCode != nil && *resp.ExitCode != 0 {
				os.Exit(*resp.ExitCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Working directory inside the sandbox")
	cmd.Flags().IntVar(&ttl, "timeout", 60, "Per-command timeout in seconds (synchronous mode)")
	cmd.Flags().BoolVar(&detached, "detached", false, "Run in background; print the command id immediately")
	return cmd
}

// ---------- commands / logs / kill ----------

func newCommandsCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "commands <sandbox-id>",
		Short: "List commands tracked for a sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			cmds, err := c.ListSandboxCommands(ctx, orgID, args[0])
			if err != nil {
				return err
			}
			if len(cmds) == 0 {
				fmt.Println("No commands.")
				return nil
			}
			fmt.Printf("%-32s  %-10s  %-5s  %s\n", "ID", "STATUS", "EXIT", "CMD")
			for _, cm := range cmds {
				exit := "-"
				if cm.ExitCode != nil {
					exit = fmt.Sprintf("%d", *cm.ExitCode)
				}
				cmdStr := cm.Cmd
				if len(cm.Args) > 0 {
					cmdStr += " " + strings.Join(cm.Args, " ")
				}
				fmt.Printf("%-32s  %-10s  %-5s  %s\n", cm.ID, cm.Status, exit, cmdStr)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name")
	return cmd
}

func newLogsCmd() *cobra.Command {
	var (
		orgFlag string
		stream  string
		follow  bool
	)
	cmd := &cobra.Command{
		Use:   "logs <sandbox-id> <cmd-id>",
		Short: "Stream the SSE log feed for a command run inside the sandbox",
		Long: `Stream stdout and/or stderr captured for a command. Pass --follow to keep
the connection open until the command exits; otherwise the existing buffer is
returned and the stream closes.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			body, err := c.StreamSandboxCommandLogs(ctx, orgID, args[0], args[1], stream, follow)
			if err != nil {
				return err
			}
			defer body.Close()
			_, err = io.Copy(os.Stdout, body)
			return err
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name")
	cmd.Flags().StringVar(&stream, "stream", "", "stdout | stderr | both (default both)")
	cmd.Flags().BoolVar(&follow, "follow", false, "Keep the stream open until the command exits")
	return cmd
}

func newKillCmd() *cobra.Command {
	var (
		orgFlag string
		signal  string
	)
	cmd := &cobra.Command{
		Use:   "kill <sandbox-id> <cmd-id>",
		Short: "Send a signal to a running command",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			if err := c.KillSandboxCommand(ctx, orgID, args[0], args[1], signal); err != nil {
				return err
			}
			fmt.Printf("Sent %s to %s\n", signalOrDefault(signal), args[1])
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name")
	cmd.Flags().StringVar(&signal, "signal", "", "Signal name (default TERM)")
	return cmd
}

func signalOrDefault(s string) string {
	if s == "" {
		return "TERM"
	}
	return s
}

// ---------- files: ls / read / write ----------

func newLsCmd() *cobra.Command {
	var (
		orgFlag string
		dir     string
	)
	cmd := &cobra.Command{
		Use:   "ls <sandbox-id>",
		Short: "List a directory inside the sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			resp, err := c.ListSandboxFiles(ctx, orgID, args[0], dir)
			if err != nil {
				return err
			}
			fmt.Printf("# %s\n", resp.Path)
			for _, e := range resp.Entries {
				kind := "f"
				if e.IsDir {
					kind = "d"
				}
				fmt.Printf("%s  %-10s  %10d  %s\n", kind, e.Mode, e.Size, e.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name")
	cmd.Flags().StringVar(&dir, "path", "/root", "Directory to list")
	return cmd
}

func newReadCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "read <sandbox-id> <path>",
		Short: "Read a file from the sandbox to stdout",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			data, err := c.ReadSandboxFile(ctx, orgID, args[0], args[1])
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(data)
			return err
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name")
	return cmd
}

func newWriteCmd() *cobra.Command {
	var (
		orgFlag string
		mode    int
	)
	cmd := &cobra.Command{
		Use:   "write <sandbox-id> <path>",
		Short: "Write stdin to a file in the sandbox",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			if err := c.WriteSandboxFile(ctx, orgID, args[0], args[1], data, mode); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Wrote %d bytes to %s\n", len(data), args[1])
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name")
	cmd.Flags().IntVar(&mode, "mode", 0, "Octal mode (e.g. 644 for 0644). 0 keeps server default.")
	return cmd
}

// ---------- terminal ----------

func newTerminalCmd() *cobra.Command {
	var (
		orgFlag string
		shell   string
	)
	cmd := &cobra.Command{
		Use:     "terminal <sandbox-id>",
		Aliases: []string{"shell", "tty"},
		Short:   "Attach an interactive terminal to the sandbox",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := newClient()
			if err != nil {
				return err
			}
			orgID, err := resolveOrg(ctx, c, orgFlag)
			if err != nil {
				return err
			}
			conn, err := c.OpenSandboxTerminal(ctx, orgID, args[0], shell)
			if err != nil {
				return err
			}
			defer conn.Close()
			return runTerminal(conn)
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name")
	cmd.Flags().StringVar(&shell, "shell", "", "Override shell (default /bin/bash, falls back to /bin/sh)")
	return cmd
}

func runTerminal(conn *websocket.Conn) error {
	stdinFd := int(os.Stdin.Fd())
	isTTY := term.IsTerminal(stdinFd)
	var oldState *term.State
	if isTTY {
		var err error
		oldState, err = term.MakeRaw(stdinFd)
		if err != nil {
			return fmt.Errorf("make raw: %w", err)
		}
		defer term.Restore(stdinFd, oldState)
	}

	cols, rows := 80, 24
	if isTTY {
		if w, h, err := term.GetSize(stdinFd); err == nil {
			cols, rows = w, h
		}
	}
	if err := conn.WriteMessage(websocket.TextMessage, mustMarshal(map[string]any{
		"type": "resize", "cols": cols, "rows": rows,
	})); err != nil {
		return fmt.Errorf("send resize: %w", err)
	}

	if isTTY {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGWINCH)
		go func() {
			for range ch {
				if w, h, err := term.GetSize(stdinFd); err == nil {
					_ = conn.WriteMessage(websocket.TextMessage, mustMarshal(map[string]any{
						"type": "resize", "cols": w, "rows": h,
					}))
				}
			}
		}()
	}

	stdinDone := make(chan struct{})
	go func() {
		defer close(stdinDone)
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, append([]byte{}, buf[:n]...)); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				fmt.Fprintf(os.Stderr, "\r\n[disconnected: %v]\r\n", err)
			}
			return nil
		}
		switch mt {
		case websocket.BinaryMessage:
			os.Stdout.Write(data)
		case websocket.TextMessage:
			var ctrl struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(data, &ctrl); err == nil && ctrl.Type == "error" {
				fmt.Fprintf(os.Stderr, "\r\n[server error: %s]\r\n", ctrl.Message)
				return errors.New(ctrl.Message)
			}
			os.Stdout.Write(data)
		}
	}
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
