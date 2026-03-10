package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kastheco/kasmos/daemon/api"
	"github.com/spf13/cobra"
)

// daemonSocketPath returns the default Unix domain socket path for the daemon
// control API. Matches the defaultSocketPath() logic in the daemon package:
// prefers $XDG_RUNTIME_DIR/kasmos/kas.sock, then falls back to
// /tmp/kasmos-<uid>/kas.sock. This keeps `kas daemon status` (and friends)
// talking to the same socket the daemon creates without requiring --socket.
func daemonSocketPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "kasmos", "kas.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("kasmos-%d", os.Getuid()), "kas.sock")
}

// daemonPIDPath returns the path to the daemon PID file.
func daemonPIDPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/kas-daemon.pid"
	}
	return filepath.Join(home, ".config", "kasmos", "daemon.pid")
}

// NewDaemonCmd returns the `kas daemon` cobra command with subcommands.
func NewDaemonCmd() *cobra.Command {
	var socketPath string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "manage the kasmos background daemon",
		Long:  "control the kasmos multi-repo background daemon that manages plan lifecycles.",
	}

	cmd.PersistentFlags().StringVar(&socketPath, "socket", daemonSocketPath(), "path to the daemon unix domain socket")

	cmd.AddCommand(newDaemonStartCmd())
	cmd.AddCommand(newDaemonStopCmd())
	// newDaemonStatusCmd takes a pointer so the persistent --socket flag is
	// respected when the subcommand executes.
	statusCmd := newDaemonStatusCmd(&socketPath)
	cmd.AddCommand(statusCmd)
	cmd.AddCommand(newDaemonAddCmd(&socketPath))
	cmd.AddCommand(newDaemonRemoveCmd(&socketPath))
	cmd.AddCommand(newDaemonReloadCmd())

	return cmd
}

// newDaemonStartCmd returns the `kas daemon start` subcommand.
func newDaemonStartCmd() *cobra.Command {
	var foreground bool
	var configPath string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "start the kasmos daemon",
		Long:  "start the kasmos multi-repo orchestration daemon. by default it daemonizes; use --foreground for systemd.",
		RunE: func(cmd *cobra.Command, args []string) error {
			execPath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve executable: %w", err)
			}

			if foreground {
				// Foreground mode: re-exec self with the hidden --run-daemon-foreground
				// flag. main.go intercepts this flag and runs the new multi-repo daemon
				// directly (avoiding a circular import between cmd and daemon packages).
				fgArgs := []string{execPath, "--run-daemon-foreground"}
				if configPath != "" {
					fgArgs = append(fgArgs, "--daemon-config", configPath)
				}
				proc, err := os.StartProcess(execPath, fgArgs, &os.ProcAttr{
					Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
				})
				if err != nil {
					return fmt.Errorf("start daemon foreground: %w", err)
				}
				state, err := proc.Wait()
				if err != nil {
					return fmt.Errorf("daemon foreground exited: %w", err)
				}
				if !state.Success() {
					return fmt.Errorf("daemon foreground exited with code %d", state.ExitCode())
				}
				return nil
			}

			// Daemonize: fork-and-exec self with --run-daemon-foreground and detach.
			childArgs := []string{execPath, "--run-daemon-foreground"}
			if configPath != "" {
				childArgs = append(childArgs, "--daemon-config", configPath)
			}

			pidDir := filepath.Dir(daemonPIDPath())
			if err := os.MkdirAll(pidDir, 0o755); err != nil {
				return fmt.Errorf("create daemon dir: %w", err)
			}

			proc, err := os.StartProcess(execPath, childArgs, &os.ProcAttr{
				Files: []*os.File{nil, nil, nil},
				Sys:   daemonSysProcAttr(),
			})
			if err != nil {
				return fmt.Errorf("start daemon process: %w", err)
			}

			// Write PID file.
			pidContent := fmt.Sprintf("%d\n", proc.Pid)
			if werr := os.WriteFile(daemonPIDPath(), []byte(pidContent), 0o644); werr != nil {
				// Non-fatal: process is already running.
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to write PID file: %v\n", werr)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "daemon started (pid=%d)\n", proc.Pid)
			return proc.Release()
		},
	}

	cmd.Flags().BoolVar(&foreground, "foreground", false, "run in foreground (for systemd / direct invocation)")
	cmd.Flags().StringVar(&configPath, "config", "", "path to daemon TOML config file")
	return cmd
}

