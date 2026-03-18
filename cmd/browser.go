package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type httpGetter interface {
	Get(string) (*http.Response, error)
}

const (
	defaultBrowserBind = "127.0.0.1"
	defaultBrowserPort = 7433
)

var (
	browserHTTPClient httpGetter = &http.Client{Timeout: 500 * time.Millisecond}
	browserOpenURL               = openURL
	browserExecutable            = os.Executable
	browserStartServe            = startPlanBrowserServer
	browserWaitReady             = waitForPlanBrowserReady
)

// OpenPlanBrowser starts or reuses kas serve and opens the plan browser.
// When planFile is non-empty it opens the task detail page for that task.
func OpenPlanBrowser(repoRoot, project, planFile string) (string, bool, error) {
	return openPlanBrowser(repoRoot, project, planFile, defaultBrowserBind, defaultBrowserPort, "")
}

// NewBrowserCmd returns `kas browser`, which starts or reuses kas serve and
// opens the plan browser in the default browser.
func NewBrowserCmd() *cobra.Command {
	var (
		bind     string
		port     int
		project  string
		adminDir string
	)

	cmd := &cobra.Command{
		Use:   "browser [task-file]",
		Short: "open the admin plan browser",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := filepath.Abs(".")
			if err != nil {
				return fmt.Errorf("resolve repo path: %w", err)
			}

			planFile := ""
			if len(args) == 1 {
				planFile = args[0]
			}

			projectName := project
			if projectName == "" {
				projectName = filepath.Base(repoRoot)
			}

			openedURL, started, err := openPlanBrowser(repoRoot, projectName, planFile, bind, port, adminDir)
			if err != nil {
				return err
			}

			if started {
				fmt.Printf("started kas serve and opened %s\n", openedURL)
			} else {
				fmt.Printf("opened %s\n", openedURL)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&bind, "bind", defaultBrowserBind, "address to bind kas serve to when browser starts it")
	cmd.Flags().IntVar(&port, "port", defaultBrowserPort, "port to use for the browser server")
	cmd.Flags().StringVar(&project, "project", "", "project name to open (defaults to current directory name)")
	cmd.Flags().StringVar(&adminDir, "admin-dir", "", "path to the built admin SPA dist/ directory (passed through when browser starts kas serve)")

	return cmd
}

func openPlanBrowser(repoRoot, project, planFile, bind string, port int, adminDir string) (string, bool, error) {
	baseURL := browserBaseURL(bind, port)
	if project == "" {
		project = filepath.Base(repoRoot)
	}

	started := false
	if !planBrowserReachable(baseURL) {
		if err := browserStartServe(repoRoot, bind, port, adminDir); err != nil {
			return "", false, fmt.Errorf("start plan browser server: %w", err)
		}
		started = true
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := browserWaitReady(ctx, baseURL); err != nil {
			return "", true, fmt.Errorf("wait for plan browser server: %w", err)
		}
	}

	openedURL := planBrowserURL(baseURL, project, planFile)
	if err := browserOpenURL(openedURL); err != nil {
		return "", started, fmt.Errorf("open plan browser: %w", err)
	}

	return openedURL, started, nil
}

func browserBaseURL(bind string, port int) string {
	host := bind
	switch bind {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

func planBrowserURL(baseURL, project, planFile string) string {
	basePath := "/admin/"
	if planFile != "" {
		basePath = "/admin/tasks/" + url.PathEscape(planFile)
	}
	q := url.Values{}
	if project != "" {
		q.Set("project", project)
	}
	if encoded := q.Encode(); encoded != "" {
		return strings.TrimRight(baseURL, "/") + basePath + "?" + encoded
	}
	return strings.TrimRight(baseURL, "/") + basePath
}

func planBrowserReachable(baseURL string) bool {
	resp, err := browserHTTPClient.Get(baseURL + "/v1/ping")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func waitForPlanBrowserReady(ctx context.Context, baseURL string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if planBrowserReachable(baseURL) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func startPlanBrowserServer(repoRoot, bind string, port int, adminDir string) error {
	exe, err := browserExecutable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	args := []string{"serve", "--bind", bind, "--port", strconv.Itoa(port), "--mcp=false"}
	if adminDir != "" {
		args = append(args, "--admin-dir", adminDir)
	}

	cmd := exec.Command(exe, args...)
	cmd.Dir = repoRoot

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer devNull.Close()

	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start kas serve: %w", err)
	}
	return nil
}
