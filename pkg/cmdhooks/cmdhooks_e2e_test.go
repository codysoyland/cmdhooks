package cmdhooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codysoyland/cmdhooks/pkg/hook"
)

// testHook is a configurable hook for E2E testing
type testHook struct {
	name             string
	commands         []string
	allowAllCommands bool
	blockedCmds      map[string]bool
}

func newTestHook(name string, commands []string) *testHook {
	return &testHook{
		name:             name,
		commands:         commands,
		allowAllCommands: true,
		blockedCmds:      make(map[string]bool),
	}
}

func (h *testHook) Name() string {
	return h.name
}

func (h *testHook) Commands() []string {
	return h.commands
}

// Block specific commands
func (h *testHook) blockCommand(cmd string) {
	h.blockedCmds[cmd] = true
}

// Allow all commands
func (h *testHook) allowAll() {
	h.allowAllCommands = true
	h.blockedCmds = make(map[string]bool)
}

// Implement LocalHook interface
func (h *testHook) EvaluateLocal(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	// Check if command should be blocked
	if len(req.Command) > 0 {
		cmd := req.Command[0]
		if h.blockedCmds[cmd] {
			return &hook.Response{Exit: true}, nil
		}
	}

	return &hook.Response{Exit: false}, nil
}

// Implement IPCHook interface
func (h *testHook) EvaluateIPC(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	// Check if command should be blocked
	if len(req.Command) > 0 {
		cmd := req.Command[0]
		if h.blockedCmds[cmd] {
			return &hook.Response{Exit: true}, nil
		}
	}

	return &hook.Response{Exit: false}, nil
}

// createTestScript creates a temporary bash script for testing
func createTestScript(t *testing.T, content string) string {
	tmpFile, err := os.CreateTemp("", "cmdhooks_test_*.sh")
	require.NoError(t, err)

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)

	err = tmpFile.Close()
	require.NoError(t, err)

	err = os.Chmod(tmpFile.Name(), 0755)
	require.NoError(t, err)

	t.Cleanup(func() {
		os.Remove(tmpFile.Name())
	})

	return tmpFile.Name()
}

// TestE2E_CommandInterception tests basic command interception
func TestE2E_CommandInterception(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	testHook := newTestHook("test-interception", []string{"echo"})

	// Create a simple script that uses echo (which should be intercepted and allowed)
	scriptContent := `#!/usr/bin/env bash
echo "test-marker-123"
`
	scriptPath := createTestScript(t, scriptContent)

	// Execute script with cmdhooks using go run
	ch, err := New(
		WithHook(testHook),
		WithWrapperPath([]string{"go", "run", "../../cmd/cmdhooks"}),
		WithVerbose(true),
	)
	require.NoError(t, err)
	defer ch.Close()

	err = ch.Execute([]string{"bash", scriptPath})
	assert.NoError(t, err, "Script execution should succeed when commands are allowed")
}

// TestE2E_HookAllowExecution tests allowing commands to execute
func TestE2E_HookAllowExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	testHook := newTestHook("test-allow", []string{"printf"})
	testHook.allowAll() // Allow all commands

	// Create a script using printf (which should be allowed)
	scriptContent := `#!/usr/bin/env bash
printf "allow-test-output"
`
	scriptPath := createTestScript(t, scriptContent)

	ch, err := New(
		WithHook(testHook),
		WithWrapperPath([]string{"go", "run", "../../cmd/cmdhooks"}),
	)
	require.NoError(t, err)
	defer ch.Close()

	err = ch.Execute([]string{"bash", scriptPath})
	assert.NoError(t, err, "Script should execute successfully when commands are allowed")
}

// TestE2E_HookBlockExecution tests blocking commands from executing
func TestE2E_HookBlockExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	testHook := newTestHook("test-block", []string{"sleep"})
	testHook.blockCommand("sleep") // Block sleep specifically

	// Create a script that tries to use sleep - when blocked, should exit quickly
	scriptContent := `#!/usr/bin/env bash
echo "Before sleep"
sleep 1
echo "After sleep"
`
	scriptPath := createTestScript(t, scriptContent)

	ch, err := New(
		WithHook(testHook),
		WithWrapperPath([]string{"go", "run", "../../cmd/cmdhooks"}),
	)
	require.NoError(t, err)
	defer ch.Close()

	err = ch.Execute([]string{"bash", scriptPath})
	// Script should still complete even if sleep is blocked
	assert.NoError(t, err, "Script should complete even when commands are blocked")
}

// TestE2E_ScriptWithMultipleCommands tests a script with multiple intercepted commands
func TestE2E_ScriptWithMultipleCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	testHook := newTestHook("test-multi", []string{"echo", "printf"})

	// Create a script with multiple commands that should be intercepted
	scriptContent := `#!/usr/bin/env bash
echo "Starting multi-command script"
echo "first command"
printf "second command"
echo "Script finished"
`
	scriptPath := createTestScript(t, scriptContent)

	ch, err := New(
		WithHook(testHook),
		WithWrapperPath([]string{"go", "run", "../../cmd/cmdhooks"}),
	)
	require.NoError(t, err)
	defer ch.Close()

	err = ch.Execute([]string{"bash", scriptPath})
	assert.NoError(t, err, "Script with multiple commands should execute successfully")
}

// TestE2E_ResourceCleanup tests that resources are properly cleaned up
func TestE2E_ResourceCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock") // Shorter name

	testHook := newTestHook("test-cleanup", []string{"echo"})

	var socketPath string

	// Create a simple script
	scriptContent := `#!/usr/bin/env bash
echo "cleanup test"
`
	scriptPath := createTestScript(t, scriptContent)

	func() {
		ch, err := New(
			WithHook(testHook),
			WithWrapperPath([]string{"go", "run", "../../cmd/cmdhooks"}),
			WithSocketPath(testSocketPath),
		)
		require.NoError(t, err)

		// Capture paths for later verification
		socketPath = ch.config.SocketPath

		// Execute the script
		err = ch.Execute([]string{"bash", scriptPath})
		assert.NoError(t, err)

		// Close should clean up resources
		err = ch.Close()
		assert.NoError(t, err)
	}()

	// Verify socket was cleaned up
	_, err := os.Stat(socketPath)
	assert.True(t, os.IsNotExist(err), "Socket file should be cleaned up: %s", socketPath)

	// Wrapper directory cleanup is handled by the temporary directory system,
	// so we don't need to explicitly check for it
}

// TestE2E_WrapperPathLookup tests that the wrapper path is correctly resolved
func TestE2E_WrapperPathLookup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	testHook := newTestHook("test-wrapper-path", []string{"echo"})

	// Get the current working directory to build the path
	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Build path to cmd/cmdhooks relative to repo root
	cmdPath := filepath.Join(cwd, "..", "..", "cmd", "cmdhooks")

	// Verify the directory exists
	_, err = os.Stat(cmdPath)
	require.NoError(t, err, "cmd/cmdhooks directory should exist at %s", cmdPath)

	scriptContent := `#!/usr/bin/env bash
echo "wrapper path test"
`
	scriptPath := createTestScript(t, scriptContent)

	ch, err := New(
		WithHook(testHook),
		WithWrapperPath([]string{"go", "run", cmdPath}),
	)
	require.NoError(t, err)
	defer ch.Close()

	err = ch.Execute([]string{"bash", scriptPath})
	assert.NoError(t, err, "Script should execute successfully with custom wrapper path")
}
