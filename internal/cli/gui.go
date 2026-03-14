//go:build !nogui

package cli

import (
	"github.com/spf13/cobra"
	"github.com/sdejongh/syncnorris/internal/gui"
)

// NewGUICommand creates the gui command
func NewGUICommand() *cobra.Command {
	return &cobra.Command{
		Use:   "gui",
		Short: "Launch the graphical user interface",
		Long:  "Launch SyncNorris with a graphical user interface for configuring and running sync operations.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gui.Run()
		},
	}
}

// DefaultRunE launches the GUI when no subcommand is given.
func DefaultRunE(cmd *cobra.Command, args []string) error {
	return gui.Run()
}
