package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/config/auditlog"
	"github.com/kastheco/kasmos/config/taskstore"
	"github.com/kastheco/kasmos/internal/mcpserver"
	"github.com/kastheco/kasmos/internal/mcpserver/fstools"
	"github.com/kastheco/kasmos/internal/mcpserver/gittools"
	webassets "github.com/kastheco/kasmos/web"
	"github.com/spf13/cobra"
)

// MCPVersion is the version advertised in MCP initialize responses.
var MCPVersion = "0.1.0"

// NewServeCmd returns the `kas serve` cobra command.
// It starts an HTTP server backed by a SQLite task store, and optionally
// an MCP server on a second port sharing the same store and signal gateway.
func NewServeCmd() *cobra.Command {
	var (
		port       int
		db         string
		bind       string
		mcpEnabled bool
		mcpPort    int
		adminDir   string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "start the task store HTTP server",
		Long:  "Start an HTTP server that exposes task state over a REST API backed by SQLite.",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := taskstore.NewSQLiteStore(db)
			if err != nil {
				return fmt.Errorf("open task store: %w", err)
			}
			defer store.Close()

			gw, err := taskstore.NewSQLiteSignalGateway(db)
			if err != nil {
				return fmt.Errorf("open signal gateway: %w", err)
			}
			defer gw.Close()

			logger, err := auditlog.NewSQLiteLogger(db)
			if err != nil {
				return fmt.Errorf("open audit logger: %w", err)
			}
			defer logger.Close()

			taskAPI := taskstore.NewHandler(store)
			auditAPI := auditlog.NewHandler(logger)

			rootMux := http.NewServeMux()
			rootMux.Handle("/v1/ping", taskAPI)
			// Route audit-events exactly, then fall through to the task API for everything else.
			// Go 1.22+ mux gives the more-specific method+path pattern precedence over the
			// plain prefix, so GET audit-events is handled by auditAPI and all other
			// /v1/projects/* requests continue to taskAPI.
			rootMux.Handle("GET /v1/projects/{project}/audit-events", auditAPI)
			rootMux.Handle("/v1/projects/", taskAPI)

			// Resolve the admin filesystem: --admin-dir flag overrides embedded assets.
			// Require the directory to contain index.html so users aren't accidentally
			// served a source tree (e.g. web/admin/) instead of the built output (dist/).
			var adminFS http.FileSystem
			if adminDir != "" {
				if _, err := os.Stat(adminDir); err != nil {
					return fmt.Errorf("stat admin dir: %w", err)
				}
				if _, err := os.Stat(filepath.Join(adminDir, "index.html")); err != nil {
					return fmt.Errorf("admin dir must contain index.html (point --admin-dir at the dist/ directory): %w", err)
				}
				adminFS = http.Dir(adminDir)
			} else {
				adminFS = webassets.AdminFS()
			}

			rootMux.Handle("/admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
			rootMux.Handle("/admin/", http.StripPrefix("/admin", adminFallbackHandler(adminFS)))
			fmt.Println("admin UI available at /admin/")

			addr := fmt.Sprintf("%s:%d", bind, port)

			srv := &http.Server{
				Addr:    addr,
				Handler: rootMux,
			}

			fmt.Printf("task store listening on http://%s (db: %s)\n", addr, db)

			// Graceful shutdown on SIGINT/SIGTERM.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			// errCh has capacity 2 so neither goroutine blocks on send when both fail.
			errCh := make(chan error, 2)

			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()

			var mcpHTTP *http.Server
			if mcpEnabled {
				mcpSrv := mcpserver.NewServer(MCPVersion, store, gw)
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("get working directory: %w", err)
				}
				allowedDirs := []string{cwd}
				if root, rootErr := config.ResolveRepoRoot(cwd); rootErr == nil && root != "" && root != cwd {
					allowedDirs = append(allowedDirs, root)
				}
				fstools.RegisterTools(mcpSrv.MCPServer(), allowedDirs)
				gittools.RegisterTools(mcpSrv.MCPServer(), allowedDirs)
				mcpAddr := fmt.Sprintf("%s:%d", bind, mcpPort)
				mcpHTTP = &http.Server{Addr: mcpAddr, Handler: mcpSrv.Handler()}
				fmt.Printf("mcp server listening on http://%s/mcp\n", mcpAddr)
				go func() {
					if err := mcpHTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						errCh <- err
					}
				}()
			}

			select {
			case err := <-errCh:
				return err
			case <-ctx.Done():
				fmt.Println("\nshutting down...")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if mcpHTTP != nil {
					_ = mcpHTTP.Shutdown(shutdownCtx)
				}
				return srv.Shutdown(shutdownCtx)
			}
		},
	}

	defaultDB := taskstore.ResolvedDBPath()

	cmd.Flags().IntVar(&port, "port", 7433, "port to listen on")
	cmd.Flags().StringVar(&db, "db", defaultDB, "path to the SQLite database file")
	cmd.Flags().StringVar(&bind, "bind", "0.0.0.0", "address to bind to")
	cmd.Flags().BoolVar(&mcpEnabled, "mcp", true, "enable the MCP server (Streamable HTTP on --mcp-port)")
	cmd.Flags().IntVar(&mcpPort, "mcp-port", 7434, "port for the MCP server")
	cmd.Flags().StringVar(&adminDir, "admin-dir", "", "path to the built admin SPA dist/ directory (overrides embedded assets)")

	return cmd
}
