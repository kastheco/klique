package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePRViewJSON(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantURL   string
		wantRD    string
		wantCS    string
		wantDraft bool
		wantNum   int
	}{
		{
			name:    "approved with passing checks",
			json:    `{"url":"https://github.com/org/repo/pull/1","reviewDecision":"APPROVED","statusCheckRollup":{"state":"SUCCESS"},"isDraft":false,"number":1}`,
			wantURL: "https://github.com/org/repo/pull/1",
			wantRD:  "APPROVED",
			wantCS:  "SUCCESS",
			wantNum: 1,
		},
		{
			name:      "draft pr",
			json:      `{"url":"https://github.com/org/repo/pull/2","reviewDecision":"","statusCheckRollup":{"state":"PENDING"},"isDraft":true,"number":2}`,
			wantURL:   "https://github.com/org/repo/pull/2",
			wantCS:    "PENDING",
			wantDraft: true,
			wantNum:   2,
		},
		{
			name:    "no status rollup",
			json:    `{"url":"https://github.com/org/repo/pull/3","reviewDecision":"REVIEW_REQUIRED","isDraft":false,"number":3}`,
			wantURL: "https://github.com/org/repo/pull/3",
			wantRD:  "REVIEW_REQUIRED",
			wantNum: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := ParsePRViewJSON([]byte(tt.json))
			require.NoError(t, err)
			assert.Equal(t, tt.wantURL, state.URL)
			assert.Equal(t, tt.wantRD, state.ReviewDecision)
			assert.Equal(t, tt.wantCS, state.CheckStatus)
			assert.Equal(t, tt.wantDraft, state.IsDraft)
			assert.Equal(t, tt.wantNum, state.Number)
		})
	}
}

func TestParsePRViewJSON_MalformedJSON(t *testing.T) {
	_, err := ParsePRViewJSON([]byte(`{not valid json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse pr view json")
}
