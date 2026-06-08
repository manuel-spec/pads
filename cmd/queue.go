package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"pads/internal/app"
)

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage the download queue",
}

var queueOutput string

var queueAddCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Add a URL to the download queue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}
		entry, err := application.QueueAdd(args[0], queueOutput)
		if err != nil {
			return err
		}
		fmt.Printf("queued %s as %s\n", args[0], entry.ID)
		return nil
	},
}

var queueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List queued downloads",
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}
		entries, err := application.QueueList()
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	},
}

var queueStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start processing the download queue",
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		return application.QueueStart(ctx)
	},
}

var queueClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the download queue",
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}
		return application.QueueClear()
	},
}

var queueRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove a queued download by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}
		return application.QueueRemove(args[0])
	},
}

func init() {
	queueAddCmd.Flags().StringVarP(&queueOutput, "output", "o", "", "output file path")
	queueCmd.AddCommand(queueAddCmd)
	queueCmd.AddCommand(queueListCmd)
	queueCmd.AddCommand(queueStartCmd)
	queueCmd.AddCommand(queueClearCmd)
	queueCmd.AddCommand(queueRemoveCmd)
	rootCmd.AddCommand(queueCmd)
}
