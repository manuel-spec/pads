package daemon

// IsRunning reports whether a daemon is listening on the published address and
// accepts the published token. A stale control file therefore reads as "not
// running" rather than as a live daemon.
func IsRunning(stateDir string) bool {
	client, err := Dial(stateDir)
	if err != nil {
		return false
	}
	_, err = client.Health()
	return err == nil
}
