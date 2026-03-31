package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "enclave",
	Short: "Secure on-prem encrypted chat",
	Long:  "Enclave is a fully on-prem, end-to-end encrypted chat application for developers.",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(inviteCmd)
	rootCmd.AddCommand(exportKeyCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(versionCmd)
}
