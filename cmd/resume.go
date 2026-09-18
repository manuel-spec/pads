package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"pads/internal/app"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <id>",
	Short: "Resume an interrupted download",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		// With a daemon up the download belongs in the daemon, so that a later
		// pause or status call can reach it.
		if client := liveDaemon(); client != nil {
			if err := client.Resume(cmd.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "resumed download %s in daemon\n", id)
			return nil
		}

		application, err := app.New()
		if err != nil {
			return err
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		return application.Resume(ctx, id)
	},
}

func init() {
	rootCmd.AddCommand(resumeCmd)
}
