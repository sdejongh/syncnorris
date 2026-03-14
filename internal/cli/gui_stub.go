//go:build nogui

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewGUICommand returns a stub command when GUI support is not compiled in
func NewGUICommand() *cobra.Command {
	return &cobra.Command{
		Use:   "gui",
		Short: "Launch the graphical user interface (not available in this build)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("GUI not available: this binary was built without GUI support (use 'make build' for a GUI-enabled build)")
		},
	}
}
