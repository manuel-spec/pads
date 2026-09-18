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
		id := args[0]

		// A download runs inside whichever process owns it. When a daemon is
		// up it owns them all, and pausing locally would only flip a flag in
		// the state file while the transfer kept running.
		if client := liveDaemon(); client != nil {
			if err := client.Pause(cmd.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "paused download %s\n", id)
			return nil
		}

		application, err := app.New()
		if err != nil {
			return err
		}
		if err := application.Pause(id); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "paused download %s\n", id)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pauseCmd)
}
