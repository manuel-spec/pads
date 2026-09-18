package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func Token() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

type daemonInfo struct {
	PID   int    `json:"pid"`
	Addr  string `json:"addr"`
	Token string `json:"token"`
}

type ownership struct {
	lockPath string
	lock     os.FileInfo
	infoPath string
	info     os.FileInfo
}

func acquireOwnership(stateDir string) (*ownership, error) {
	path := filepath.Join(stateDir, "daemon.lock")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("daemon lock %q already exists; stop the existing daemon, or if it is no longer running manually remove this stale lock (no automatic PID killing)", path)
		}
		return nil, fmt.Errorf("create daemon lock: %w", err)
	}
	owned := &ownership{lockPath: path, infoPath: filepath.Join(stateDir, "daemon.json")}
	owned.lock, err = f.Stat()
	closeErr := f.Close()
	if err != nil {
		return nil, fmt.Errorf("stat daemon lock: %w", err)
	}
	if closeErr != nil {
		owned.release()
		return nil, fmt.Errorf("close daemon lock: %w", closeErr)
	}
	return owned, nil
}

func (o *ownership) writeInfo(info daemonInfo) error {
	if !sameFile(o.lockPath, o.lock) {
		return errors.New("daemon lock ownership lost")
	}
	f, err := os.CreateTemp(filepath.Dir(o.infoPath), ".daemon-*")
	if err != nil {
		return fmt.Errorf("create daemon info: %w", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(info); err != nil {
		return fmt.Errorf("write daemon info: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync daemon info: %w", err)
	}
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), o.infoPath); err != nil {
		return fmt.Errorf("publish daemon info: %w", err)
	}
	o.info = stat
	return nil
}

func sameFile(path string, owned os.FileInfo) bool {
	current, err := os.Lstat(path)
	return err == nil && owned != nil && os.SameFile(current, owned)
}

func (o *ownership) release() {
	if !sameFile(o.lockPath, o.lock) {
		return
	}
	if sameFile(o.infoPath, o.info) {
		_ = os.Remove(o.infoPath)
	}
	_ = os.Remove(o.lockPath)
}

func ReadInfo(stateDir string) (pid int, addr, token string, err error) {
	f, err := os.Open(filepath.Join(stateDir, "daemon.json"))
	if err != nil {
		return 0, "", "", fmt.Errorf("read daemon info: %w", err)
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, 4097))
	dec.DisallowUnknownFields()
	var info daemonInfo
	if err := dec.Decode(&info); err != nil {
		return 0, "", "", fmt.Errorf("decode daemon info: %w", err)
	}
	if err := dec.Decode(new(interface{})); err != io.EOF {
		return 0, "", "", errors.New("invalid trailing daemon info")
	}
	if info.PID <= 0 || info.Token == "" {
		return 0, "", "", errors.New("invalid daemon info")
	}
	if err := validateAddress(info.Addr); err != nil {
		return 0, "", "", err
	}
	return info.PID, info.Addr, info.Token, nil
}
