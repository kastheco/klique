package tmux

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/kastheco/kasmos/cmd"
)

// OpenPopup opens a tmux popup running the provided command in repoRoot.
func OpenPopup(ex cmd.Executor, repoRoot, title string, commandArgs ...string) error {
	args := []string{"display-popup", "-E", "-w", "80%", "-h", "80%"}
	if title != "" {
		args = append(args, "-T", title)
	}
	if repoRoot != "" {
		args = append(args, "-d", repoRoot)
	}
	args = append(args, shellQuoteArgs(commandArgs...))
	return ex.Run(exec.Command("tmux", args...))
}

func shellQuoteArgs(args ...string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, strconv.Quote(arg))
	}
	return strings.Join(quoted, " ")
}
