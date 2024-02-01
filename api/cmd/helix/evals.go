package helix

import (
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/evals"
)

var BASE_URL := "https://app.tryhelix.ai"
var evalTargets []string

func newEvalsCommand() *cobra.Command {
	var evalsCmd = &cobra.Command{
		Use:   "evals",
		Short: "A CLI tool for evaluating finetuned LLMs",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Run helix evals --help to see available subcommands")
		},
	}

	// Add subcommands
	evalsCmd.AddCommand(newBaseCommand())
	evalsCmd.AddCommand(newInitCommand())
	evalsCmd.AddCommand(newListCommand())
	evalsCmd.AddCommand(newRunCommand())
	evalsCmd.AddCommand(newShowCommand())

	evalsCmd.Flags().StringSliceVar(&evalTargets, "target", []string{},
		"Target(s) to use, defaults to all",
	)

	return evalsCmd
}
func newShowCommand() *cobra.Command {
	var showCmd = &cobra.Command{
		Use:   "show <id>",
		Short: "Show the sessions and scores within an eval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			baseSessions, err := evals.GetEvalSessions(args[0])
			if err != nil {
				return err
			}

			// Create a new table
			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"ID", "Name", "Link", "Score"})

			// Add data to the table
			for _, session := range baseSessions {
				table.Append([]string{session.ID, session.Name, fmt.Sprintf("%s/session/%s", BASE_URL, session.ID), session.Score})
			}

			// Render the table
			table.Render()

			return nil
		},
	}

	return showCmd
}

func newBaseCommand() *cobra.Command {
	var baseCmd = &cobra.Command{
		Use:   "base",
		Short: "List base eval sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			baseSessions, err := evals.GetBaseSessions()
			if err != nil {
				return err
			}

			// Create a new table
			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"ID", "Name", "Link", "Score"})

			// Add data to the table
			for _, session := range baseSessions {
				table.Append([]string{session.ID, session.Name, fmt.Sprintf("%s/session/%s", BASE_URL, session.ID), session.Score})
			}

			// Render the table
			table.Render()

			return nil
		},
	}

	return baseCmd
}

func newInitCommand() *cobra.Command {
	var initCmd = &cobra.Command{
		Use:   "init",
		Short: "Create a new eval session, returning the id",
		RunE: func(cmd *cobra.Command, args []string) error {
			newEvalId, err := evals.Init()
			if err != nil {
				return err
			}
			fmt.Println(newEvalId)
			return nil
		},
	}

	return initCmd
}

func newListCommand() *cobra.Command {
	var listCmd = &cobra.Command{
		Use:   "list",
		Short: "List all evals, including the base, showing the score in a table",
		Run: func(cmd *cobra.Command, args []string) {

			baseSessions, err := evals.ListEvalSummary()
			if err != nil {
				return err
			}

			// Create a new table
			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"ID", "Name", "Score"})

			// Add data to the table
			for _, session := range baseSessions {
				table.Append([]string{session.ID, session.Name, session.Score})
			}

			// Render the table
			table.Render()

			return nil
			// TODO: Implement list command logic
		},
	}

	return listCmd
}

func newRunCommand() *cobra.Command {
	var runCmd = &cobra.Command{
		Use:   "run <id>",
		Short: "Start an eval on the given id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := evals.Run(args[0])
			return er
		},
	}

	return runCmd
}
