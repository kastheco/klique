package session

import (
	"testing"

	"github.com/kastheco/kasmos/internal/opencodesession"
	"github.com/stretchr/testify/assert"
)

func TestBuildTitleOptsFromInstance(t *testing.T) {
	tests := []struct {
		name string
		inst *Instance
		want string
	}{
		{
			name: "planner with plan file",
			inst: &Instance{
				PlanFile:  "2026-03-02-automatic-session-naming.md",
				AgentType: AgentTypePlanner,
				Title:     "automatic-session-naming-plan",
			},
			want: "kas: plan automatic-session-naming",
		},
		{
			name: "coder wave task",
			inst: &Instance{
				PlanFile:   "2026-03-02-automatic-session-naming.md",
				AgentType:  AgentTypeCoder,
				WaveNumber: 2,
				TaskNumber: 3,
				Title:      "automatic-session-naming-W2-T3",
			},
			want: "kas: implement automatic-session-naming w2/t3",
		},
		{
			name: "reviewer",
			inst: &Instance{
				PlanFile:  "2026-03-02-automatic-session-naming.md",
				AgentType: AgentTypeReviewer,
				Title:     "automatic-session-naming-review",
			},
			want: "kas: review automatic-session-naming",
		},
		{
			name: "fixer ad-hoc",
			inst: &Instance{
				AgentType: AgentTypeFixer,
				Title:     "fix-login-bug",
			},
			want: "kas: fix fix-login-bug",
		},
		{
			name: "ad-hoc no agent type",
			inst: &Instance{
				Title: "my-session",
			},
			want: "kas: my-session",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := buildTitleOpts(tt.inst)
			got := opencodesession.BuildTitle(opts)
			assert.Equal(t, tt.want, got)
		})
	}
}
