package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var getOutput string

var getCmd = &cobra.Command{
	Use:   "get <url>",
	Short: "Download a file from a URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		url := args[0]
		return fmt.Errorf("download not yet implemented: %s", url)
	},
}

func init() {
	getCmd.Flags().StringVarP(&getOutput, "output", "o", "", "output file path")
	rootCmd.AddCommand(getCmd)
}
