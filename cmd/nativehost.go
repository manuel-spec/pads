package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pads/internal/app"
	"pads/internal/nativehost"
)

var (
	nativehostBrowsers   []string
	nativehostFirefoxID  string
	nativehostChromiumID string
)

var nativehostCmd = &cobra.Command{
	Use:   "nativehost",
	Short: "Browser native-messaging bridge to the daemon",
	Long: "Reads native-messaging frames on stdin and forwards them to the PADS daemon.\n\n" +
		"The browser starts this itself; it is not meant to be run by hand. Use\n" +
		"'pads nativehost install' once to register it.",
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
	Short: "Register the native-messaging host with your browsers",
	Long: "Registers the host with each browser given by --browser, or with every\n" +
		"supported browser found on this machine when none is named.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return err
		}

		browsers, err := resolveBrowsers(nativehostBrowsers)
		if err != nil {
			return err
		}

		results, wrapper, err := nativehost.Install(nativehost.Options{
			StateDir:   application.Config.StateDir,
			Browsers:   browsers,
			FirefoxID:  nativehostFirefoxID,
			ChromiumID: nativehostChromiumID,
		})
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		for _, result := range results {
			fmt.Fprintf(out, "%-9s %s\n", result.Browser, result.ManifestPath)
		}
		fmt.Fprintf(out, "%-9s %s\n", "wrapper", wrapper)
		fmt.Fprintln(out, "\nBuild the extension with 'make extension', load dist/extension/<browser>,")
		fmt.Fprintln(out, "then restart the browser so it picks up the host.")
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
		removed, err := nativehost.Uninstall(application.Config.StateDir)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		if len(removed) == 0 {
			fmt.Fprintln(out, "nothing to remove")
			return nil
		}
		for _, path := range removed {
			fmt.Fprintf(out, "removed %s\n", path)
		}
		return nil
	},
}

// resolveBrowsers maps --browser values onto known browsers. An empty list
// means "every browser installed here", which Install works out for itself.
func resolveBrowsers(names []string) ([]nativehost.Browser, error) {
	var browsers []nativehost.Browser
	for _, name := range names {
		browser, ok := nativehost.LookupBrowser(name)
		if !ok {
			return nil, fmt.Errorf("unknown browser %q; supported browsers are %s",
				name, strings.Join(nativehost.BrowserNames(), ", "))
		}
		browsers = append(browsers, browser)
	}
	return browsers, nil
}

func init() {
	nativehostInstallCmd.Flags().StringSliceVar(
		&nativehostBrowsers,
		"browser",
		nil,
		"browsers to register with ("+strings.Join(nativehost.BrowserNames(), ", ")+"); defaults to every one found",
	)
	nativehostInstallCmd.Flags().StringVar(
		&nativehostFirefoxID,
		"firefox-id",
		nativehost.DefaultFirefoxID,
		"Firefox extension ID permitted to use the host",
	)
	nativehostInstallCmd.Flags().StringVar(
		&nativehostChromiumID,
		"chromium-id",
		nativehost.DefaultChromiumID,
		"Chromium extension ID permitted to use the host",
	)
	nativehostCmd.AddCommand(nativehostInstallCmd)
	nativehostCmd.AddCommand(nativehostUninstallCmd)
	rootCmd.AddCommand(nativehostCmd)
}
