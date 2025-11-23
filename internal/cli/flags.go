package cli

import (
	"github.com/spf13/cobra"
)

// GlobalFlags holds global flag values
type GlobalFlags struct {
	ConfigFile string
	Verbose    bool
	Quiet      bool
}

var globalFlags GlobalFlags

// AddGlobalFlags adds global flags to the root command
func AddGlobalFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(
		&globalFlags.ConfigFile,
		"config",
		"",
		"config file (default is $HOME/.config/syncnorris/config.yaml)",
	)
	cmd.PersistentFlags().BoolVarP(
		&globalFlags.Verbose,
		"verbose",
		"v",
		false,
		"verbose output",
	)
	cmd.PersistentFlags().BoolVarP(
		&globalFlags.Quiet,
		"quiet",
		"q",
		false,
		"suppress non-error output",
	)
}

// GetGlobalFlags returns the global flags
func GetGlobalFlags() *GlobalFlags {
	return &globalFlags
}
