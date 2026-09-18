package nativehost

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func readManifest(t *testing.T, path string) manifest {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var doc manifest
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return doc
}

// The two browser families disagree on how a host names the extensions allowed
// to use it, and writing the wrong key silently blocks every connection.
func TestInstallWritesPerFamilyManifests(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("installation is not supported on %s", runtime.GOOS)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	stateDir := filepath.Join(home, ".pads")
	firefox, _ := LookupBrowser("firefox")
	chrome, _ := LookupBrowser("chrome")

	results, wrapper, err := Install(Options{
		StateDir: stateDir,
		Browsers: []Browser{firefox, chrome},
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	for _, result := range results {
		doc := readManifest(t, result.ManifestPath)
		if doc.Name != HostName || doc.Type != "stdio" {
			t.Fatalf("%s: unexpected manifest %+v", result.Browser, doc)
		}
		if doc.Path != wrapper {
			t.Fatalf("%s: path = %q, want the wrapper %q", result.Browser, doc.Path, wrapper)
		}

		switch result.Browser {
		case "firefox":
			if len(doc.AllowedExtensions) != 1 || doc.AllowedExtensions[0] != DefaultFirefoxID {
				t.Fatalf("firefox: allowed_extensions = %v", doc.AllowedExtensions)
			}
			if len(doc.AllowedOrigins) != 0 {
				t.Fatalf("firefox manifest must not carry allowed_origins: %v", doc.AllowedOrigins)
			}
		case "chrome":
			want := "chrome-extension://" + DefaultChromiumID + "/"
			if len(doc.AllowedOrigins) != 1 || doc.AllowedOrigins[0] != want {
				t.Fatalf("chrome: allowed_origins = %v, want %q", doc.AllowedOrigins, want)
			}
			if len(doc.AllowedExtensions) != 0 {
				t.Fatalf("chrome manifest must not carry allowed_extensions: %v", doc.AllowedExtensions)
			}
		}
	}
}

// Brave reads the same manifest format as Chrome but from its own directory, so
// registering Chrome alone leaves Brave unable to reach the host.
func TestInstallTargetsEachChromiumBrowserSeparately(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("installation is not supported on %s", runtime.GOOS)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	seen := make(map[string]string)
	for _, name := range []string{"chrome", "brave", "chromium", "edge"} {
		browser, ok := LookupBrowser(name)
		if !ok {
			t.Fatalf("browser %q is not registered", name)
		}
		dir, err := browser.ManifestDir()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if other, clash := seen[dir]; clash {
			t.Fatalf("%s and %s share manifest directory %s", name, other, dir)
		}
		seen[dir] = name

		if !strings.HasPrefix(dir, home) {
			t.Fatalf("%s: directory %q is outside the home directory", name, dir)
		}
	}

	brave, _ := LookupBrowser("brave")
	results, _, err := Install(Options{StateDir: filepath.Join(home, ".pads"), Browsers: []Browser{brave}})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(results) != 1 || results[0].Browser != "brave" {
		t.Fatalf("results = %+v", results)
	}
	if _, err := os.Stat(results[0].ManifestPath); err != nil {
		t.Fatalf("brave manifest missing: %v", err)
	}
}

func TestInstallHonoursIDOverrides(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("installation is not supported on %s", runtime.GOOS)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	chrome, _ := LookupBrowser("chrome")
	results, _, err := Install(Options{
		StateDir:   filepath.Join(home, ".pads"),
		Browsers:   []Browser{chrome},
		ChromiumID: "abcdefghijklmnopabcdefghijklmnop",
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	doc := readManifest(t, results[0].ManifestPath)
	want := "chrome-extension://abcdefghijklmnopabcdefghijklmnop/"
	if doc.AllowedOrigins[0] != want {
		t.Fatalf("allowed_origins = %v, want %q", doc.AllowedOrigins, want)
	}
}

// Install with no browsers named must not invent configuration directories for
// browsers that are not on the machine.
func TestInstallSkipsAbsentBrowsers(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("installation is not supported on %s", runtime.GOOS)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, _, err := Install(Options{StateDir: filepath.Join(home, ".pads")}); err == nil {
		t.Fatal("expected an error when no browser is installed")
	}

	// Make one browser look present and install again.
	brave, _ := LookupBrowser("brave")
	braveDir, err := brave.ManifestDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(braveDir), 0o755); err != nil {
		t.Fatal(err)
	}

	results, _, err := Install(Options{StateDir: filepath.Join(home, ".pads")})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(results) != 1 || results[0].Browser != "brave" {
		t.Fatalf("results = %+v, want brave alone", results)
	}
}

func TestUninstallRemovesEveryManifest(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("installation is not supported on %s", runtime.GOOS)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := filepath.Join(home, ".pads")

	firefox, _ := LookupBrowser("firefox")
	brave, _ := LookupBrowser("brave")
	results, wrapper, err := Install(Options{StateDir: stateDir, Browsers: []Browser{firefox, brave}})
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	removed, err := Uninstall(stateDir)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(removed) != len(results)+1 {
		t.Fatalf("removed %v, want %d manifests plus the wrapper", removed, len(results))
	}
	for _, result := range results {
		if _, err := os.Stat(result.ManifestPath); !os.IsNotExist(err) {
			t.Fatalf("%s manifest still present", result.Browser)
		}
	}
	if _, err := os.Stat(wrapper); !os.IsNotExist(err) {
		t.Fatal("wrapper still present")
	}

	// A second uninstall is a no-op rather than an error.
	again, err := Uninstall(stateDir)
	if err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("second uninstall removed %v", again)
	}
}

// Chromium derives an extension's ID from the public key pinned in its
// manifest. If that key and DefaultChromiumID ever disagree, the host manifest
// names an ID no extension has and every connection is refused with nothing to
// show for it, so the two are checked against each other here.
func TestDefaultChromiumIDMatchesManifestKey(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "extension", "manifest.chrome.json"))
	if err != nil {
		t.Skipf("chrome manifest not readable: %v", err)
	}

	var doc struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse chrome manifest: %v", err)
	}
	if doc.Key == "" {
		t.Fatal("chrome manifest has no key; the extension ID would vary per machine")
	}

	der, err := base64.StdEncoding.DecodeString(doc.Key)
	if err != nil {
		t.Fatalf("decode key: %v", err)
	}

	// The ID is the first 16 bytes of the key's SHA-256, in hex, with the
	// digits 0-f mapped onto the letters a-p.
	sum := sha256.Sum256(der)
	var id strings.Builder
	for _, b := range sum[:16] {
		for _, nibble := range []byte{b >> 4, b & 0x0f} {
			id.WriteByte('a' + nibble)
		}
	}

	if id.String() != DefaultChromiumID {
		t.Fatalf("manifest key yields extension ID %q, but DefaultChromiumID is %q",
			id.String(), DefaultChromiumID)
	}
}
