package orchestration

type ArchitectMeta struct {
	PlanID          string            `json:"plan_id"`
	SchemaVersion   int               `json:"schema_version"`
	ArchitectModel  string            `json:"architect_model,omitempty"`
	ArchitectEffort string            `json:"architect_effort,omitempty"`
	Waves           []WaveMeta        `json:"waves"`
	CacheVersion    int               `json:"cache_version"`
	CachedSnippets  map[string]string `json:"cached_snippets,omitempty"`
}

type WaveMeta struct {
	Wave             int        `json:"wave"`
	Parallel         bool       `json:"parallel"`
	ConflictAnalysis string     `json:"conflict_analysis,omitempty"`
	Tasks            []TaskMeta `json:"tasks"`
}

type TaskMeta struct {
	TaskNumber        int      `json:"task_number"`
	Title             string   `json:"title"`
	PreferredModel    string   `json:"preferred_model,omitempty"`
	FallbackModel     string   `json:"fallback_model,omitempty"`
	EscalationPolicy  string   `json:"escalation_policy,omitempty"`
	EstimatedTokens   int      `json:"estimated_tokens,omitempty"`
	FilesToModify     []string `json:"files_to_modify,omitempty"`
	DependencyNumbers []int    `json:"dependency_task_numbers,omitempty"`
	VerifyChecks      []string `json:"verify_checks,omitempty"`
	ContextRefs       []string `json:"context_refs,omitempty"`
}

func (m *ArchitectMeta) TaskByNumber(num int) *TaskMeta {
	if m == nil {
		return nil
	}

	for i := range m.Waves {
		for j := range m.Waves[i].Tasks {
			if m.Waves[i].Tasks[j].TaskNumber == num {
				return &m.Waves[i].Tasks[j]
			}
		}
	}

	return nil
}
