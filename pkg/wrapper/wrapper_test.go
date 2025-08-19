package wrapper

import (
	"context"
	"maps"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/codysoyland/cmdhooks/pkg/hook"
)

// mockLocalHook implements hook.LocalHook for testing
type mockLocalHook struct {
	name      string
	commands  []string
	responses map[string]*hook.Response
	allowAll  bool
	evalCount int
}

func (m *mockLocalHook) Name() string {
	return m.name
}

func (m *mockLocalHook) Commands() []string {
	return m.commands
}

func (m *mockLocalHook) EvaluateLocal(ctx context.Context, req *hook.Request) (*hook.Response, error) {
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

func newMockLocalHook(name string, commands []string) *mockLocalHook {
	return &mockLocalHook{
		name:      name,
		commands:  commands,
		allowAll:  true,
		responses: make(map[string]*hook.Response),
	}
}

// mockHook implements basic hook interface for testing
type mockHook struct {
	name     string
	commands []string
}

func (m *mockHook) Name() string {
	return m.name
}

func (m *mockHook) Commands() []string {
	return m.commands
}

func newMockHook(name string, commands []string) *mockHook {
	return &mockHook{
		name:     name,
		commands: commands,
	}
}

func TestNewWrapperCommand(t *testing.T) {
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")

	tests := []struct {
		name     string
		hook     hook.Hook
		options  []WrapperOption
		expected func(*WrapperCommand) bool
	}{
		{
			name:    "default values with nil hook",
			hook:    nil,
			options: []WrapperOption{},
			expected: func(w *WrapperCommand) bool {
				return w.SocketPath == "" &&
					w.Verbose == false &&
					w.Hook == nil
			},
		},
		{
			name: "with socket path",
			hook: nil,
			options: []WrapperOption{
				WithSocketPath(testSocketPath),
			},
			expected: func(w *WrapperCommand) bool {
				return w.SocketPath == testSocketPath
			},
		},
		{
			name: "with verbose enabled",
			hook: nil,
			options: []WrapperOption{
				WithVerbose(true),
			},
			expected: func(w *WrapperCommand) bool {
				return w.Verbose == true
			},
		},
		{
			name:    "with local hook",
			hook:    newMockLocalHook("test", []string{"curl"}),
			options: []WrapperOption{},
			expected: func(w *WrapperCommand) bool {
				return w.Hook != nil &&
					w.Hook.Name() == "test"
			},
		},
		{
			name:    "with regular hook",
			hook:    newMockHook("test", []string{"curl"}),
			options: []WrapperOption{},
			expected: func(w *WrapperCommand) bool {
				return w.Hook != nil &&
					w.Hook.Name() == "test"
			},
		},
		{
			name: "all options combined",
			hook: newMockLocalHook("test", []string{"curl"}),
			options: []WrapperOption{
				WithSocketPath(testSocketPath),
				WithVerbose(true),
			},
			expected: func(w *WrapperCommand) bool {
				return w.SocketPath == testSocketPath &&
					w.Verbose == true &&
					w.Hook != nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapper := NewWrapperCommand(tt.hook, tt.options...)
			assert.True(t, tt.expected(wrapper), "Expected conditions not met")
		})
	}
}

func TestWrapperCommand_evaluateHooks(t *testing.T) {
	t.Run("no hooks", func(t *testing.T) {
		wrapper := NewWrapperCommand(nil)

		req := &hook.Request{
			Command: []string{"curl", "-s", "https://example.com"},
			PID:     123,
			Hook:    hook.HookPreRun,
		}

		response, err := wrapper.evaluateHooks(req)
		assert.NoError(t, err)
		assert.False(t, response.Exit)
		// No reason field to check
	})

	t.Run("local hook allows", func(t *testing.T) {
		localHook := newMockLocalHook("test", []string{"curl"})
		wrapper := NewWrapperCommand(localHook)

		req := &hook.Request{
			Command: []string{"curl", "-s", "https://example.com"},
			PID:     123,
			Hook:    hook.HookPreRun,
		}

		response, err := wrapper.evaluateHooks(req)
		assert.NoError(t, err)
		assert.False(t, response.Exit)
		// No reason field to check
		assert.Equal(t, 1, localHook.evalCount)
	})

	t.Run("local hook denies", func(t *testing.T) {
		localHook := newMockLocalHook("test", []string{"curl"})
		localHook.allowAll = false
		wrapper := NewWrapperCommand(localHook)

		req := &hook.Request{
			Command: []string{"curl", "-s", "https://example.com"},
			PID:     123,
			Hook:    hook.HookPreRun,
		}

		response, err := wrapper.evaluateHooks(req)
		assert.NoError(t, err)
		assert.True(t, response.Exit)
		// No reason field to check
		assert.Equal(t, 1, localHook.evalCount)
	})

	t.Run("local hook doesn't handle command", func(t *testing.T) {
		localHook := newMockLocalHook("test", []string{"wget"})
		wrapper := NewWrapperCommand(localHook)

		req := &hook.Request{
			Command: []string{"curl", "-s", "https://example.com"}, // Different from hook's commands
			PID:     123,
			Hook:    hook.HookPreRun,
		}

		response, err := wrapper.evaluateHooks(req)
		assert.NoError(t, err)
		assert.False(t, response.Exit)
		// No reason field to check
		assert.Equal(t, 0, localHook.evalCount) // Hook shouldn't be called
	})
}

func TestWrapperCommand_SetSocketPath(t *testing.T) {
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")

	wrapper := NewWrapperCommand(nil)
	assert.Empty(t, wrapper.SocketPath)

	wrapper.SetSocketPath(testSocketPath)

	assert.Equal(t, testSocketPath, wrapper.SocketPath)
}

func TestWrapperCommand_SetVerbose(t *testing.T) {
	wrapper := NewWrapperCommand(nil)
	assert.False(t, wrapper.Verbose)

	wrapper.SetVerbose(true)
	assert.True(t, wrapper.Verbose)

	wrapper.SetVerbose(false)
	assert.False(t, wrapper.Verbose)
}

func TestWrapperCommand_RunWrapperDirectExecution(t *testing.T) {
	// Test commands that should run directly (no interception)
	wrapper := NewWrapperCommand(nil)

	// No local hooks and no socket path - should run directly without error
	err := wrapper.Run([]string{"echo", "hello", "world"})
	assert.NoError(t, err)
}

func TestWrapperCommand_RunWrapperWithLocalHook(t *testing.T) {
	t.Run("local hook allows", func(t *testing.T) {
		localHook := newMockLocalHook("test", []string{"echo"})
		wrapper := NewWrapperCommand(localHook)

		err := wrapper.Run([]string{"echo", "test"})
		assert.NoError(t, err)
		assert.Equal(t, 2, localHook.evalCount) // Pre-run + post-run
	})

	t.Run("local hook denies pre-run", func(t *testing.T) {
		localHook := newMockLocalHook("test", []string{"echo"})
		localHook.allowAll = false
		wrapper := NewWrapperCommand(localHook)

		err := wrapper.Run([]string{"echo", "test"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "process termination requested")
		assert.Equal(t, 1, localHook.evalCount) // Only pre-run
	})
}

func TestWrapperCommand_VerboseOutput(t *testing.T) {
	// Test that verbose mode doesn't break functionality
	localHook := newMockLocalHook("test", []string{"echo"})
	wrapper := NewWrapperCommand(localHook, WithVerbose(true))

	err := wrapper.Run([]string{"echo", "test"})
	assert.NoError(t, err)
}

func TestWrapperOptions(t *testing.T) {
	t.Run("WithSocketPath", func(t *testing.T) {
		tmpDir := t.TempDir()
		testSocketPath := filepath.Join(tmpDir, "test.sock")

		w := &WrapperCommand{}
		option := WithSocketPath(testSocketPath)
		option(w)
		assert.Equal(t, testSocketPath, w.SocketPath)
	})

	t.Run("WithVerbose", func(t *testing.T) {
		w := &WrapperCommand{}
		option := WithVerbose(true)
		option(w)
		assert.True(t, w.Verbose)
	})
}

func TestMetadataPassing(t *testing.T) {
	t.Run("local hook adds metadata for IPC", func(t *testing.T) {
		// Create a mock local hook that adds metadata
		localHook := newMockLocalHook("test", []string{"*"})
		localHook.allowAll = false
		localHook.responses["curl:pre_run"] = &hook.Response{
			Metadata: map[string]interface{}{
				"local_processed": true,
				"local_count":     42,
				"local_hook_name": "test",
			},
		}

		wrapper := NewWrapperCommand(localHook)

		req := &hook.Request{
			Command: []string{"curl", "-s", "https://example.com"},
			PID:     123,
			Hook:    hook.HookPreRun,
			Metadata: map[string]interface{}{
				"original_key": "original_value",
			},
		}

		// Since we don't have IPC set up in this test, we should get the local response
		response, err := wrapper.evaluateHooks(req)
		assert.NoError(t, err)
		assert.False(t, response.Exit)
		// No reason field to check

		// Check that metadata from local hook is preserved
		expectedMetadata := map[string]interface{}{
			"local_processed": true,
			"local_count":     42,
			"local_hook_name": "test",
		}
		assert.Equal(t, expectedMetadata, response.Metadata)
	})

	t.Run("metadata merging behavior", func(t *testing.T) {
		// Test the metadata merging logic directly
		originalMetadata := map[string]interface{}{
			"original_key": "original_value",
			"shared_key":   "original_shared",
		}

		localMetadata := map[string]interface{}{
			"local_key":  "local_value",
			"shared_key": "local_shared", // This should override the original
		}

		// Simulate the merging logic from evaluateHooks
		mergedMetadata := make(map[string]interface{})

		// Add original metadata
		maps.Copy(mergedMetadata, originalMetadata)

		// Add local metadata (overrides conflicts)
		maps.Copy(mergedMetadata, localMetadata)

		expected := map[string]interface{}{
			"original_key": "original_value",
			"shared_key":   "local_shared", // Local should override
			"local_key":    "local_value",
		}

		assert.Equal(t, expected, mergedMetadata)
	})
}
