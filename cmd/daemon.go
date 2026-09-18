package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pads/internal/app"
	"pads/internal/daemon"
)

var daemonAddr string

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run and control the PADS background daemon",
}

var daemonRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the daemon in the foreground until interrupted",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemon.Run(cmd.Context(), daemonAddr)
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report whether the daemon is running",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := daemonClient()
		if err != nil {
			fmt.Fprintln(cmd.OutOrStdout(), "daemon: not running")
			return nil
		}
		jobs, err := client.Health()
		if err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "daemon: not responding at %s\n", client.Addr())
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "daemon: running at %s (%d jobs)\n", client.Addr(), jobs)
		return nil
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Ask a running daemon to shut down",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := daemonClient()
		if err != nil {
			return err
		}
		if err := client.Shutdown(cmd.Context()); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "daemon: stopping")
		return nil
	},
}

// daemonClient dials the daemon named by the current configuration.
func daemonClient() (*daemon.Client, error) {
	application, err := app.New()
	if err != nil {
		return nil, err
	}
	return daemon.Dial(application.Config.StateDir)
}

// liveDaemon returns a client only when a daemon actually answers, so callers
// can fall back to in-process work without treating a stale control file as a
// running daemon.
func liveDaemon() *daemon.Client {
	client, err := daemonClient()
	if err != nil {
		return nil
	}
	if _, err := client.Health(); err != nil {
		return nil
	}
	return client
}

func init() {
	daemonRunCmd.Flags().StringVar(&daemonAddr, "addr", "127.0.0.1:0", "loopback address to listen on")
	daemonCmd.AddCommand(daemonRunCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	rootCmd.AddCommand(daemonCmd)
}
