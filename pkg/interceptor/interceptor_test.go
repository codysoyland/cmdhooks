package interceptor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codysoyland/cmdhooks/pkg/hook"
)

// mockHook implements both LocalHook and IPCHook interfaces
type mockHook struct {
	name      string
	commands  []string
	responses map[string]*hook.Response
	allowAll  bool
	evalCount int
	mu        sync.Mutex
}

func (m *mockHook) Name() string {
	return m.name
}

func (m *mockHook) Commands() []string {
	return m.commands
}

func (m *mockHook) EvaluateLocal(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	return m.evaluate(ctx, req)
}

func (m *mockHook) EvaluateIPC(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	return m.evaluate(ctx, req)
}

func (m *mockHook) evaluate(_ context.Context, req *hook.Request) (*hook.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.evalCount++

	if m.allowAll {
		return &hook.Response{}, nil
	}

	cmd := "unknown"
	if len(req.Command) > 0 {
		cmd = req.Command[0]
	}
	key := cmd + ":" + string(req.Hook)
	if response, exists := m.responses[key]; exists {
		return response, nil
	}

	return &hook.Response{Exit: true}, nil
}

func newMockHook(name string, commands []string) *mockHook {
	return &mockHook{
		name:      name,
		commands:  commands,
		allowAll:  true,
		responses: make(map[string]*hook.Response),
	}
}

func TestInterceptorExitFieldMapping(t *testing.T) {
	// Test that Exit field is properly mapped from Response to Response

	// Create mock hook that returns Exit=true
	mockHook := newMockHook("exit-test", []string{"*"})
	mockHook.allowAll = false // Disable allowAll to use custom responses
	mockHook.responses["curl:pre_run"] = &hook.Response{
		Exit: true,
	}

	// Simulate response evaluation (without full IPC setup)
	ctx := context.Background()
	req := &hook.Request{
		Command: []string{"curl", "https://example.com"},
		Hook:    hook.HookPreRun,
	}

	response, err := mockHook.EvaluateIPC(ctx, req)
	assert.NoError(t, err)
	assert.True(t, response.Exit)
	// No reason field to check

	// Verify Response construction
	resp := hook.Response{
		Exit: response.Exit,
	}

	assert.True(t, resp.Exit)
	// No reason field to check
}

func TestInterceptorExitSignal(t *testing.T) {
	// Test that exit signal channel works correctly
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test-exit-signal.sock")

	mockHook := newMockHook("exit-signal-test", []string{"*"})
	interceptor := New(testSocketPath, false, mockHook)

	// Test that exit signal channel is accessible
	exitSignal := interceptor.ExitSignal()
	assert.NotNil(t, exitSignal)

	// Channel should be empty initially
	select {
	case <-exitSignal:
		t.Fatal("Exit signal channel should be empty initially")
	default:
		// Expected - channel is empty
	}

	// Test that we can signal exit by closing the channel (simulate what happens in handleConnection)
	select {
	case <-interceptor.exitSignal:
		// Already closed
		t.Fatal("Exit signal channel should not be closed yet")
	default:
		close(interceptor.exitSignal)
	}

	// Now the channel should be closed and readable
	select {
	case <-exitSignal:
		// Expected - signal received
	case <-time.After(50 * time.Millisecond):
		t.Fatal("Should have received exit signal")
	}
}

// Unit tests for readRequest function
func TestReadRequest(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantRequest *hook.Request
		wantError   bool
	}{
		{
			name:  "valid request",
			input: `{"command":["curl","https://example.com"],"pid":123,"hook":"pre_run"}` + "\n",
			wantRequest: &hook.Request{
				Command: []string{"curl", "https://example.com"},
				PID:     123,
				Hook:    hook.HookPreRun,
			},
			wantError: false,
		},
		{
			name:  "request with metadata",
			input: `{"command":["wget","test.sh"],"pid":456,"hook":"post_run","metadata":{"key":"value"}}` + "\n",
			wantRequest: &hook.Request{
				Command:  []string{"wget", "test.sh"},
				PID:      456,
				Hook:     hook.HookPostRun,
				Metadata: map[string]interface{}{"key": "value"},
			},
			wantError: false,
		},
		{
			name:        "empty input",
			input:       "",
			wantRequest: nil,
			wantError:   true,
		},
		{
			name:        "malformed json",
			input:       `{"command":["curl","invalid json` + "\n",
			wantRequest: nil,
			wantError:   true,
		},
		{
			name:  "invalid hook type",
			input: `{"command":["curl"],"pid":123,"hook":"invalid_hook"}` + "\n",
			wantRequest: &hook.Request{
				Command: []string{"curl"},
				PID:     123,
				Hook:    hook.HookType("invalid_hook"),
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			req, err := readRequest(scanner)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, req)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantRequest, req)
			}
		})
	}
}

