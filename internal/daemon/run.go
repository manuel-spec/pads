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

// Run starts the daemon: it takes the state-directory lock, binds a loopback
// listener, publishes the control file, and blocks until SIGINT/SIGTERM or a
// shutdown request arrives.
func Run(ctx context.Context, addr string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	application, err := app.New()
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	if err := application.EnsureDirs(); err != nil {
		return err
	}
	owned, err := acquireOwnership(application.Config.StateDir)
	if err != nil {
		return err
	}
	defer owned.release()

	token, err := Token()
	if err != nil {
		return err
	}
	manager := NewManager(application)
	defer manager.Close()
	server := NewServer(manager, token)
	defer server.http.Close()

	listener, err := server.Listen(addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	listenAddr := listener.Addr().String()
	if err := owned.writeInfo(daemonInfo{PID: os.Getpid(), Addr: listenAddr, Token: token}); err != nil {
		return err
	}
	log.Printf("pads daemon listening on %s (pid %d)", listenAddr, os.Getpid())

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	select {
	case <-ctx.Done():
	case <-server.ShutdownRequested():
		log.Printf("pads daemon: shutdown requested")
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	}

	manager.Close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.http.Close()
		<-serveErr
		return fmt.Errorf("shutdown daemon: %w", err)
	}
	if err := <-serveErr; err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

const shutdownTimeout = 5 * time.Second
