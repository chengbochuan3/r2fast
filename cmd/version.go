package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is overridable at build time with -ldflags "-X ...cmd.Version=...".
var Version = "0.1.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("r2fast", Version)
	},
}
