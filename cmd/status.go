package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"pads/internal/app"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show active download status",
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}
		return application.Status(os.Stdout)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
