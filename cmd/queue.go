package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage the download queue",
}

var queueAddCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Add a URL to the download queue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("queue add not yet implemented: %s", args[0])
	},
}

var queueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List queued downloads",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("queue list not yet implemented")
	},
}

var queueStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start processing the download queue",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("queue start not yet implemented")
	},
}

var queueClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the download queue",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("queue clear not yet implemented")
	},
}

var queueRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove a queued download by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("queue remove not yet implemented: %s", args[0])
	},
}

func init() {
	queueCmd.AddCommand(queueAddCmd)
	queueCmd.AddCommand(queueListCmd)
	queueCmd.AddCommand(queueStartCmd)
	queueCmd.AddCommand(queueClearCmd)
	queueCmd.AddCommand(queueRemoveCmd)
	rootCmd.AddCommand(queueCmd)
}
