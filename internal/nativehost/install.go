package nativehost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// HostName is the native-messaging host name the extension connects to.
const HostName = "pads"

// DefaultFirefoxID matches browser_specific_settings.gecko.id in the Firefox
// manifest. Firefox gates host access by extension ID.
const DefaultFirefoxID = "pads@localhost"

// DefaultChromiumID is the ID the Chromium manifest's "key" field pins. Without
// that key Chromium would derive an ID from the install path, which differs per
// machine and could not be named here.
const DefaultChromiumID = "cijdklmepoblhjbjkimmkfiipdkdnini"

// Browser is one browser family PADS can register with.
type Browser struct {
	// Name is the value accepted by --browser.
	Name string
	// Gecko selects Firefox's manifest shape: allowed_extensions with an
	// extension ID, rather than allowed_origins with a chrome-extension URL.
	Gecko bool
	// dirs maps GOOS to the browser's native-messaging directory, relative to
	// the user's home directory.
	dirs map[string][]string
	// markers are paths, relative to home, whose presence means the browser is
	// installed. An empty directory list means it is not.
	markers map[string][]string
}

// SupportedBrowsers lists every browser the installer knows about.
//
// The Chromium family all read the same manifest format from their own
// per-browser directory, which is the only reason Brave, Chromium and Edge need
// separate entries at all.
var SupportedBrowsers = []Browser{
	{
		Name:  "firefox",
		Gecko: true,
		dirs: map[string][]string{
			"linux":  {".mozilla", "native-messaging-hosts"},
			"darwin": {"Library", "Application Support", "Mozilla", "NativeMessagingHosts"},
		},
		markers: map[string][]string{
			"linux":  {".mozilla"},
			"darwin": {"Library", "Application Support", "Mozilla"},
		},
	},
	{
		Name: "chrome",
		dirs: map[string][]string{
			"linux":  {".config", "google-chrome", "NativeMessagingHosts"},
			"darwin": {"Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts"},
		},
		markers: map[string][]string{
			"linux":  {".config", "google-chrome"},
			"darwin": {"Library", "Application Support", "Google", "Chrome"},
		},
	},
	{
		Name: "brave",
		dirs: map[string][]string{
			"linux":  {".config", "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"},
			"darwin": {"Library", "Application Support", "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"},
		},
		markers: map[string][]string{
			"linux":  {".config", "BraveSoftware", "Brave-Browser"},
			"darwin": {"Library", "Application Support", "BraveSoftware", "Brave-Browser"},
		},
	},
	{
		Name: "chromium",
		dirs: map[string][]string{
			"linux":  {".config", "chromium", "NativeMessagingHosts"},
			"darwin": {"Library", "Application Support", "Chromium", "NativeMessagingHosts"},
		},
		markers: map[string][]string{
			"linux":  {".config", "chromium"},
			"darwin": {"Library", "Application Support", "Chromium"},
		},
	},
	{
		Name: "edge",
		dirs: map[string][]string{
			"linux":  {".config", "microsoft-edge", "NativeMessagingHosts"},
			"darwin": {"Library", "Application Support", "Microsoft Edge", "NativeMessagingHosts"},
		},
		markers: map[string][]string{
			"linux":  {".config", "microsoft-edge"},
			"darwin": {"Library", "Application Support", "Microsoft Edge"},
		},
	},
}

// BrowserNames lists the accepted --browser values in a stable order.
func BrowserNames() []string {
	names := make([]string, 0, len(SupportedBrowsers))
	for _, b := range SupportedBrowsers {
		names = append(names, b.Name)
	}
	sort.Strings(names)
	return names
}

// LookupBrowser finds a browser by name.
func LookupBrowser(name string) (Browser, bool) {
	for _, b := range SupportedBrowsers {
		if strings.EqualFold(b.Name, name) {
			return b, true
		}
	}
	return Browser{}, false
}

// ManifestDir returns the directory this browser reads host manifests from.
func (b Browser) ManifestDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	parts, ok := b.dirs[runtime.GOOS]
	if !ok {
		// Windows locates the manifest through a registry key, which this
		// installer does not write.
		return "", fmt.Errorf("%s registration is not supported on %s", b.Name, runtime.GOOS)
	}
	return filepath.Join(append([]string{home}, parts...)...), nil
}

