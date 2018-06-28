package cmd

import (
	"fmt"

	"nanny/pkg/version"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print Nanny version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version.VersionString)
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
