package org

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// `helix org instances`: bot instances — extra sessions sharing a bot's
// identity, each with its own minimal sandbox (one per end customer, or one
// per test). Every new instance copies the bot's CURRENT prompt.
func newInstancesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "instances",
		Aliases: []string{"instance", "inst"},
		Short:   "Create, list, delete and one-shot bot instances",
		Long: `Bot instances share a bot's prompt, harness and model but each gets its own sandbox,
the instance profile's MCP servers/tools, and a restricted key. No activation turn.

Examples:
  helix org instances create b-support --runtime headless-ubuntu      # prints the session id
  helix session send ses_01xxx "hi"                                   # then talk to it
  helix org instances ask b-support "What's the balance on BA-10694?" --tools
  helix org instances list b-support
  helix org instances delete b-support --all --idle
`,
	}
	cmd.AddCommand(newInstancesListCmd(), newInstancesCreateCmd(), newInstancesDeleteCmd(), newInstancesAskCmd())
	return cmd
}

func newInstancesListCmd() *cobra.Command {
	var (
		orgFlag string
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "list <bot-id>",
		Short: "List a bot's instances",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			orgID, err := c.resolveOrg(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			ins, err := c.listInstances(cmd.Context(), orgID, args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(ins)
			}
			fmt.Printf("%-32s %-28s %-16s %-16s %s\n", "SESSION", "NAME", "RUNTIME", "STATUS", "CREATED")
			for _, i := range ins {
				st := i.SandboxStatus
				if st == "" {
					st = "stopped"
				}
				fmt.Printf("%-32s %-28s %-16s %-16s %s\n", i.SessionID, truncate(i.Name, 28), i.SandboxRuntime, st, truncate(i.CreatedAt, 19))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func newInstancesCreateCmd() *cobra.Command {
	var (
		orgFlag, name, runtime, message string
		jsonOut, wait                   bool
	)
	cmd := &cobra.Command{
		Use:   "create <bot-id>",
		Short: "Create an instance and print its session id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			orgID, err := c.resolveOrg(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			inst, err := c.createInstance(cmd.Context(), orgID, args[0], name, runtime, message)
			if err != nil {
				return err
			}
			if wait {
				if err := waitSandbox(cmd.Context(), orgID, inst.SessionID, 3*time.Minute); err != nil {
					return err
				}
			}
			if jsonOut {
				return printJSON(inst)
			}
			fmt.Println(inst.SessionID)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().StringVar(&name, "name", "", "Instance name")
	cmd.Flags().StringVar(&runtime, "runtime", "", "headless-ubuntu | ubuntu-desktop (default: profile, then bot)")
	cmd.Flags().StringVar(&message, "message", "", "Queue this as the first turn")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait until the sandbox is running (fails fast if it fails to start)")
	return cmd
}

func newInstancesDeleteCmd() *cobra.Command {
	var (
		orgFlag   string
		all, idle bool
	)
	cmd := &cobra.Command{
		Use:   "delete <bot-id> [session-id...]",
		Short: "Delete instances (sandbox, workspace and session)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			orgID, err := c.resolveOrg(ctx, orgFlag)
			if err != nil {
				return err
			}
			sids := args[1:]
			if all {
				ins, err := c.listInstances(ctx, orgID, args[0])
				if err != nil {
					return err
				}
				for _, i := range ins {
					if !idle || i.SandboxStatus == "" || i.SandboxStatus == "terminated_idle" || i.SandboxStatus == "stopped" {
						sids = append(sids, i.SessionID)
					}
				}
			}
			if len(sids) == 0 {
				return fmt.Errorf("nothing to delete: pass session ids or --all")
			}
			for _, s := range sids {
				if err := c.deleteInstance(ctx, orgID, args[0], s); err != nil {
					fmt.Fprintf(os.Stderr, "%s: %v\n", s, err)
					continue
				}
				fmt.Println("deleted", s)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().BoolVar(&all, "all", false, "All of the bot's instances")
	cmd.Flags().BoolVar(&idle, "idle", false, "With --all: only stopped / idle-terminated ones")
	return cmd
}

func newInstancesAskCmd() *cobra.Command {
	var (
		orgFlag, runtime string
		attach           []string
		keep, raw, tools bool
		timeout          int
	)
	cmd := &cobra.Command{
		Use:   "ask <bot-id> <message>",
		Short: "One-shot: new instance, one turn, print the reply, delete the instance",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			orgID, err := c.resolveOrg(ctx, orgFlag)
			if err != nil {
				return err
			}
			inst, err := c.createInstance(ctx, orgID, args[0], "helix ask", runtime, "")
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "[instance %s]\n", inst.SessionID)
			defer func() {
				if keep {
					fmt.Fprintf(os.Stderr, "[kept %s]\n", inst.SessionID)
					return
				}
				_ = c.deleteInstance(ctx, orgID, args[0], inst.SessionID)
			}()
			if err := waitSandbox(ctx, orgID, inst.SessionID, 3*time.Minute); err != nil {
				return err
			}
			r, err := c.sendTurn(ctx, inst.SessionID, args[1], attach, "", time.Duration(timeout)*time.Second)
			if err != nil {
				return err
			}
			if raw {
				fmt.Println(r.Raw)
			} else {
				fmt.Println(r.Reply)
			}
			printTurnFooter(r, tools)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().StringVar(&runtime, "runtime", "", "headless-ubuntu | ubuntu-desktop")
	cmd.Flags().StringArrayVarP(&attach, "attach", "a", nil, "File to attach (repeatable)")
	cmd.Flags().BoolVar(&keep, "keep", false, "Keep the instance")
	cmd.Flags().BoolVar(&raw, "raw", false, "Print the whole turn blob")
	cmd.Flags().BoolVar(&tools, "tools", false, "Print a tool-call summary (stderr)")
	cmd.Flags().IntVar(&timeout, "timeout", 900, "Seconds to wait for the turn")
	return cmd
}
