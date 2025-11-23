package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sdejongh/syncnorris/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	rootCmd := &cobra.Command{
		Use:   "syncnorris",
		Short: "Cross-platform file synchronization utility",
		Long: `syncnorris is a cross-platform file synchronization utility built in Go.
It supports one-way and bidirectional synchronization between local folders,
network shares, and remote storage with multiple comparison methods.`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
		SilenceUsage: true,
		SilenceErrors: true,
	}

	// Add global flags
	cli.AddGlobalFlags(rootCmd)

	// Add commands
	rootCmd.AddCommand(cli.NewSyncCommand())
	rootCmd.AddCommand(cli.NewCompareCommand())
	rootCmd.AddCommand(cli.NewConfigCommand())

	return rootCmd.Execute()
}