// Unit tests for writeResponse function
func TestWriteResponse(t *testing.T) {
	tests := []struct {
		name       string
		response   *hook.Response
		wantOutput string
		wantError  bool
	}{
		{
			name:       "exit false response",
			response:   &hook.Response{Exit: false},
			wantOutput: `{}` + "\n", // omitempty means false is omitted
			wantError:  false,
		},
		{
			name:       "exit true response",
			response:   &hook.Response{Exit: true},
			wantOutput: `{"exit":true}` + "\n",
			wantError:  false,
		},
		{
			name:       "response with metadata",
			response:   &hook.Response{Exit: false, Metadata: map[string]interface{}{"key": "value"}},
			wantOutput: `{"metadata":{"key":"value"}}` + "\n", // omitempty means false is omitted
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			writer := bufio.NewWriter(&buf)

			err := writeResponse(writer, tt.response)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.JSONEq(t, strings.TrimSuffix(tt.wantOutput, "\n"), strings.TrimSuffix(buf.String(), "\n"))
			}
		})
	}
}

// Mock hooks for testing
type mockIPCHook struct {
	name     string
	commands []string
	response *hook.Response
	err      error
}

func (m *mockIPCHook) Name() string {
	if m.name == "" {
		return "mock-ipc-hook"
	}
	return m.name
}

func (m *mockIPCHook) Commands() []string {
	if m.commands == nil {
		return []string{"*"}
	}
	return m.commands
}

func (m *mockIPCHook) EvaluateIPC(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	return m.response, m.err
}

type mockBasicHook struct {
	name     string
	commands []string
}

func (m *mockBasicHook) Name() string {
	return m.name
}

func (m *mockBasicHook) Commands() []string {
	return m.commands
}

// Unit tests for processRequest method
func TestProcessRequest(t *testing.T) {
	tests := []struct {
		name      string
		hook      hook.Hook
		request   *hook.Request
		wantExit  bool
		wantError bool
	}{
		{
			name: "IPC hook allows command",
			hook: &mockIPCHook{
				response: &hook.Response{Exit: false},
			},
			request: &hook.Request{
				Command: []string{"curl", "https://example.com"},
				PID:     123,
				Hook:    hook.HookPreRun,
			},
			wantExit:  false,
			wantError: false,
		},
		{
			name: "IPC hook blocks command",
			hook: &mockIPCHook{
				response: &hook.Response{Exit: true},
			},
			request: &hook.Request{
				Command: []string{"curl", "https://malicious.com"},
				PID:     456,
				Hook:    hook.HookPreRun,
			},
			wantExit:  true,
			wantError: false,
		},
		{
			name: "IPC hook returns error",
			hook: &mockIPCHook{
				response: nil,
				err:      fmt.Errorf("hook evaluation failed"),
			},
			request: &hook.Request{
				Command: []string{"wget", "test.sh"},
				PID:     789,
				Hook:    hook.HookPreRun,
			},
			wantExit:  true,
			wantError: false,
		},
		{
			name: "non-IPC hook defaults to exit",
			hook: &mockBasicHook{
				name:     "basic-hook",
				commands: []string{"curl"},
			},
			request: &hook.Request{
				Command: []string{"curl", "https://example.com"},
				PID:     123,
				Hook:    hook.HookPreRun,
			},
			wantExit:  true,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			socketPath := filepath.Join(tmpDir, "test.sock")
			interceptor := New(socketPath, false, tt.hook)

			resp, err := interceptor.processRequest(tt.request)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantExit, resp.Exit)
			}
		})
	}
}

// Lifecycle tests
func TestNew(t *testing.T) {
	t.Run("with valid hook", func(t *testing.T) {
		mockHook := newMockHook("test-hook", []string{"curl"})
		interceptor := New("/tmp/test.sock", true, mockHook)

		assert.NotNil(t, interceptor)
		assert.Equal(t, "/tmp/test.sock", interceptor.socketPath)
		assert.True(t, interceptor.verbose)
		assert.Equal(t, mockHook, interceptor.hook)
		assert.NotNil(t, interceptor.stop)
		assert.NotNil(t, interceptor.exitSignal)
	})

	t.Run("with nil hook panics", func(t *testing.T) {
		assert.Panics(t, func() {
			New("/tmp/test.sock", false, nil)
		})
	})
}

