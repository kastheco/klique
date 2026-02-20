package harness

// AgentConfig holds the wizard-collected configuration for one agent role.
type AgentConfig struct {
	Role        string   // "coder", "reviewer", "planner", or custom
	Harness     string   // "claude", "opencode", "codex"
	Model       string
	Temperature *float64 // nil = harness default
	Effort      string   // "" = harness default
	Enabled     bool
	ExtraFlags  []string
}

// Harness defines the interface each supported CLI adapter must implement.
type Harness interface {
	Name() string
	Detect() (path string, found bool)
	ListModels() ([]string, error)
	BuildFlags(agent AgentConfig) []string
	InstallSuperpowers() error
	ScaffoldProject(dir string, agents []AgentConfig, force bool) error
	SupportsTemperature() bool
	SupportsEffort() bool
}

// Registry holds all known harness adapters keyed by name.
type Registry struct {
	harnesses map[string]Harness
}

// NewRegistry creates a registry with all built-in harness adapters.
func NewRegistry() *Registry {
	r := &Registry{harnesses: make(map[string]Harness)}
	r.Register(&Claude{})
	r.Register(&OpenCode{})
	r.Register(&Codex{})
	return r
}

// Register adds a harness adapter to the registry.
func (r *Registry) Register(h Harness) {
	r.harnesses[h.Name()] = h
}

// Get returns the harness adapter for the given name, or nil.
func (r *Registry) Get(name string) Harness {
	return r.harnesses[name]
}

// All returns all registered harness names in stable order.
func (r *Registry) All() []string {
	return []string{"claude", "opencode", "codex"}
}

// DetectResult holds the result of detecting a single harness.
type DetectResult struct {
	Name  string
	Path  string
	Found bool
}

// DetectAll probes each harness and returns detection results.
func (r *Registry) DetectAll() []DetectResult {
	var results []DetectResult
	for _, name := range r.All() {
		h := r.harnesses[name]
		path, found := h.Detect()
		results = append(results, DetectResult{Name: name, Path: path, Found: found})
	}
	return results
}
