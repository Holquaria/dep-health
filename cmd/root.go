// Package cmd wires together the dep-health CLI using cobra.
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "dep-health",
	Short: "Scan repositories for outdated dependencies and score them by risk",
	Long: `dep-health discovers dependency manifests in a repository, resolves the
latest published versions, checks for known CVEs via OSV.dev, and produces
a risk-scored report with upgrade guidance.

Subcommands:
  scan        — analyse a local directory and print a ranked dependency table
  scan-multi  — scan multiple repos and aggregate results with cross-repo scoring
  serve       — start the HTTP server and React dashboard`,
}

// Execute is the main entry point called from main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(scanMultiCmd)
	rootCmd.AddCommand(serveCmd)
}
