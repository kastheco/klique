package planstore

// NewStoreFromConfig creates a Store from a plan store URL and project name.
// If planStoreURL is empty, it returns (nil, nil) — the caller should fall
// back to legacy plan-state.json behavior.
// The returned store uses lazy connection: the URL is validated syntactically
// but no network connection is made until the first operation (or Ping).
func NewStoreFromConfig(planStoreURL, project string) (Store, error) {
	if planStoreURL == "" {
		return nil, nil // no remote store configured
	}
	return NewHTTPStore(planStoreURL, project), nil
}
