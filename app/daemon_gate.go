package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"time"

	daemonpkg "github.com/kastheco/kasmos/daemon"
	"github.com/kastheco/kasmos/daemon/api"
	"github.com/kastheco/kasmos/ui/overlay"

	tea "charm.land/bubbletea/v2"
)

type daemonStatusMsg struct {
	ready   bool
	message string
}

func checkDaemonStatus(repoPath string) daemonStatusMsg {
	socketPath := daemonpkg.DefaultSocketPath()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			d.Timeout = 300 * time.Millisecond
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 500 * time.Millisecond}

	resp, err := client.Get("http://daemon/v1/status")
	if err != nil {
		return daemonStatusMsg{
			message: fmt.Sprintf(
				"agent workflows require the kasmos daemon.\n\nstart it in another shell:\n  kas daemon start\n\nthen register this repo:\n  kas daemon add %s",
				repoPath,
			),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return daemonStatusMsg{
			message: fmt.Sprintf(
				"agent workflows require the kasmos daemon, but the daemon status check failed.\n\nstart it in another shell:\n  kas daemon start\n\nthen register this repo:\n  kas daemon add %s",
				repoPath,
			),
		}
	}

	var status api.StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return daemonStatusMsg{
			message: fmt.Sprintf(
				"agent workflows require the kasmos daemon, but its status response could not be read.\n\nstart it in another shell:\n  kas daemon start\n\nthen register this repo:\n  kas daemon add %s",
				repoPath,
			),
		}
	}

	cleanRepoPath := filepath.Clean(repoPath)
	for _, repo := range status.Repos {
		if filepath.Clean(repo.Path) == cleanRepoPath {
			return daemonStatusMsg{ready: true}
		}
	}

	return daemonStatusMsg{
		message: fmt.Sprintf(
			"the kasmos daemon is running, but this repo is not registered.\n\nregister it with:\n  kas daemon add %s",
			repoPath,
		),
	}
}

func (m *home) daemonStartupCheckCmd() tea.Cmd {
	if m.daemonStatusChecker == nil {
		return nil
	}
	repoPath := m.activeRepoPath
	checker := m.daemonStatusChecker
	return func() tea.Msg {
		return checker(repoPath)
	}
}

func (m *home) requireDaemonForAgents() bool {
	if m.daemonStatusChecker == nil {
		return true
	}
	status := m.daemonStatusChecker(m.activeRepoPath)
	if status.ready {
		return true
	}
	m.showDaemonRequiredDialog(status.message)
	return false
}

func (m *home) showDaemonRequiredDialog(message string) {
	if m.overlays == nil {
		m.overlays = overlay.NewManager()
	}
	m.state = stateConfirm
	m.pendingConfirmAction = nil
	co := overlay.NewConfirmationOverlay(message)
	co.SetSize(76, 0)
	m.overlays.Show(co)
}
