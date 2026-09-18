package nativehost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// HostName is the native-messaging host name the extension connects to.
const HostName = "pads"

// DefaultExtensionID matches the ID declared in the bundled extension's
// manifest. Only extensions listed here may talk to the host.
const DefaultExtensionID = "pads@localhost"

// manifest is Firefox's native-messaging host manifest. Firefox gates access by
// extension ID; Chrome uses an origin list instead, which is why this is
// Firefox-shaped.
type manifest struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Path              string   `json:"path"`
	Type              string   `json:"type"`
	AllowedExtensions []string `json:"allowed_extensions"`
}

// InstallResult reports what Install wrote, so the command can tell the user
// which files to remove later.
type InstallResult struct {
	ManifestPath string
	WrapperPath  string
}

// ManifestDir returns the per-user directory Firefox reads host manifests from.
func ManifestDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Mozilla", "NativeMessagingHosts"), nil
	case "windows":
		// Firefox finds the manifest through a registry key on Windows, which
		// this installer does not write.
		return "", fmt.Errorf("automatic installation is not supported on windows; register the manifest manually")
	default:
		return filepath.Join(home, ".mozilla", "native-messaging-hosts"), nil
	}
}

// Install writes the wrapper script and the host manifest, and returns their
// paths. Firefox executes the manifest's path with no arguments, so the wrapper
// exists purely to turn that into "pads nativehost".
func Install(stateDir, extensionID string) (InstallResult, error) {
	if extensionID == "" {
		extensionID = DefaultExtensionID
	}

	binary, err := os.Executable()
	if err != nil {
		return InstallResult{}, fmt.Errorf("resolve pads binary: %w", err)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return InstallResult{}, fmt.Errorf("resolve pads binary path: %w", err)
	}

	dir, err := ManifestDir()
	if err != nil {
		return InstallResult{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create manifest directory: %w", err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create state directory: %w", err)
	}

	wrapperPath := filepath.Join(stateDir, "pads-nativehost")
	script := fmt.Sprintf("#!/bin/sh\nexec %q nativehost \"$@\"\n", binary)
	if err := os.WriteFile(wrapperPath, []byte(script), 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("write host wrapper: %w", err)
	}

	doc := manifest{
		Name:              HostName,
		Description:       "PADS download handoff",
		Path:              wrapperPath,
		Type:              "stdio",
		AllowedExtensions: []string{extensionID},
	}
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return InstallResult{}, fmt.Errorf("encode manifest: %w", err)
	}

	manifestPath := filepath.Join(dir, HostName+".json")
	if err := os.WriteFile(manifestPath, append(encoded, '\n'), 0o644); err != nil {
		return InstallResult{}, fmt.Errorf("write manifest: %w", err)
	}

	return InstallResult{ManifestPath: manifestPath, WrapperPath: wrapperPath}, nil
}

// Uninstall removes the manifest and wrapper written by Install.
func Uninstall(stateDir string) error {
	dir, err := ManifestDir()
	if err != nil {
		return err
	}

	for _, path := range []string{
		filepath.Join(dir, HostName+".json"),
		filepath.Join(stateDir, "pads-nativehost"),
	} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	return nil
}
