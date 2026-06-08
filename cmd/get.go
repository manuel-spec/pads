package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"pads/internal/app"
)

var getOutput string

var getCmd = &cobra.Command{
	Use:   "get <url>",
	Short: "Download a file from a URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		return application.Download(ctx, args[0], getOutput)
	},
}

func init() {
	getCmd.Flags().StringVarP(&getOutput, "output", "o", "", "output file path")
	rootCmd.AddCommand(getCmd)
}
