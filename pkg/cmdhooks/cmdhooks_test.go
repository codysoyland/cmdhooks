package cmdhooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codysoyland/cmdhooks/pkg/hook"
)

// mockHook is a test double for hook.Hook
type mockHook struct {
	name      string
	commands  []string
	allowAll  bool
	responses map[string]*hook.Response
}

func (m *mockHook) Name() string {
	return m.name
}

func (m *mockHook) Commands() []string {
	return m.commands
}

func (m *mockHook) Evaluate(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	if m.allowAll {
		return &hook.Response{}, nil
	}

	cmd := "unknown"
	args := []string{}
	if len(req.Command) > 0 {
		cmd = req.Command[0]
		args = req.Command[1:]
	}
	key := cmd + ":" + strings.Join(args, ",")
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

// mockLocalHook implements LocalHook for testing interceptor scenarios
type mockLocalHook struct {
	*mockHook
}

func (m *mockLocalHook) EvaluateLocal(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	return m.Evaluate(ctx, req)
}

func newMockLocalHook(name string, commands []string) *mockLocalHook {
	return &mockLocalHook{
		mockHook: newMockHook(name, commands),
	}
}

// mockIPCHook implements IPCHook for testing interceptor scenarios
type mockIPCHook struct {
	*mockHook
}

func (m *mockIPCHook) EvaluateIPC(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	return m.Evaluate(ctx, req)
}

func newMockIPCHook(name string, commands []string) *mockIPCHook {
	return &mockIPCHook{
		mockHook: newMockHook(name, commands),
	}
}

func TestNew(t *testing.T) {
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")

	tests := []struct {
		name        string
		options     []Option
		expectError bool
		errorMsg    string
	}{
		{
			name:        "missing hook should fail",
			options:     []Option{},
			expectError: true,
			errorMsg:    "must provide hook",
		},
		{
			name: "valid configuration should succeed",
			options: []Option{
				WithHook(newMockHook("test", []string{"curl"})),
				WithVerbose(true),
			},
			expectError: false,
		},
		{
			name: "all options should work together",
			options: []Option{
				WithHook(newMockHook("test", []string{"curl", "wget"})),
				WithVerbose(true),
				WithSocketPath(testSocketPath),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch, err := New(tt.options...)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, ch)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, ch)

				// Verify configuration was applied
				assert.NotNil(t, ch.config)
				assert.NotNil(t, ch.hook)
				// Interceptor is always created for consistent behavior
				assert.NotNil(t, ch.interceptor)
			}
		})
	}
}

func TestOptions(t *testing.T) {
	t.Run("WithHook", func(t *testing.T) {
		hook := newMockHook("test", []string{"curl"})
		config := &Config{}

		option := WithHook(hook)
		err := option(config)

		assert.NoError(t, err)
		assert.Equal(t, hook, config.Hook)
	})

	t.Run("WithVerbose", func(t *testing.T) {
		tests := []bool{true, false}

		for _, verbose := range tests {
			config := &Config{}
			option := WithVerbose(verbose)
			err := option(config)

			assert.NoError(t, err)
			assert.Equal(t, verbose, config.Verbose)
		}
	})

	t.Run("WithSocketPath", func(t *testing.T) {
		tmpDir := t.TempDir()
		testSocketPath := filepath.Join(tmpDir, "test.sock")
		config := &Config{}

		option := WithSocketPath(testSocketPath)
		err := option(config)

		assert.NoError(t, err)
		assert.Equal(t, testSocketPath, config.SocketPath)
	})

	t.Run("WithWrapperPath", func(t *testing.T) {
		wrapperPath := []string{"/custom/exe", "run"}
		config := &Config{}

		option := WithWrapperPath(wrapperPath)
		err := option(config)

		assert.NoError(t, err)
		assert.Equal(t, wrapperPath, config.WrapperPath)
	})

}

func TestCmdHooks_SetHook(t *testing.T) {
	hook1 := newMockHook("hook1", []string{"curl"})
	hook2 := newMockHook("hook2", []string{"wget"})

	ch, err := New(WithHook(hook1))
	require.NoError(t, err)

	// Initial hook should be hook1
	assert.Equal(t, hook1, ch.GetHook())

	// Change to hook2
	ch.SetHook(hook2)
	assert.Equal(t, hook2, ch.GetHook())
	assert.Equal(t, hook2, ch.config.Hook)
}

