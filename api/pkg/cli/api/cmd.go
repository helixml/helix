// Package api implements `helix api` — an authenticated REST escape hatch
// modelled on `gh api` (https://cli.github.com/manual/gh_api). Agents and
// humans can hit any Helix HTTP path without inventing a first-class CLI
// command for every endpoint.
package api

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	orgcli "github.com/helixml/helix/api/pkg/cli/org"
)

// New returns the `helix api` command.
func New() *cobra.Command {
	var (
		method   string
		input    string
		rawField string
		paginate bool // reserved, not implemented
		timeout  int
		jq       string // not implemented — print raw JSON
	)
	_ = paginate
	_ = jq

	cmd := &cobra.Command{
		Use:   "api <path>",
		Short: "Make an authenticated Helix API request (like gh api)",
		Long: `Call any Helix REST endpoint with the CLI's credentials.

Path may be:
  /orgs/unmanned-org/bots
  /api/v1/orgs/unmanned-org/bots
  orgs/unmanned-org/bots

Examples:
  helix api /orgs/unmanned-org/bots
  helix api --method POST /orgs/unmanned-org/bots/chief-of-staff/activate
  helix api -X POST /sessions/chat --input '{"session_id":"ses_…","type":"text","stream":false,"messages":[…]}'
  echo '{"name":"x"}' | helix api -X POST /orgs/unmanned-org/bots --input -

Uses HELIX_URL (default http://localhost:8080) and HELIX_API_KEY.
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if method == "" {
				method = "GET"
			}
			method = strings.ToUpper(method)

			var body []byte
			if input != "" {
				if input == "-" {
					var err error
					body, err = io.ReadAll(os.Stdin)
					if err != nil {
						return err
					}
				} else if strings.HasPrefix(input, "@") {
					var err error
					body, err = os.ReadFile(strings.TrimPrefix(input, "@"))
					if err != nil {
						return err
					}
				} else {
					body = []byte(input)
				}
			}
			if rawField != "" {
				// gh-style -f key=value → JSON object merge (simple).
				parts := strings.SplitN(rawField, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("--raw-field expects key=value")
				}
				// Only set when no full input body.
				if len(body) == 0 {
					body = []byte(fmt.Sprintf(`{%q:%q}`, parts[0], parts[1]))
				}
			}

			// Reuse org http client plumbing.
			// newHTTPClient is in package org — export a small constructor.
			status, raw, err := doAPI(cmd.Context(), method, path, body, time.Duration(timeout)*time.Second)
			if err != nil {
				return err
			}
			if status >= 300 {
				fmt.Fprintf(os.Stderr, "HTTP %d\n", status)
				_, _ = os.Stderr.Write(raw)
				if !strings.HasSuffix(string(raw), "\n") {
					fmt.Fprintln(os.Stderr)
				}
				return fmt.Errorf("request failed with status %d", status)
			}
			return printOut(raw)
		},
	}

	cmd.Flags().StringVarP(&method, "method", "X", "GET", "HTTP method")
	cmd.Flags().StringVar(&input, "input", "", "Request body JSON string, @file, or - for stdin")
	cmd.Flags().StringVarP(&rawField, "raw-field", "f", "", "Add a string field (key=value) when body empty")
	cmd.Flags().IntVar(&timeout, "timeout", 120, "Request timeout seconds")
	return cmd
}

// doAPI and printOut are implemented via org package helpers by thin
// wrappers in api_bridge.go so we don't export org internals widely.
func printOut(raw []byte) error {
	return orgcli.PrintJSONRaw(raw)
}