func TestInterceptorStart(t *testing.T) {
	t.Run("successful start", func(t *testing.T) {
		// Use shorter socket path for macOS Unix domain socket limits
		socketPath := fmt.Sprintf("/tmp/test_%d.sock", time.Now().UnixNano())
		mockHook := newMockHook("test-hook", []string{"curl"})
		interceptor := New(socketPath, false, mockHook)
		defer os.Remove(socketPath) // Ensure cleanup

		err := interceptor.Start()
		assert.NoError(t, err)
		assert.NotNil(t, interceptor.listener)

		// Verify socket file exists and has correct permissions
		info, err := os.Stat(socketPath)
		assert.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

		// Cleanup
		interceptor.Stop()
	})

	t.Run("start with existing socket removes old file", func(t *testing.T) {
		// Use shorter socket path for macOS Unix domain socket limits
		socketPath := fmt.Sprintf("/tmp/test_%d.sock", time.Now().UnixNano())
		defer os.Remove(socketPath) // Ensure cleanup

		// Create existing socket file
		f, err := os.Create(socketPath)
		require.NoError(t, err)
		f.Close()

		mockHook := newMockHook("test-hook", []string{"curl"})
		interceptor := New(socketPath, false, mockHook)

		err = interceptor.Start()
		assert.NoError(t, err)
		assert.NotNil(t, interceptor.listener)

		// Cleanup
		interceptor.Stop()
	})

	t.Run("start failure - invalid path", func(t *testing.T) {
		// Try to create socket in non-existent directory
		socketPath := "/nonexistent/directory/test.sock"
		mockHook := newMockHook("test-hook", []string{"curl"})
		interceptor := New(socketPath, false, mockHook)

		err := interceptor.Start()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create socket listener")
	})
}

