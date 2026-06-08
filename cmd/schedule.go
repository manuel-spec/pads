package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var scheduleAt string

var scheduleCmd = &cobra.Command{
	Use:   "schedule <url>",
	Short: "Schedule a download for a specific time",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if scheduleAt == "" {
			return fmt.Errorf("schedule time is required: use --at HH:MM")
		}
		return fmt.Errorf("schedule not yet implemented: %s at %s", args[0], scheduleAt)
	},
}

func init() {
	scheduleCmd.Flags().StringVar(&scheduleAt, "at", "", "schedule time in HH:MM format")
	_ = scheduleCmd.MarkFlagRequired("at")
	rootCmd.AddCommand(scheduleCmd)
}
