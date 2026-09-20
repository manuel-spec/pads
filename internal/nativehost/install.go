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
	// flatpakID is the Flatpak application ID, empty when PADS knows of no
	// Flatpak build.
	flatpakID string
	// flatpakDirs is the manifest directory relative to ~/.var/app/<id>/. A
	// Flatpak browser keeps its own configuration tree and never reads the one
	// under ~/.config, so registering the native location alone leaves it with
	// no host at all.
	flatpakDirs []string
}

// Target is one directory a browser reads host manifests from. A machine can
// carry both a native and a Flatpak build of the same browser, and they share
// nothing.
type Target struct {
	Browser      string
	Flatpak      bool
	FlatpakAppID string
	Dir          string
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
		flatpakID:   "org.mozilla.firefox",
		flatpakDirs: []string{".mozilla", "native-messaging-hosts"},
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
		flatpakID:   "com.google.Chrome",
		flatpakDirs: []string{"config", "google-chrome", "NativeMessagingHosts"},
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
		flatpakID:   "com.brave.Browser",
		flatpakDirs: []string{"config", "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"},
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
		flatpakID:   "org.chromium.Chromium",
		flatpakDirs: []string{"config", "chromium", "NativeMessagingHosts"},
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
		flatpakID:   "com.microsoft.Edge",
		flatpakDirs: []string{"config", "microsoft-edge", "NativeMessagingHosts"},
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

// ManifestDir returns the directory a native install of this browser reads host
// manifests from.
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

// FlatpakManifestDir returns the directory a Flatpak build of this browser reads
// host manifests from. A Flatpak browser's configuration lives under
// ~/.var/app/<id> and it never reads the native location.
func (b Browser) FlatpakManifestDir() (string, error) {
	if b.flatpakID == "" {
		return "", fmt.Errorf("no Flatpak build is known for %s", b.Name)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	parts := append([]string{home, ".var", "app", b.flatpakID}, b.flatpakDirs...)
	return filepath.Join(parts...), nil
}

// Installed reports whether a native build appears to be present, so that
// "install everywhere" does not create configuration directories for browsers
// the user does not have.
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

// FlatpakInstalled reports whether a Flatpak build is present.
func (b Browser) FlatpakInstalled() bool {
	if b.flatpakID == "" {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(home, ".var", "app", b.flatpakID))
	return err == nil && info.IsDir()
}

// Targets lists every manifest directory this browser actually reads on this
// machine. Both builds can be installed at once and they share nothing.
func (b Browser) Targets() []Target {
	var targets []Target
	if b.Installed() {
		if dir, err := b.ManifestDir(); err == nil {
			targets = append(targets, Target{Browser: b.Name, Dir: dir})
		}
	}
	if b.FlatpakInstalled() {
		if dir, err := b.FlatpakManifestDir(); err == nil {
			targets = append(targets, Target{
				Browser:      b.Name,
				Flatpak:      true,
				FlatpakAppID: b.flatpakID,
				Dir:          dir,
			})
		}
	}
	return targets
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
	Flatpak      bool
	FlatpakAppID string
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

	browsers := opts.Browsers
	if len(browsers) == 0 {
		browsers = SupportedBrowsers
	}

	var targets []Target
	for _, browser := range browsers {
		found := browser.Targets()
		if len(found) == 0 && len(opts.Browsers) > 0 {
			// Named explicitly but nothing detected: fall back to the native
			// location rather than refusing to register anything.
			dir, err := browser.ManifestDir()
			if err != nil {
				return nil, "", err
			}
			found = []Target{{Browser: browser.Name, Dir: dir}}
		}
		for _, target := range found {
			targets = append(targets, target)
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
	for _, target := range targets {
		browser, ok := LookupBrowser(target.Browser)
		if !ok {
			return nil, "", fmt.Errorf("unknown browser %q", target.Browser)
		}
		if err := os.MkdirAll(target.Dir, 0o755); err != nil {
			return nil, "", fmt.Errorf("create %s manifest directory: %w", target.Browser, err)
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
			return nil, "", fmt.Errorf("encode %s manifest: %w", target.Browser, err)
		}

		path := filepath.Join(target.Dir, HostName+".json")
		if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
			return nil, "", fmt.Errorf("write %s manifest: %w", target.Browser, err)
		}
		results = append(results, InstallResult{
			Browser:      target.Browser,
			Flatpak:      target.Flatpak,
			FlatpakAppID: target.FlatpakAppID,
			ManifestPath: path,
		})
	}
	return results, wrapperPath, nil
}

// FlatpakOverride is the command that lets a sandboxed browser reach the host.
//
// The sandbox cannot see the pads binary or the daemon's control file, so both
// are granted read-only. That is the whole of what the host needs: it reads the
// daemon's address and token, then talks to it over loopback, which a Flatpak
// browser already shares with the host.
func FlatpakOverride(appID, binaryDir, stateDir string) string {
	return fmt.Sprintf(
		"flatpak override --user --filesystem=%s:ro --filesystem=%s:ro %s",
		binaryDir, stateDir, appID,
	)
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
		var dirs []string
		if dir, err := browser.ManifestDir(); err == nil {
			dirs = append(dirs, dir)
		}
		if dir, err := browser.FlatpakManifestDir(); err == nil {
			dirs = append(dirs, dir)
		}

		for _, dir := range dirs {
			path := filepath.Join(dir, HostName+".json")
			if err := os.Remove(path); err != nil {
				if !os.IsNotExist(err) {
					return removed, fmt.Errorf("remove %s: %w", path, err)
				}
				continue
			}
			removed = append(removed, path)
		}
	}

	wrapper := filepath.Join(stateDir, "pads-nativehost")
	if err := os.Remove(wrapper); err != nil && !os.IsNotExist(err) {
		return removed, fmt.Errorf("remove %s: %w", wrapper, err)
	} else if err == nil {
		removed = append(removed, wrapper)
	}
	return removed, nil
}
