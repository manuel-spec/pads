package cmd

import (
	"context"
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
		application, err := app.New()
		if err != nil {
			return err
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		return application.Resume(ctx, args[0])
	},
}

func init() {
	rootCmd.AddCommand(resumeCmd)
}