func TestCmdHooks_Close(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	hook := newMockHook("test", []string{"curl"})

	ch, err := New(
		WithHook(hook),
		WithSocketPath(socketPath),
	)
	require.NoError(t, err)

	// Create a dummy socket file to test cleanup
	err = os.WriteFile(socketPath, []byte("test"), 0644)
	require.NoError(t, err)

	// Close should clean up resources
	err = ch.Close()
	assert.NoError(t, err)

	// Socket file should be removed
	_, err = os.Stat(socketPath)
	assert.True(t, os.IsNotExist(err))
}

func TestCmdHooks_CreateWrappers(t *testing.T) {
	hook := newMockHook("test", []string{"curl", "wget"})

	// Use current executable as wrapper for testing (simulates WithWrapperPath override)
	exePath, err := os.Executable()
	require.NoError(t, err)

	ch, err := New(WithHook(hook), WithWrapperPath([]string{exePath, "run"}))
	require.NoError(t, err)

	wrapperDir, cleanup, err := ch.createWrappers()
	require.NoError(t, err)
	defer cleanup()

	// Check wrapper directory exists
	assert.DirExists(t, wrapperDir)

	// Check wrapper files exist and are executable
	for _, cmd := range []string{"curl", "wget"} {
		wrapperPath := filepath.Join(wrapperDir, cmd)
		assert.FileExists(t, wrapperPath)

		// Check file is executable
		info, err := os.Stat(wrapperPath)
		require.NoError(t, err)
		assert.True(t, info.Mode()&0100 != 0, "wrapper should be executable")

		// Check wrapper content
		content, err := os.ReadFile(wrapperPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "#!/usr/bin/env bash")
		assert.Contains(t, string(content), cmd)
		assert.Contains(t, string(content), "run")
	}
}

func TestCmdHooks_CreateWrappersNoCommands(t *testing.T) {
	// Hook with no commands should create empty wrapper dir
	hook := newMockHook("test", []string{})

	// Use current executable as wrapper for testing (simulates WithWrapperPath override)
	exePath, err := os.Executable()
	require.NoError(t, err)

	ch, err := New(WithHook(hook), WithWrapperPath([]string{exePath, "run"}))
	require.NoError(t, err)

	wrapperDir, cleanup, err := ch.createWrappers()
	require.NoError(t, err)
	defer cleanup()

	// Directory should exist but be empty
	assert.DirExists(t, wrapperDir)

	entries, err := os.ReadDir(wrapperDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "wrapper directory should be empty when no commands specified")
}

func TestInterceptorAlwaysCreated(t *testing.T) {
	t.Run("LocalHook always creates interceptor", func(t *testing.T) {
		localHook := newMockLocalHook("test", []string{"curl"})

		ch, err := New(WithHook(localHook))
		require.NoError(t, err)

		// Interceptor is always created for consistent behavior
		assert.NotNil(t, ch.interceptor)
		assert.NotNil(t, ch.hook)
		assert.NotNil(t, ch.config)
	})

	t.Run("IPCHook always creates interceptor", func(t *testing.T) {
		ipcHook := newMockIPCHook("test", []string{"curl"})

		ch, err := New(WithHook(ipcHook))
		require.NoError(t, err)

		// IPCHook creates interceptor as before
		assert.NotNil(t, ch.interceptor)
		assert.NotNil(t, ch.hook)
		assert.NotNil(t, ch.config)
	})

	t.Run("Basic Hook always creates interceptor", func(t *testing.T) {
		basicHook := newMockHook("test", []string{"curl"})

		ch, err := New(WithHook(basicHook))
		require.NoError(t, err)

		// Interceptor is always created for consistent behavior
		assert.NotNil(t, ch.interceptor)
		assert.NotNil(t, ch.hook)
		assert.NotNil(t, ch.config)
	})
}

func TestCmdHooks_CreateWrappersCustomPath(t *testing.T) {
	hook := newMockHook("test", []string{"curl"})
	customWrapperPath := []string{"/custom/path/myexe", "wrapper-cmd"}

	ch, err := New(
		WithHook(hook),
		WithWrapperPath(customWrapperPath),
	)
	require.NoError(t, err)

	wrapperDir, cleanup, err := ch.createWrappers()
	require.NoError(t, err)
	defer cleanup()

	// Check wrapper file exists and contains custom path
	wrapperPath := filepath.Join(wrapperDir, "curl")
	assert.FileExists(t, wrapperPath)

	content, err := os.ReadFile(wrapperPath)
	require.NoError(t, err)
	contentStr := string(content)

	// Should contain custom wrapper path
	assert.Contains(t, contentStr, "/custom/path/myexe")
	assert.Contains(t, contentStr, "wrapper-cmd")
	assert.Contains(t, contentStr, "curl")
	assert.Contains(t, contentStr, "#!/usr/bin/env bash")
}
