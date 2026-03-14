package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kastheco/kasmos/config"
	"github.com/kastheco/kasmos/daemon"
	"github.com/kastheco/kasmos/session"
	"github.com/spf13/cobra"
)

func newPopupCmd() *cobra.Command {
	popupCmd := &cobra.Command{
		Use:    "popup",
		Short:  "internal tmux popup actions",
		Hidden: true,
	}

	popupCmd.AddCommand(newPopupNewPlanCmd())
	popupCmd.AddCommand(newPopupSpawnAgentCmd())
	popupCmd.AddCommand(newPopupSendPromptCmd())

	return popupCmd
}

func newPopupNewPlanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "new-plan",
		Short: "create a new plan from a tmux popup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := popupPromptLine("plan title", true)
			if err != nil {
				return err
			}
			topic, err := popupPromptLine("topic (optional)", false)
			if err != nil {
				return err
			}
			description, err := popupPromptMultiline("plan description", "enter description, then press ctrl+d to submit")
			if err != nil {
				return err
			}
			if description == "" {
				return fmt.Errorf("description cannot be empty")
			}
			content := popupPlanStub(name, description)
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			createArgs := []string{"task", "create", name, "--description", description, "--content", content}
			if topic != "" {
				createArgs = append(createArgs, "--topic", topic)
			}
			runCmd := exec.Command(exe, createArgs...)
			runCmd.Stdout = os.Stdout
			runCmd.Stderr = os.Stderr
			return runCmd.Run()
		},
	}
}

func newPopupSpawnAgentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "spawn-agent",
		Short: "spawn an ad-hoc fixer agent from a tmux popup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := popupRepoRoot()
			if err != nil {
				return err
			}
			if err := ensureDaemonRepoRegistered(repoRoot); err != nil {
				return err
			}
			name, err := popupPromptLine("agent name", true)
			if err != nil {
				return err
			}
			branch, err := popupPromptLine("branch override (optional)", false)
			if err != nil {
				return err
			}
			workPath, err := popupPromptLine("worktree path (optional)", false)
			if err != nil {
				return err
			}
			return popupSpawnAgent(repoRoot, name, branch, workPath)
		},
	}
}

func newPopupSendPromptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "send-prompt <instance-title>",
		Short: "send a prompt to an instance from a tmux popup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]
			prompt, err := popupPromptMultiline(
				fmt.Sprintf("prompt for %s", title),
				"enter prompt text, then press ctrl+d to submit",
			)
			if err != nil {
				return err
			}
			if prompt == "" {
				return fmt.Errorf("prompt cannot be empty")
			}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			sendCmd := exec.Command(exe, "instance", "send", title, prompt)
			sendCmd.Stdout = os.Stdout
			sendCmd.Stderr = os.Stderr
			return sendCmd.Run()
		},
	}
}

func popupPromptLine(label string, required bool) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s: ", label)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimSpace(line)
	if required && line == "" {
		return "", fmt.Errorf("%s cannot be empty", label)
	}
	return line, nil
}

func popupPromptMultiline(label, hint string) (string, error) {
	if label != "" {
		fmt.Println(label)
	}
	if hint != "" {
		fmt.Println(hint)
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func popupRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return config.ResolveRepoRoot(cwd)
}

func popupPlanStub(name, description string) string {
	filename := popupPlanFilename(name)
	return fmt.Sprintf("# %s\n\n## Context\n\n%s\n\n## Notes\n\n- Created by kas lifecycle flow\n- Plan file: %s\n", name, description, filename)
}

func popupPlanFilename(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.NewReplacer("/", "-", "_", "-", ".", "-").Replace(name)
	fields := strings.FieldsFunc(name, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	if len(fields) == 0 {
		return "plan"
	}
	return strings.Join(fields, "-")
}

func popupSpawnAgent(repoRoot, name, branch, workPath string) error {
	path := repoRoot
	if workPath != "" {
		path = workPath
	}
	cfg := config.LoadConfig()
	profile := cfg.ResolveProfile("fixer", cfg.DefaultProgram)
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:         name,
		Path:          path,
		Program:       profile.BuildCommand(),
		ExecutionMode: session.ExecutionModeTmux,
		AutoYes:       cfg.AutoYes,
	})
	if err != nil {
		return err
	}
	inst.AgentType = session.AgentTypeFixer
	inst.LoadingTotal = 8
	inst.LoadingMessage = "preparing session..."
	inst.SetStatus(session.Loading)
	if branch != "" {
		err = inst.StartOnBranch(branch)
	} else {
		err = inst.StartOnMainBranch()
	}
	if err != nil {
		return err
	}
	storage, err := session.NewStorage(config.LoadState())
	if err != nil {
		return err
	}
	existing, err := storage.LoadInstances()
	if err != nil {
		return err
	}
	for _, item := range existing {
		if item.Title == inst.Title {
			return nil
		}
	}
	existing = append(existing, inst)
	return storage.SaveInstances(existing)
}

func ensureDaemonRepoRegistered(repoRoot string) error {
	client := daemon.NewSocketClient(daemon.DefaultSocketPath())
	status, err := client.Status()
	if err != nil {
		return fmt.Errorf("agent workflows require the kasmos daemon; start it with `kas daemon start`")
	}
	cleanRoot := filepath.Clean(repoRoot)
	for _, repo := range status.Repos {
		if filepath.Clean(repo.Path) == cleanRoot {
			return nil
		}
	}
	return client.AddRepo(repoRoot)
}
