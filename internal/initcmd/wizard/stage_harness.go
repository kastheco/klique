package wizard

import (
	"fmt"

	"github.com/charmbracelet/huh"
)

func runHarnessStage(state *State) error {
	// Build options from detection results
	var options []huh.Option[string]
	var preSelected []string

	for _, d := range state.DetectResults {
		label := d.Name
		if d.Found {
			label = fmt.Sprintf("%s  (detected: %s)", d.Name, d.Path)
			preSelected = append(preSelected, d.Name)
		} else {
			label = fmt.Sprintf("%s  (not found)", d.Name)
		}
		options = append(options, huh.NewOption(label, d.Name))
	}

	state.SelectedHarness = preSelected

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("which agent harnesses do you want to configure?").
				Options(options...).
				Value(&state.SelectedHarness),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return fmt.Errorf("harness selection: %w", err)
	}

	if len(state.SelectedHarness) == 0 {
		return fmt.Errorf("no harnesses selected")
	}

	return nil
}
