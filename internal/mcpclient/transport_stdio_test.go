package mcpclient_test

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/kastheco/kasmos/internal/mcpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStdioTransport_SendReceive(t *testing.T) {
	// Simulate server: write a response line, then read the request
	respBody := mcpclient.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(`{"ok":true}`),
	}
	respBytes, _ := json.Marshal(respBody)

	serverOut := bytes.NewBuffer(append(respBytes, '\n'))
	serverIn := &bytes.Buffer{}

	tr := mcpclient.NewStdioTransportFromPipes(io.NopCloser(serverOut), serverIn)

	req := mcpclient.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "test"}
	resp, err := tr.Send(req)
	require.NoError(t, err)
	assert.Equal(t, 1, resp.ID)
	assert.Nil(t, resp.Error)

	// Verify request was written
	var sentReq mcpclient.JSONRPCRequest
	require.NoError(t, json.NewDecoder(serverIn).Decode(&sentReq))
	assert.Equal(t, "test", sentReq.Method)
}
