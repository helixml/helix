package helix

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

func GetHelixVersion() string {
	helixVersion := "<unknown>"
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, kv := range info.Settings {
			if kv.Value == "" {
				continue
			}
			switch kv.Key {
			case "vcs.revision":
				helixVersion = kv.Value
			}
		}
	}
	return helixVersion
}

func newVersionCommand() *cobra.Command {
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(GetHelixVersion())
		},
	}
	return versionCmd
}
