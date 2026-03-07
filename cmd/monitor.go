package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

// NewMonitorCmd returns the `kas monitor` cobra command with subcommands.
// The default behaviour (no subcommand) is a live tail of the daemon event
// stream via SSE from the control socket.
func NewMonitorCmd() *cobra.Command {
	var (
		socketPath string
		repoFilter string
		planFilter string
		jsonOutput bool
	)

	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "monitor the kasmos daemon event stream",
		Long: `monitor connects to the running kasmos daemon and streams real-time
orchestration events. by default it outputs colored ANSI text; use --json for
raw JSON suitable for piping to jq.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMonitorTail(cmd, socketPath, repoFilter, planFilter, jsonOutput)
		},
	}

	cmd.PersistentFlags().StringVar(&socketPath, "socket", daemonSocketPath(), "path to the daemon unix domain socket")
	cmd.Flags().StringVar(&repoFilter, "repo", "", "filter events to a specific repo path")
	cmd.Flags().StringVar(&planFilter, "plan", "", "filter events to a specific plan slug")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output raw JSON event stream (for piping to jq)")

	cmd.AddCommand(newMonitorStatusCmd(&socketPath))

	return cmd
}

// runMonitorTail opens the daemon SSE event stream and writes events to the
// command output until the stream is closed or the user interrupts.
func runMonitorTail(cmd *cobra.Command, socketPath, repoFilter, planFilter string, jsonOutput bool) error {
	client := daemonHTTPClient(socketPath)

	resp, err := client.Get("http://kas/events")
	if err != nil {
		return fmt.Errorf("daemon not running: %w", err)
	}
	defer resp.Body.Close()

	out := cmd.OutOrStdout()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		// SSE lines start with "data: ".
		const prefix = "data: "
		if len(line) > len(prefix) && line[:len(prefix)] == prefix {
			payload := line[len(prefix):]

			if jsonOutput {
				fmt.Fprintln(out, payload)
				continue
			}

			// Pretty-print for human consumption.
			if err := printMonitorEvent(out, payload, repoFilter, planFilter); err != nil {
				// Non-fatal: keep reading even if one event is malformed.
				fmt.Fprintf(out, "event: %s\n", payload)
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("read event stream: %w", err)
	}
	return nil
}

// printMonitorEvent pretty-prints a single SSE JSON payload.
func printMonitorEvent(out io.Writer, payload, repoFilter, planFilter string) error {
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return err
	}

	eventType, _ := event["type"].(string)
	repo, _ := event["repo"].(string)
	plan, _ := event["plan"].(string)

	// Apply filters.
	if repoFilter != "" && repo != repoFilter {
		return nil
	}
	if planFilter != "" && plan != planFilter {
		return nil
	}

	switch eventType {
	case "heartbeat":
		repos, _ := event["repos"].([]interface{})
		fmt.Fprintf(out, "\033[2m[heartbeat] %d repo(s) active\033[0m\n", len(repos))
	case "connected":
		fmt.Fprintf(out, "\033[32m[connected] monitoring daemon events\033[0m\n")
	default:
		fmt.Fprintf(out, "[%s] %s\n", eventType, payload)
	}
	return nil
}

// newMonitorStatusCmd returns the `kas monitor status` subcommand — a one-shot
// snapshot of daemon state: registered repos, active plans, running agents.
func newMonitorStatusCmd(socketPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "show a snapshot of daemon state (repos, plans, agents)",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := daemonHTTPClient(*socketPath)

			resp, err := client.Get("http://kas/status")
			if err != nil {
				return fmt.Errorf("daemon not running: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status %d from daemon", resp.StatusCode)
			}

			var status map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
				return fmt.Errorf("decode status: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "daemon status snapshot:")

			repos, _ := status["repos"].([]interface{})
			if len(repos) == 0 {
				fmt.Fprintln(out, "  repos: none")
			} else {
				fmt.Fprintln(out, "  repos:")
				for _, r := range repos {
					fmt.Fprintf(out, "    - %s\n", r)
				}
			}

			if agents, ok := status["agents"].([]interface{}); ok {
				fmt.Fprintln(out, "  agents:")
				for _, a := range agents {
					fmt.Fprintf(out, "    - %v\n", a)
				}
			}

			return nil
		},
	}
}
