package mcpclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// StdioTransport speaks JSON-RPC over stdin/stdout of a subprocess.
type StdioTransport struct {
	cmd    *exec.Cmd // nil when created from pipes
	reader *bufio.Reader
	writer io.Writer
	closer io.Closer // stdin pipe or reader closer
	mu     sync.Mutex
}

// NewStdioTransport spawns a subprocess and connects to its stdin/stdout.
func NewStdioTransport(command string, args []string, env []string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = append(cmd.Environ(), env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}
	return &StdioTransport{
		cmd:    cmd,
		reader: bufio.NewReader(stdout),
		writer: stdin,
		closer: stdin,
	}, nil
}

// NewStdioTransportFromPipes creates a transport from pre-existing reader/writer (for testing).
func NewStdioTransportFromPipes(r io.ReadCloser, w io.Writer) *StdioTransport {
	return &StdioTransport{
		reader: bufio.NewReader(r),
		writer: w,
		closer: r,
	}
}

// Send writes a JSON-RPC request and reads the response.
func (t *StdioTransport) Send(req JSONRPCRequest) (JSONRPCResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')
	if _, err := t.writer.Write(data); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("write request: %w", err)
	}

	line, err := t.reader.ReadBytes('\n')
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("read response: %w", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("parse response: %w", err)
	}
	return resp, nil
}

// Close terminates the subprocess.
func (t *StdioTransport) Close() error {
	t.closer.Close()
	if t.cmd != nil {
		return t.cmd.Wait()
	}
	return nil
}
