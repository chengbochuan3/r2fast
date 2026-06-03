package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "r2fast [file...]",
	Short: "Fast uploads to Cloudflare R2 with shareable links and auto-expiry",
	Long: `r2fast uploads local files straight to your Cloudflare R2 bucket and prints
a fast download link from your own domain. Files can auto-delete after N days
using R2 lifecycle rules.

First time:  r2fast config init
Then:        r2fast upload bigfile.tar --ttl 7d
Shorthand:   r2fast bigfile.tar`,
	SilenceUsage: true,
	Args:         cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return runUpload(cmd, args)
	},
}

// Execute runs the root command.
func Execute() error { return rootCmd.Execute() }

func init() {
	addUploadFlags(rootCmd)
	rootCmd.AddCommand(uploadCmd, configCmd, lsCmd, rmCmd, lifecycleCmd, versionCmd)
}
