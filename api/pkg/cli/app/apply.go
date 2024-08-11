package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

type applyOptions struct {
	filename string
}

// applyCmd represents the apply command
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Create or update an application",
	Long:  `Create or update an application.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filename, err := cmd.Flags().GetString("filename")
		if err != nil {
			return err
		}

		if filename == "" {
			return fmt.Errorf("filename is required")
		}

		fmt.Println("apply called with filename: ", filename)
		return nil
	},
}

func NewApplyCmd() *cobra.Command {
	return applyCmd
}

func init() {
	rootCmd.AddCommand(NewApplyCmd())

	applyCmd.Flags().StringP("filename", "f", "", "Filename to apply")
}
