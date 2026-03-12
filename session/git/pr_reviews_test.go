package git

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	cmd_test "github.com/kastheco/kasmos/cmd/cmd_test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withMockGHExec replaces the package-level ghExec for the duration of the test
// and restores the original executor when the test finishes.
func withMockGHExec(t *testing.T, mock ghExecutor) {
	t.Helper()
	orig := ghExec
	ghExec = mock
	t.Cleanup(func() { ghExec = orig })
}

// repoViewJSON returns the standard fake gh-repo-view output.
const repoViewJSON = `{"nameWithOwner":"acme/widgets"}`

// seqMock builds a MockCmdExec whose OutputFunc cycles through outputs in order.
// If more calls arrive than outputs provided the test fails.
func seqOutputMock(t *testing.T, outputs ...func(*exec.Cmd) ([]byte, error)) *cmd_test.MockCmdExec {
	t.Helper()
	idx := 0
	return &cmd_test.MockCmdExec{
		OutputFunc: func(c *exec.Cmd) ([]byte, error) {
			if idx >= len(outputs) {
				t.Fatalf("unexpected gh Output call #%d (args: %v)", idx, c.Args)
			}
			fn := outputs[idx]
			idx++
			return fn(c)
		},
		RunFunc: func(c *exec.Cmd) error {
			t.Fatalf("unexpected gh Run call (args: %v)", c.Args)
			return nil
		},
	}
}

// seqMixedMock builds a MockCmdExec where Output and Run each have their own
// sequential response list.
func seqMixedMock(
	t *testing.T,
	outputs []func(*exec.Cmd) ([]byte, error),
	runs []func(*exec.Cmd) error,
) *cmd_test.MockCmdExec {
	t.Helper()
	oIdx := 0
	rIdx := 0
	return &cmd_test.MockCmdExec{
		OutputFunc: func(c *exec.Cmd) ([]byte, error) {
			if oIdx >= len(outputs) {
				t.Fatalf("unexpected gh Output call #%d (args: %v)", oIdx, c.Args)
			}
			fn := outputs[oIdx]
			oIdx++
			return fn(c)
		},
		RunFunc: func(c *exec.Cmd) error {
			if rIdx >= len(runs) {
				t.Fatalf("unexpected gh Run call #%d (args: %v)", rIdx, c.Args)
			}
			fn := runs[rIdx]
			rIdx++
			return fn(c)
		},
	}
}

// staticOutput returns an OutputFunc that always returns the given bytes.
func staticOutput(data string) func(*exec.Cmd) ([]byte, error) {
	return func(*exec.Cmd) ([]byte, error) { return []byte(data), nil }
}

// errOutput returns an OutputFunc that always returns an error.
func errOutput(msg string) func(*exec.Cmd) ([]byte, error) {
	return func(*exec.Cmd) ([]byte, error) { return nil, fmt.Errorf("%s", msg) }
}

// ─── ExtractPRNumber ─────────────────────────────────────────────────────────

func TestExtractPRNumber(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    int
		wantErr string
	}{
		{
			name: "standard github url",
			url:  "https://github.com/acme/widgets/pull/42",
			want: 42,
		},
		{
			name: "with trailing slash",
			url:  "https://github.com/org/repo/pull/7/",
			// path has extra segment after number so format check fails
			wantErr: "unexpected format",
		},
		{
			name: "enterprise github url",
			url:  "https://github.example.com/acme/widgets/pull/99",
			want: 99,
		},
		{
			name:    "empty url",
			url:     "",
			wantErr: "must not be empty",
		},
		{
			name:    "no pull segment",
			url:     "https://github.com/acme/widgets/issues/3",
			wantErr: "unexpected format",
		},
		{
			name:    "non-integer pr number",
			url:     "https://github.com/acme/widgets/pull/abc",
			wantErr: "not an integer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := ExtractPRNumber(tt.url)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, n)
		})
	}
}

// ─── ExtractOwnerRepo ────────────────────────────────────────────────────────

func TestExtractOwnerRepo(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		outputErr  error
		wantOwner  string
		wantRepo   string
		wantErrStr string
	}{
		{
			name:      "valid owner/repo",
			output:    `{"nameWithOwner":"acme/widgets"}`,
			wantOwner: "acme",
			wantRepo:  "widgets",
		},
		{
			name:       "malformed json",
			output:     `not-json`,
			wantErrStr: "unmarshal",
		},
		{
			name:       "missing slash in nameWithOwner",
			output:     `{"nameWithOwner":"noslash"}`,
			wantErrStr: "unexpected nameWithOwner format",
		},
		{
			name:       "empty nameWithOwner",
			output:     `{"nameWithOwner":""}`,
			wantErrStr: "unexpected nameWithOwner format",
		},
		{
			name:       "gh command error",
			outputErr:  fmt.Errorf("authentication required"),
			wantErrStr: "extract owner/repo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &cmd_test.MockCmdExec{
				OutputFunc: func(c *exec.Cmd) ([]byte, error) {
					if tt.outputErr != nil {
						return nil, tt.outputErr
					}
					return []byte(tt.output), nil
				},
				RunFunc: func(c *exec.Cmd) error { return nil },
			}
			withMockGHExec(t, mock)

			owner, repo, err := ExtractOwnerRepo("/fake/repo")
			if tt.wantErrStr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrStr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}

