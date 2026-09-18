package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"pads/internal/app"
	"pads/internal/nativehost"
)

var nativehostExtensionID string

var nativehostCmd = &cobra.Command{
	Use:   "nativehost",
	Short: "Browser native-messaging bridge to the daemon",
	Long: "Reads native-messaging frames on stdin and forwards them to the PADS daemon.\n\n" +
		"Firefox starts this itself; it is not meant to be run by hand. Use\n" +
		"'pads nativehost install' once to register it with Firefox.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}
		// Anything written to stdout is a protocol frame, so diagnostics must
		// not go there.
		return nativehost.New(application.Config).Serve(cmd.Context(), os.Stdin, os.Stdout)
	},
}

var nativehostInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Register the native-messaging host with Firefox",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}
		result, err := nativehost.Install(application.Config.StateDir, nativehostExtensionID)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "manifest: %s\n", result.ManifestPath)
		fmt.Fprintf(out, "wrapper:  %s\n", result.WrapperPath)
		fmt.Fprintln(out, "\nLoad extension/firefox in Firefox, then restart Firefox to pick up the host.")
		return nil
	},
}

var nativehostUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the native-messaging host registration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}
		if err := nativehost.Uninstall(application.Config.StateDir); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "native-messaging host removed")
		return nil
	},
}

func init() {
	nativehostInstallCmd.Flags().StringVar(
		&nativehostExtensionID,
		"extension-id",
		nativehost.DefaultExtensionID,
		"extension ID permitted to use the host",
	)
	nativehostCmd.AddCommand(nativehostInstallCmd)
	nativehostCmd.AddCommand(nativehostUninstallCmd)
	rootCmd.AddCommand(nativehostCmd)
}
