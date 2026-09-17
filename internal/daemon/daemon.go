package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Token generates a 128-bit hex token for local API authentication.
func Token() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// WriteFiles persists the PID and token files for a running daemon.
func WriteFiles(stateDir, pid, token string) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "daemon.pid"), []byte(pid+"\n"), 0o644); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "daemon.token"), []byte(token+"\n"), 0o600); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}
	return nil
}

// ReadInfo returns the daemon PID, address, and token from the state dir.
func ReadInfo(stateDir string) (pid int, addr, token string, err error) {
	pidData, err := os.ReadFile(filepath.Join(stateDir, "daemon.pid"))
	if err != nil {
		return 0, "", "", fmt.Errorf("read pid file: %w", err)
	}
	pid, err = strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return 0, "", "", fmt.Errorf("parse pid file: %w", err)
	}
	addrData, err := os.ReadFile(filepath.Join(stateDir, "daemon.addr"))
	if err != nil {
		return 0, "", "", fmt.Errorf("read addr file: %w", err)
	}
	addr = strings.TrimSpace(string(addrData))
	tokenData, err := os.ReadFile(filepath.Join(stateDir, "daemon.token"))
	if err != nil {
		return 0, "", "", fmt.Errorf("read token file: %w", err)
	}
	token = strings.TrimSpace(string(tokenData))
	if addr == "" || token == "" {
		return 0, "", "", errors.New("daemon info files are empty")
	}
	return pid, addr, token, nil
}

// RemoveFiles deletes daemon control files on shutdown.
func RemoveFiles(stateDir string) {
	_ = os.Remove(filepath.Join(stateDir, "daemon.pid"))
	_ = os.Remove(filepath.Join(stateDir, "daemon.addr"))
	_ = os.Remove(filepath.Join(stateDir, "daemon.token"))
}

// IsRunning reports whether a daemon with the recorded PID is alive.
func IsRunning(stateDir string) bool {
	pid, _, _, err := ReadInfo(stateDir)
	if err != nil {
		return false
	}
	return processAlive(pid)
}
