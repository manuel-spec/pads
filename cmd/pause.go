package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pads/internal/app"
)

var pauseCmd = &cobra.Command{
	Use:   "pause <id>",
	Short: "Pause an active download",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}
		if err := application.Pause(args[0]); err != nil {
			return err
		}
		fmt.Printf("paused download %s\n", args[0])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pauseCmd)
}
