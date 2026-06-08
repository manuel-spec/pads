package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <id>",
	Short: "Resume an interrupted download",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("resume not yet implemented: %s", args[0])
	},
}

func init() {
	rootCmd.AddCommand(resumeCmd)
}
