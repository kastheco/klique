package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/spf13/cobra"
)

// NewServeCmd returns the `kas serve` cobra command.
// It starts an HTTP server backed by a SQLite plan store.
func NewServeCmd() *cobra.Command {
	var (
		port int
		db   string
		bind string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "start the plan store HTTP server",
		Long:  "Start an HTTP server that exposes plan state over a REST API backed by SQLite.",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := taskstore.NewSQLiteStore(db)
			if err != nil {
				return fmt.Errorf("open plan store: %w", err)
			}
			defer store.Close()

			handler := taskstore.NewHandler(store)
			addr := fmt.Sprintf("%s:%d", bind, port)

			srv := &http.Server{
				Addr:    addr,
				Handler: handler,
			}

			fmt.Printf("plan store listening on http://%s (db: %s)\n", addr, db)

			// Graceful shutdown on SIGINT/SIGTERM.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			errCh := make(chan error, 1)
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
				close(errCh)
			}()

			select {
			case err := <-errCh:
				return err
			case <-ctx.Done():
				fmt.Println("\nshutting down...")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return srv.Shutdown(shutdownCtx)
			}
		},
	}

	defaultDB := taskstore.ResolvedDBPath()

	cmd.Flags().IntVar(&port, "port", 7433, "port to listen on")
	cmd.Flags().StringVar(&db, "db", defaultDB, "path to the SQLite database file")
	cmd.Flags().StringVar(&bind, "bind", "0.0.0.0", "address to bind to")

	return cmd
}
