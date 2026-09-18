package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"pads/internal/app"
	"pads/internal/daemon"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show download status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}
		return application.Status(os.Stdout, activeDownloadIDs(cmd, application))
	},
}

// activeDownloadIDs asks the daemon which downloads are running. Without a
// daemon the only downloads this process can see are its own, which for a
// fresh CLI invocation is none.
func activeDownloadIDs(cmd *cobra.Command, application *app.App) []string {
	client := liveDaemon()
	if client == nil {
		return application.ActiveIDs()
	}
	jobs, err := client.Jobs(cmd.Context())
	if err != nil {
		return application.ActiveIDs()
	}
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		if job.Status == daemon.JobRunning {
			ids = append(ids, job.ID)
		}
	}
	return ids
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
