package taskfsm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// WebhookHook sends a JSON-encoded TransitionEvent to a configured HTTP endpoint
// via POST on each FSM transition.
type WebhookHook struct {
	url     string
	headers map[string]string
	client  *http.Client
}

// NewWebhookHook creates a WebhookHook that POSTs events to url, adding any
// provided headers to each request. The headers map is defensively copied so
// later caller mutation does not affect runtime behavior.
func NewWebhookHook(url string, headers map[string]string) *WebhookHook {
	var h map[string]string
	if headers != nil {
		h = make(map[string]string, len(headers))
		for k, v := range headers {
			h[k] = v
		}
	}
	return &WebhookHook{
		url:     url,
		headers: h,
		client:  &http.Client{},
	}
}

// Name returns the hook type identifier used in log messages.
func (w *WebhookHook) Name() string { return "webhook" }

// Run marshals ev to JSON and POSTs it to the configured URL. The provided
// context controls the request lifetime (cancellation / timeout). Any non-2xx
// response is returned as an error.
func (w *WebhookHook) Run(ctx context.Context, ev TransitionEvent) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("webhook marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("webhook: server returned status %d", resp.StatusCode)
	}
	return nil
}