// newDaemonStopCmd returns the `kas daemon stop` subcommand.
func newDaemonStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "stop the running daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := stopDaemonByPID(daemonPIDPath()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "daemon stopped")
			return nil
		},
	}
}

// stopDaemonByPID reads the PID file at path and sends SIGTERM to the process.
func stopDaemonByPID(pidPath string) error {
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("daemon not running (no PID file)")
		}
		return fmt.Errorf("read PID file: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(raw), "%d", &pid); err != nil {
		return fmt.Errorf("malformed PID file: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal process %d: %w", pid, err)
	}

	_ = os.Remove(pidPath)
	return nil
}

// newDaemonStatusCmd returns the `kas daemon status` subcommand.
// socketPath is accepted as a pointer so parent flag binding updates are
// observed at execution time while tests can still inject a fixed value.
func newDaemonStatusCmd(socketPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := daemonHTTPClient(*socketPath)

			resp, err := client.Get("http://kas/v1/status")
			if err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "daemon not running")
				return fmt.Errorf("daemon not running: %w", err)
			}
			defer resp.Body.Close()

			var status api.StatusResponse
			if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
				return fmt.Errorf("decode status: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "daemon status:")
			if len(status.Repos) == 0 {
				fmt.Fprintln(out, "  no repos registered")
			} else {
				for _, r := range status.Repos {
					fmt.Fprintf(out, "  - %s (%s) [%d active plans]\n", r.Project, r.Path, r.ActivePlans)
				}
			}
			return nil
		},
	}
}

// newDaemonAddCmd returns the `kas daemon add <repo-path>` subcommand.
func newDaemonAddCmd(socketPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "add <repo-path>",
		Short: "register a repository with the daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}

			body, _ := json.Marshal(map[string]string{"path": repoPath})
			resp, err := daemonPost(*socketPath, "/v1/repos", body)
			if err != nil {
				return fmt.Errorf("daemon not running: %w", err)
			}
			defer resp.Body.Close()

			// Server returns 201 Created on success; treat any 2xx as success.
			if resp.StatusCode >= 300 {
				return fmt.Errorf("add repo failed (status %d)", resp.StatusCode)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "registered repo: %s\n", repoPath)
			return nil
		},
	}
}

// newDaemonRemoveCmd returns the `kas daemon remove <repo-path>` subcommand.
func newDaemonRemoveCmd(socketPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <repo-path>",
		Short: "unregister a repository from the daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}

			// The API identifies repos by project name (basename of path).
			project := filepath.Base(repoPath)

			// Issue DELETE /v1/repos/{project} — no request body needed.
			client := daemonHTTPClient(*socketPath)
			req, err := http.NewRequest(http.MethodDelete, "http://kas/v1/repos/"+project, nil)
			if err != nil {
				return fmt.Errorf("build request: %w", err)
			}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("daemon not running: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("remove repo failed (status %d)", resp.StatusCode)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "unregistered repo: %s\n", repoPath)
			return nil
		},
	}
}

// newDaemonReloadCmd returns the `kas daemon reload` subcommand.
func newDaemonReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "reload daemon configuration (sends SIGHUP)",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := os.ReadFile(daemonPIDPath())
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("daemon not running")
				}
				return fmt.Errorf("read PID file: %w", err)
			}

			var pid int
			if _, err := fmt.Sscanf(string(raw), "%d", &pid); err != nil {
				return fmt.Errorf("malformed PID file: %w", err)
			}

			proc, err := os.FindProcess(pid)
			if err != nil {
				return fmt.Errorf("find process %d: %w", pid, err)
			}

			if err := proc.Signal(syscall.SIGHUP); err != nil {
				return fmt.Errorf("signal process %d: %w", pid, err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "daemon reloaded")
			return nil
		},
	}
}

// daemonHTTPClient returns an http.Client configured to connect to the daemon's
// Unix domain socket at the given path.
func daemonHTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
		Timeout: 3 * time.Second,
	}
}

// daemonPost sends a POST request to the daemon control socket.
func daemonPost(socketPath, endpoint string, body []byte) (*http.Response, error) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}
	return client.Post("http://kas"+endpoint, "application/json", bytes.NewReader(body))
}
