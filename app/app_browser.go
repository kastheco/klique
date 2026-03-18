package app

import "fmt"

import tea "charm.land/bubbletea/v2"

type planBrowserOpenedMsg struct {
	url           string
	startedServer bool
}

func (m *home) openPlanBrowserForSelection() (tea.Model, tea.Cmd) {
	planFile := m.nav.GetSelectedPlanFile()
	if planFile == "" {
		if inst := m.nav.GetSelectedInstance(); inst != nil {
			planFile = inst.TaskFile
		}
	}

	m.toastManager.Info("opening plan browser...")
	return m, tea.Batch(m.toastTickCmd(), func() tea.Msg {
		if m.planBrowserOpener == nil {
			return fmt.Errorf("plan browser opener is not configured")
		}
		openedURL, startedServer, err := m.planBrowserOpener(m.activeRepoPath, m.taskStoreProject, planFile)
		if err != nil {
			return err
		}
		return planBrowserOpenedMsg{url: openedURL, startedServer: startedServer}
	})
}
