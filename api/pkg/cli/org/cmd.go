// Package org is the helix CLI surface for helix-org (bots, topics,
// processors, chat). Auth: $HELIX_API_KEY + $HELIX_URL (default
// http://localhost:8080). Org: --org / $HELIX_ORG / first membership.
package org

import (
	"github.com/spf13/cobra"
)

// New returns the `helix org` command tree.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "org",
		Short:   "Helix-org: bots, topics, processors, and chat",
		Aliases: []string{"helix-org", "ho"},
		Long: `Operate on a helix-org organization graph from the CLI.

Examples:
  helix org bots list --org unmanned-org
  helix org bots get chief-of-staff --org unmanned-org
  helix org bots start chief-of-staff --org unmanned-org
  helix org bots chat chief-of-staff --org unmanned-org "What bots exist?"
  helix org topics list --org unmanned-org
  helix org processors list --org unmanned-org

Auth via HELIX_URL + HELIX_API_KEY. For raw REST, use: helix api GET /orgs/{org}/bots
`,
	}
	cmd.AddCommand(newBotsCmd())
	cmd.AddCommand(newTopicsCmd())
	cmd.AddCommand(newProcessorsCmd())
	return cmd
}