// Installed reports whether the browser appears to be present, so that "install
// everywhere" does not create configuration directories for browsers the user
// does not have.
func (b Browser) Installed() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	parts, ok := b.markers[runtime.GOOS]
	if !ok {
		return false
	}
	info, err := os.Stat(filepath.Join(append([]string{home}, parts...)...))
	return err == nil && info.IsDir()
}

// manifest is a native-messaging host manifest. The two families disagree on
// exactly one field, so both are declared and the unused one is omitted.
type manifest struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Path              string   `json:"path"`
	Type              string   `json:"type"`
	AllowedExtensions []string `json:"allowed_extensions,omitempty"`
	AllowedOrigins    []string `json:"allowed_origins,omitempty"`
}

// Options configures installation.
type Options struct {
	StateDir string
	// Browsers to register with. Empty means every browser that is installed.
	Browsers []Browser
	// FirefoxID and ChromiumID override the extension IDs permitted to use the
	// host, for anyone running a repacked extension.
	FirefoxID  string
	ChromiumID string
}

// InstallResult reports one browser's registration.
type InstallResult struct {
	Browser      string
	ManifestPath string
}

// Install writes the wrapper script and one host manifest per browser.
func Install(opts Options) ([]InstallResult, string, error) {
	if opts.FirefoxID == "" {
		opts.FirefoxID = DefaultFirefoxID
	}
	if opts.ChromiumID == "" {
		opts.ChromiumID = DefaultChromiumID
	}

	targets := opts.Browsers
	if len(targets) == 0 {
		for _, b := range SupportedBrowsers {
			if b.Installed() {
				targets = append(targets, b)
			}
		}
	}
	if len(targets) == 0 {
		return nil, "", fmt.Errorf("no supported browser found; name one with --browser (%s)",
			strings.Join(BrowserNames(), ", "))
	}

	wrapperPath, err := writeWrapper(opts.StateDir)
	if err != nil {
		return nil, "", err
	}

	results := make([]InstallResult, 0, len(targets))
	for _, browser := range targets {
		dir, err := browser.ManifestDir()
		if err != nil {
			return nil, "", err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, "", fmt.Errorf("create %s manifest directory: %w", browser.Name, err)
		}

		doc := manifest{
			Name:        HostName,
			Description: "PADS download handoff",
			Path:        wrapperPath,
			Type:        "stdio",
		}
		if browser.Gecko {
			doc.AllowedExtensions = []string{opts.FirefoxID}
		} else {
			doc.AllowedOrigins = []string{"chrome-extension://" + opts.ChromiumID + "/"}
		}

		encoded, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return nil, "", fmt.Errorf("encode %s manifest: %w", browser.Name, err)
		}

		path := filepath.Join(dir, HostName+".json")
		if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
			return nil, "", fmt.Errorf("write %s manifest: %w", browser.Name, err)
		}
		results = append(results, InstallResult{Browser: browser.Name, ManifestPath: path})
	}
	return results, wrapperPath, nil
}

// writeWrapper creates the script the browser executes. A manifest's path is
// run with no arguments, so the wrapper exists only to turn that into
// "pads nativehost".
func writeWrapper(stateDir string) (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve pads binary: %w", err)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return "", fmt.Errorf("resolve pads binary path: %w", err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", fmt.Errorf("create state directory: %w", err)
	}

	path := filepath.Join(stateDir, "pads-nativehost")
	script := fmt.Sprintf("#!/bin/sh\nexec %q nativehost \"$@\"\n", binary)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return "", fmt.Errorf("write host wrapper: %w", err)
	}
	return path, nil
}

// Uninstall removes every manifest this installer may have written, plus the
// wrapper script.
func Uninstall(stateDir string) ([]string, error) {
	var removed []string

	for _, browser := range SupportedBrowsers {
		dir, err := browser.ManifestDir()
		if err != nil {
			// Unsupported on this platform, so nothing was ever written.
			continue
		}
		path := filepath.Join(dir, HostName+".json")
		if err := os.Remove(path); err != nil {
			if !os.IsNotExist(err) {
				return removed, fmt.Errorf("remove %s: %w", path, err)
			}
			continue
		}
		removed = append(removed, path)
	}

	wrapper := filepath.Join(stateDir, "pads-nativehost")
	if err := os.Remove(wrapper); err != nil && !os.IsNotExist(err) {
		return removed, fmt.Errorf("remove %s: %w", wrapper, err)
	} else if err == nil {
		removed = append(removed, wrapper)
	}
	return removed, nil
}
