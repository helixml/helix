package helix

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/spf13/cobra"
)

func NewVersionCommand() *cobra.Command {
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(*cobra.Command, []string) {
			fmt.Println(data.GetHelixVersion())
		},
	}
	return versionCmd
}
