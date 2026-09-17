package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pads/internal/app"
)

// Run starts the daemon: it binds a loopback listener, writes control
// files, and blocks until SIGINT/SIGTERM.
func Run(ctx context.Context, addr string) error {
	application, err := app.New()
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	if err := application.EnsureDirs(); err != nil {
		return err
	}

	token, err := Token()
	if err != nil {
		return err
	}
	manager := NewManager(application)
	server := NewServer(manager, token)

	listener, err := server.Listen(addr)
	if err != nil {
		return err
	}
	listenAddr := listener.Addr().String()

	if err := WriteFiles(application.Config.StateDir, fmt.Sprintf("%d", os.Getpid()), token); err != nil {
		return err
	}
	if err := os.WriteFile(
		fmt.Sprintf("%s/daemon.addr", application.Config.StateDir),
		[]byte(listenAddr+"\n"), 0o600,
	); err != nil {
		return fmt.Errorf("write addr file: %w", err)
	}

	log.Printf("pads daemon listening on %s (pid %d)", listenAddr, os.Getpid())

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if err != nil {
			RemoveFiles(application.Config.StateDir)
			return fmt.Errorf("serve: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("daemon: shutdown: %v", err)
	}
	RemoveFiles(application.Config.StateDir)
	log.Printf("pads daemon stopped")
	return nil
}

const shutdownTimeout = 5 * time.Second
