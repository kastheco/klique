package taskfsm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookHook_PostsJSON(t *testing.T) {
	var received TransitionEvent
	var capturedMethod string
	var capturedContentType string
	var capturedCustomHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedContentType = r.Header.Get("Content-Type")
		capturedCustomHeader = r.Header.Get("X-Custom-Header")

		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hook := NewWebhookHook(srv.URL, map[string]string{
		"X-Custom-Header": "test-value",
	})

	assert.Equal(t, "webhook", hook.Name())

	ev := TransitionEvent{
		PlanFile:   "plans/test.md",
		FromStatus: StatusReady,
		ToStatus:   StatusPlanning,
		Event:      PlanStart,
		Timestamp:  time.Now().UTC(),
		Project:    "test-proj",
	}

	err := hook.Run(context.Background(), ev)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, capturedMethod)
	assert.Equal(t, "application/json", capturedContentType)
	assert.Equal(t, "test-value", capturedCustomHeader)
	assert.Equal(t, ev.PlanFile, received.PlanFile)
	assert.Equal(t, ev.Event, received.Event)
}

func TestWebhookHook_RespectsContextTimeout(t *testing.T) {
	// handlerDone is closed by t.Cleanup to unblock the handler before srv.Close()
	// waits for the WaitGroup to drain. Without this, srv.Close() deadlocks because
	// Go's HTTP server does not cancel r.Context() when a non-reading handler is
	// running and the client forcibly closes the connection.
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the client disconnects/times out OR the test cleans up.
		select {
		case <-r.Context().Done():
		case <-handlerDone:
		}
	}))
	t.Cleanup(func() {
		close(handlerDone)
		srv.Close()
	})

	hook := NewWebhookHook(srv.URL, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ev := TransitionEvent{
		PlanFile:  "plans/test.md",
		Event:     PlanStart,
		Timestamp: time.Now().UTC(),
	}

	err := hook.Run(ctx, ev)
	assert.Error(t, err, "Run should return an error when context times out")
}
