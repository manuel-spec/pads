package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "pads",
	Short: "Predictive Adaptive Download Scheduler",
	Long:  "PADS is a CLI download manager with segmented downloads, adaptive connection scaling, and resume support.",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize()
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
