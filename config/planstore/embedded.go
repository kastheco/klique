package planstore

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// EmbeddedServer wraps an HTTP server + SQLite store that runs in-process.
// The TUI starts this on boot and stops it on exit.
type EmbeddedServer struct {
	store   *SQLiteStore
	server  *http.Server
	url     string
	stopped sync.Once
}

// StartEmbedded opens the SQLite DB, creates the HTTP handler, and starts
// listening on 127.0.0.1:port. Use port 0 for auto-assignment.
// Returns the running server and its base URL.
func StartEmbedded(dbPath string, port int) (*EmbeddedServer, error) {
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("embedded server: open db: %w", err)
	}

	handler := NewHandler(store)
	srv := &http.Server{Handler: handler}

	// Listen on the specified port (0 = OS-assigned).
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("embedded server: listen: %w", err)
	}

	baseURL := fmt.Sprintf("http://%s", ln.Addr().String())

	embedded := &EmbeddedServer{
		store:  store,
		server: srv,
		url:    baseURL,
	}

	// Start serving in a background goroutine.
	go func() {
		// ErrServerClosed is expected on graceful shutdown — ignore it.
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			// Nothing useful to do here; the server is embedded and the
			// caller has no channel to receive errors on. Log would be
			// ideal but planstore has no logger dependency.
			_ = err
		}
	}()

	return embedded, nil
}

// URL returns the base URL (e.g. "http://127.0.0.1:7433").
func (s *EmbeddedServer) URL() string { return s.url }

// Store returns the underlying SQLite store for direct access (audit log, etc.).
func (s *EmbeddedServer) Store() *SQLiteStore { return s.store }

// Stop gracefully shuts down the HTTP server and closes the DB.
// It is safe to call Stop multiple times — subsequent calls are no-ops.
func (s *EmbeddedServer) Stop() {
	s.stopped.Do(func() {
		// Shutdown with a background context — we don't need to wait for
		// in-flight requests since this is a local embedded server.
		_ = s.server.Shutdown(context.Background())
		_ = s.store.Close()
	})
}
