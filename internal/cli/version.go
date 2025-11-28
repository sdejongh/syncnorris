package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Build information - set via ldflags
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// NewVersionCommand creates the version command
func NewVersionCommand() *cobra.Command {
	var short bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  `Display detailed version information including build date, commit hash, and Go version.`,
		Run: func(cmd *cobra.Command, args []string) {
			if short {
				fmt.Println(Version)
				return
			}

			fmt.Printf("syncnorris %s\n", Version)
			fmt.Printf("  Commit:     %s\n", Commit)
			fmt.Printf("  Built:      %s\n", BuildDate)
			fmt.Printf("  Go version: %s\n", runtime.Version())
			fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}

	cmd.Flags().BoolVarP(&short, "short", "s", false, "print only the version number")

	return cmd
}
