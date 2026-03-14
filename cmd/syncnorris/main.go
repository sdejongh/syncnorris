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
	// Set build info in cli package
	cli.Version = version
	cli.Commit = commit
	cli.BuildDate = date

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
network shares, and remote storage with multiple comparison methods.

Running without a subcommand launches the graphical user interface.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Launch GUI only when invoked with no arguments at all.
			// Any flag (--help, --version, --verbose, etc.) falls through to help.
			if len(os.Args) == 1 {
				return cli.DefaultRunE(cmd, args)
			}
			return cmd.Help()
		},
	}

	// Add global flags
	cli.AddGlobalFlags(rootCmd)

	// Add commands
	rootCmd.AddCommand(cli.NewSyncCommand())
	rootCmd.AddCommand(cli.NewCompareCommand())
	rootCmd.AddCommand(cli.NewConfigCommand())
	rootCmd.AddCommand(cli.NewVersionCommand())
	rootCmd.AddCommand(cli.NewGUICommand())

	return rootCmd.Execute()
}
