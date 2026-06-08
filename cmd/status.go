package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show active download status",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("status not yet implemented")
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
