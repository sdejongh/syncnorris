package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/sdejongh/syncnorris/pkg/config"
)

// NewConfigCommand creates the config command
func NewConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  `View or modify syncnorris configuration.`,
	}

	cmd.AddCommand(newConfigShowCommand())
	cmd.AddCommand(newConfigInitCommand())

	return cmd
}

func newConfigShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadDefault()
			if err != nil {
				return err
			}

			fmt.Printf("Sync Mode: %s\n", cfg.Sync.Mode)
			fmt.Printf("Comparison: %s\n", cfg.Sync.Comparison)
			fmt.Printf("Conflict Resolution: %s\n", cfg.Sync.ConflictResolution)
			fmt.Printf("Max Workers: %d\n", cfg.Performance.MaxWorkers)
			fmt.Printf("Output Format: %s\n", cfg.Output.Format)
			fmt.Printf("Log Format: %s\n", cfg.Logging.Format)
			fmt.Printf("Log Level: %s\n", cfg.Logging.Level)

			return nil
		},
	}
}

func newConfigInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create default configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.DefaultConfigPath()
			if err != nil {
				return err
			}

			cfg := config.Default()
			if err := config.SaveToFile(cfg, path); err != nil {
				return err
			}

			fmt.Printf("Configuration file created at: %s\n", path)
			return nil
		},
	}
}