func TestExtractOwnerRepo_EmptyRepoPath(t *testing.T) {
	_, _, err := ExtractOwnerRepo("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

// ─── ListPRReviews ───────────────────────────────────────────────────────────

func TestListPRReviews(t *testing.T) {
	const reviewsJSON = `[
		{"id":10,"state":"APPROVED","body":"lgtm","user":{"login":"bob"},"submitted_at":"2024-01-15T12:00:00Z"},
		{"id":11,"state":"CHANGES_REQUESTED","body":"fix nits","user":{"login":"alice"},"submitted_at":"2024-01-16T09:30:00Z"},
		{"id":12,"state":"COMMENTED","body":"","user":{"login":"carol"},"submitted_at":""}
	]`

	mock := seqOutputMock(t,
		staticOutput(repoViewJSON),
		staticOutput(reviewsJSON),
	)
	withMockGHExec(t, mock)

	reviews, err := ListPRReviews("/fake/repo", 5)
	require.NoError(t, err)
	require.Len(t, reviews, 3)

	assert.Equal(t, 10, reviews[0].ID)
	assert.Equal(t, "APPROVED", reviews[0].State)
	assert.Equal(t, "lgtm", reviews[0].Body)
	assert.Equal(t, "bob", reviews[0].User)
	wantTime, _ := time.Parse(time.RFC3339, "2024-01-15T12:00:00Z")
	assert.Equal(t, wantTime, reviews[0].SubmittedAt)

	assert.Equal(t, 11, reviews[1].ID)
	assert.Equal(t, "CHANGES_REQUESTED", reviews[1].State)
	assert.Equal(t, "alice", reviews[1].User)

	// Empty submitted_at should result in zero time, not an error.
	assert.Equal(t, 12, reviews[2].ID)
	assert.True(t, reviews[2].SubmittedAt.IsZero(), "expected zero time for empty submitted_at")
}

func TestListPRReviews_EmptyList(t *testing.T) {
	mock := seqOutputMock(t,
		staticOutput(repoViewJSON),
		staticOutput(`[]`),
	)
	withMockGHExec(t, mock)

	reviews, err := ListPRReviews("/fake/repo", 1)
	require.NoError(t, err)
	assert.Empty(t, reviews)
}

func TestListPRReviews_InvalidArgs(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		prNum   int
		wantErr string
	}{
		{"empty repoPath", "", 1, "must not be empty"},
		{"zero pr number", "/repo", 0, "invalid pr number"},
		{"negative pr number", "/repo", -1, "invalid pr number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ListPRReviews(tt.path, tt.prNum)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestListPRReviews_APIError(t *testing.T) {
	mock := seqOutputMock(t,
		staticOutput(repoViewJSON),
		errOutput("HTTP 500"),
	)
	withMockGHExec(t, mock)

	_, err := ListPRReviews("/fake/repo", 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list pr reviews")
}

// ─── ListReviewComments ──────────────────────────────────────────────────────

func TestListReviewComments(t *testing.T) {
	const commentsJSON = `[{"id":101},{"id":102},{"id":103}]`

	var capturedEndpoint string
	mock := seqOutputMock(t,
		staticOutput(repoViewJSON),
		func(c *exec.Cmd) ([]byte, error) {
			// Record the endpoint argument (last arg after "api").
			capturedEndpoint = c.Args[len(c.Args)-1]
			return []byte(commentsJSON), nil
		},
	)
	withMockGHExec(t, mock)

	comments, err := ListReviewComments("/fake/repo", 5, 10)
	require.NoError(t, err)
	require.Len(t, comments, 3)
	assert.Equal(t, 101, comments[0].ID)
	assert.Equal(t, 102, comments[1].ID)
	assert.Equal(t, 103, comments[2].ID)

	assert.Contains(t, capturedEndpoint, "pulls/5/reviews/10/comments")
}

func TestListReviewComments_EmptyList(t *testing.T) {
	mock := seqOutputMock(t,
		staticOutput(repoViewJSON),
		staticOutput(`[]`),
	)
	withMockGHExec(t, mock)

	comments, err := ListReviewComments("/fake/repo", 1, 1)
	require.NoError(t, err)
	assert.Empty(t, comments)
}

func TestListReviewComments_InvalidArgs(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		prNum   int
		wantErr string
	}{
		{"empty repoPath", "", 1, "must not be empty"},
		{"zero pr number", "/repo", 0, "invalid pr number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ListReviewComments(tt.path, tt.prNum, 1)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// ─── AddReviewReaction ───────────────────────────────────────────────────────

func TestAddReviewReaction(t *testing.T) {
	var capturedArgs []string

	mock := seqMixedMock(t,
		// Output calls: ExtractOwnerRepo
		[]func(*exec.Cmd) ([]byte, error){
			staticOutput(repoViewJSON),
		},
		// Run calls: AddReviewReaction POST
		[]func(*exec.Cmd) error{
			func(c *exec.Cmd) error {
				capturedArgs = c.Args
				return nil
			},
		},
	)
	withMockGHExec(t, mock)

	err := AddReviewReaction("/fake/repo", 42, "eyes")
	require.NoError(t, err)

	// Verify the gh api invocation includes all required arguments.
	joined := fmt.Sprintf("%v", capturedArgs)
	assert.Contains(t, joined, "api")
	assert.Contains(t, joined, "--method")
	assert.Contains(t, joined, "POST")
	assert.Contains(t, joined, "Accept:application/vnd.github+json")
	assert.Contains(t, joined, "pulls/comments/42/reactions")
	assert.Contains(t, joined, "content=eyes")
}

func TestAddReviewReaction_InvalidArgs(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		commentID int
		reaction  string
		wantErr   string
	}{
		{"empty repoPath", "", 1, "eyes", "must not be empty"},
		{"zero comment id", "/repo", 0, "eyes", "invalid comment id"},
		{"negative comment id", "/repo", -5, "eyes", "invalid comment id"},
		{"empty reaction", "/repo", 1, "", "must not be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AddReviewReaction(tt.path, tt.commentID, tt.reaction)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// ─── PROpen ──────────────────────────────────────────────────────────────────

func TestPROpen(t *testing.T) {
	tests := []struct {
		name     string
		apiJSON  string
		apiErr   error
		wantOpen bool
		wantErr  string
	}{
		{
			name:     "open pr",
			apiJSON:  `{"state":"open","merged_at":null}`,
			wantOpen: true,
		},
		{
			name:     "closed pr",
			apiJSON:  `{"state":"closed","merged_at":null}`,
			wantOpen: false,
		},
		{
			name:     "merged pr",
			apiJSON:  `{"state":"closed","merged_at":"2024-03-01T08:00:00Z"}`,
			wantOpen: false,
		},
		{
			name:     "404 treated as missing",
			apiErr:   fmt.Errorf("HTTP 404: Not Found"),
			wantOpen: false,
		},
		{
			name:    "other api error",
			apiErr:  fmt.Errorf("HTTP 500: Internal Server Error"),
			wantErr: "pr open check",
		},
		{
			name:    "malformed json",
			apiJSON: `not-json`,
			wantErr: "unmarshal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outputs []func(*exec.Cmd) ([]byte, error)
			outputs = append(outputs, staticOutput(repoViewJSON))
			if tt.apiErr != nil {
				outputs = append(outputs, func(*exec.Cmd) ([]byte, error) {
					return nil, tt.apiErr
				})
			} else {
				outputs = append(outputs, staticOutput(tt.apiJSON))
			}

			mock := seqOutputMock(t, outputs...)
			withMockGHExec(t, mock)

			open, err := PROpen("/fake/repo", 7)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOpen, open)
		})
	}
}

func TestPROpen_InvalidArgs(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		prNum   int
		wantErr string
	}{
		{"empty repoPath", "", 1, "must not be empty"},
		{"zero pr number", "/repo", 0, "invalid pr number"},
		{"negative pr number", "/repo", -3, "invalid pr number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PROpen(tt.path, tt.prNum)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// ─── API endpoint construction ───────────────────────────────────────────────

func TestListPRReviews_EndpointConstruction(t *testing.T) {
	var capturedEndpoint string
	mock := seqOutputMock(t,
		staticOutput(`{"nameWithOwner":"myorg/myrepo"}`),
		func(c *exec.Cmd) ([]byte, error) {
			capturedEndpoint = c.Args[len(c.Args)-1]
			return []byte(`[]`), nil
		},
	)
	withMockGHExec(t, mock)

	_, err := ListPRReviews("/some/path", 99)
	require.NoError(t, err)
	assert.Equal(t, "repos/myorg/myrepo/pulls/99/reviews?per_page=100", capturedEndpoint)
}

func TestAddReviewReaction_EndpointConstruction(t *testing.T) {
	var capturedArgs string
	mock := seqMixedMock(t,
		[]func(*exec.Cmd) ([]byte, error){
			staticOutput(`{"nameWithOwner":"myorg/myrepo"}`),
		},
		[]func(*exec.Cmd) error{
			func(c *exec.Cmd) error {
				capturedArgs = strings.Join(c.Args, " ")
				return nil
			},
		},
	)
	withMockGHExec(t, mock)

	err := AddReviewReaction("/some/path", 55, "hooray")
	require.NoError(t, err)
	assert.Contains(t, capturedArgs, "repos/myorg/myrepo/pulls/comments/55/reactions")
	assert.Contains(t, capturedArgs, "content=hooray")
	assert.Contains(t, capturedArgs, "--method POST")
}
