package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pauseCmd = &cobra.Command{
	Use:   "pause <id>",
	Short: "Pause an active download",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("pause not yet implemented: %s", args[0])
	},
}

func init() {
	rootCmd.AddCommand(pauseCmd)
}