func TestInterceptorStop(t *testing.T) {
	t.Run("stop after start", func(t *testing.T) {
		// Use shorter socket path for macOS Unix domain socket limits
		socketPath := fmt.Sprintf("/tmp/test_%d.sock", time.Now().UnixNano())
		mockHook := newMockHook("test-hook", []string{"curl"})
		interceptor := New(socketPath, false, mockHook)
		defer os.Remove(socketPath) // Ensure cleanup

		err := interceptor.Start()
		require.NoError(t, err)

		// Verify socket exists
		_, err = os.Stat(socketPath)
		assert.NoError(t, err)

		interceptor.Stop()

		// Verify socket is removed
		_, err = os.Stat(socketPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("stop without start is safe", func(t *testing.T) {
		mockHook := newMockHook("test-hook", []string{"curl"})
		interceptor := New("/tmp/test.sock", false, mockHook)

		// Should not panic or error
		interceptor.Stop()
	})

	t.Run("double stop is safe", func(t *testing.T) {
		// Use shorter socket path for macOS Unix domain socket limits
		socketPath := fmt.Sprintf("/tmp/test_%d.sock", time.Now().UnixNano())
		mockHook := newMockHook("test-hook", []string{"curl"})
		interceptor := New(socketPath, false, mockHook)
		defer os.Remove(socketPath) // Ensure cleanup

		err := interceptor.Start()
		require.NoError(t, err)

		interceptor.Stop()
		interceptor.Stop() // Should be safe
	})
}

func TestSetHookAndHook(t *testing.T) {
	mockHook1 := newMockHook("hook1", []string{"curl"})
	mockHook2 := newMockHook("hook2", []string{"wget"})

	interceptor := New("/tmp/test.sock", false, mockHook1)

	// Test initial hook
	assert.Equal(t, mockHook1, interceptor.Hook())

	// Test hook replacement
	interceptor.SetHook(mockHook2)
	assert.Equal(t, mockHook2, interceptor.Hook())
}

// Integration tests with real socket communication
func TestSocketCommunication(t *testing.T) {
	t.Run("full request-response cycle", func(t *testing.T) {
		// Use shorter socket path for macOS Unix domain socket limits
		socketPath := fmt.Sprintf("/tmp/test_%d.sock", time.Now().UnixNano())
		defer os.Remove(socketPath) // Ensure cleanup
		mockHook := &mockIPCHook{
			response: &hook.Response{Exit: false},
		}
		interceptor := New(socketPath, false, mockHook)

		// Start interceptor
		err := interceptor.Start()
		require.NoError(t, err)
		defer interceptor.Stop()

		// Give listener time to start
		time.Sleep(5 * time.Millisecond)

		// Connect as client
		conn, err := net.Dial("unix", socketPath)
		require.NoError(t, err)
		defer conn.Close()

		// Send request
		request := map[string]interface{}{
			"command": []string{"curl", "https://example.com"},
			"pid":     123,
			"hook":    "pre_run",
		}
		reqData, err := json.Marshal(request)
		require.NoError(t, err)

		_, err = fmt.Fprintf(conn, "%s\n", string(reqData))
		require.NoError(t, err)

		// Read response
		scanner := bufio.NewScanner(conn)
		require.True(t, scanner.Scan())

		var response map[string]interface{}
		err = json.Unmarshal(scanner.Bytes(), &response)
		require.NoError(t, err)

		// Check that exit is not present (omitempty) or explicitly false
		if exit, exists := response["exit"]; exists {
			assert.False(t, exit.(bool))
		}
		// If exit key doesn't exist, it means false due to omitempty
	})

	t.Run("multiple concurrent connections", func(t *testing.T) {
		// Use shorter socket path for macOS Unix domain socket limits
		socketPath := fmt.Sprintf("/tmp/test_%d.sock", time.Now().UnixNano())
		defer os.Remove(socketPath) // Ensure cleanup
		mockHook := &mockIPCHook{
			response: &hook.Response{Exit: false},
		}
		interceptor := New(socketPath, false, mockHook)

		// Start interceptor
		err := interceptor.Start()
		require.NoError(t, err)
		defer interceptor.Stop()

		// Give listener time to start
		time.Sleep(5 * time.Millisecond)

		// Launch multiple concurrent clients
		numClients := 5
		var wg sync.WaitGroup
		errorChan := make(chan error, numClients)

		for i := 0; i < numClients; i++ {
			wg.Add(1)
			go func(clientID int) {
				defer wg.Done()

				conn, err := net.Dial("unix", socketPath)
				if err != nil {
					errorChan <- err
					return
				}
				defer conn.Close()

				request := map[string]interface{}{
					"command": []string{"curl", fmt.Sprintf("https://example%d.com", clientID)},
					"pid":     123 + clientID,
					"hook":    "pre_run",
				}
				reqData, err := json.Marshal(request)
				if err != nil {
					errorChan <- err
					return
				}

				_, err = fmt.Fprintf(conn, "%s\n", string(reqData))
				if err != nil {
					errorChan <- err
					return
				}

				scanner := bufio.NewScanner(conn)
				if !scanner.Scan() {
					errorChan <- fmt.Errorf("failed to read response")
					return
				}

				var response map[string]interface{}
				err = json.Unmarshal(scanner.Bytes(), &response)
				if err != nil {
					errorChan <- err
					return
				}

				// Check that exit is not present (omitempty) or explicitly false
				if exit, exists := response["exit"]; exists {
					if exitBool, ok := exit.(bool); ok && exitBool {
						errorChan <- fmt.Errorf("unexpected exit value: %v", response["exit"])
						return
					}
				}
				// If exit key doesn't exist, it means false due to omitempty
			}(i)
		}

		wg.Wait()
		close(errorChan)

		// Check for any errors
		for err := range errorChan {
			t.Errorf("Client error: %v", err)
		}
	})

	t.Run("malformed request handling", func(t *testing.T) {
		// Use shorter socket path for macOS Unix domain socket limits
		socketPath := fmt.Sprintf("/tmp/test_%d.sock", time.Now().UnixNano())
		defer os.Remove(socketPath) // Ensure cleanup
		mockHook := &mockIPCHook{
			response: &hook.Response{Exit: false},
		}
		interceptor := New(socketPath, false, mockHook)

		// Start interceptor
		err := interceptor.Start()
		require.NoError(t, err)
		defer interceptor.Stop()

		// Give listener time to start
		time.Sleep(5 * time.Millisecond)

		// Connect as client
		conn, err := net.Dial("unix", socketPath)
		require.NoError(t, err)
		defer conn.Close()

		// Send malformed JSON
		_, err = fmt.Fprintf(conn, "invalid json\n")
		require.NoError(t, err)

		// Should receive error response with exit=true
		scanner := bufio.NewScanner(conn)
		require.True(t, scanner.Scan())

		var response map[string]interface{}
		err = json.Unmarshal(scanner.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, true, response["exit"])
	})
}
