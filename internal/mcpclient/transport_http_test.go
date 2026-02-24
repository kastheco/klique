package mcpclient_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kastheco/kasmos/internal/mcpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPTransport_SendReceive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req mcpclient.JSONRPCRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		resp := mcpclient.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"tools":[]}`),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tr := mcpclient.NewHTTPTransport(srv.URL, "test-token")
	req := mcpclient.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"}
	resp, err := tr.Send(req)
	require.NoError(t, err)
	assert.Equal(t, 1, resp.ID)
}

func TestHTTPTransport_BearerAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		resp := mcpclient.JSONRPCResponse{JSONRPC: "2.0", ID: 1, Result: json.RawMessage(`{}`)}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tr := mcpclient.NewHTTPTransport(srv.URL, "my-secret-token")
	_, _ = tr.Send(mcpclient.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "test"})
	assert.Equal(t, "Bearer my-secret-token", gotAuth)
}

func TestHTTPTransport_NoAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		resp := mcpclient.JSONRPCResponse{JSONRPC: "2.0", ID: 1, Result: json.RawMessage(`{}`)}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tr := mcpclient.NewHTTPTransport(srv.URL, "")
	_, _ = tr.Send(mcpclient.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "test"})
	assert.Empty(t, gotAuth)
}

func TestHTTPTransport_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	tr := mcpclient.NewHTTPTransport(srv.URL, "tok")
	_, err := tr.Send(mcpclient.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestHTTPTransport_Close(t *testing.T) {
	tr := mcpclient.NewHTTPTransport("http://localhost", "")
	assert.NoError(t, tr.Close())
}
