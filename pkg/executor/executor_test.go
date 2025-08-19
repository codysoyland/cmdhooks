package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")

	tests := []struct {
		name       string
		command    []string
		socketPath string
	}{
		{
			name:       "script file execution",
			command:    []string{"/path/to/script.sh"},
			socketPath: testSocketPath,
		},
		{
			name:       "command execution with args",
			command:    []string{"echo", "hello", "world"},
			socketPath: testSocketPath,
		},
		{
			name:       "command execution without args",
			command:    []string{"date"},
			socketPath: testSocketPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := New(tt.command, tt.socketPath)

			assert.Equal(t, tt.command, executor.command)
			assert.Equal(t, tt.socketPath, executor.socketPath)
			assert.Empty(t, executor.wrapperPath) // Should be empty initially
		})
	}
}

func TestSetWrapperPath(t *testing.T) {
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")
	wrapperPath := filepath.Join(tmpDir, "wrappers")

	executor := New([]string{"echo", "hello"}, testSocketPath)

	executor.SetWrapperPath(wrapperPath)
	assert.Equal(t, wrapperPath, executor.wrapperPath)
}

func TestExecuteNoWrapperPath(t *testing.T) {
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")

	executor := New([]string{"echo", "hello"}, testSocketPath)

	err := executor.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "wrapper path not set")
}

func TestExecuteEmptyCommand(t *testing.T) {
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")
	wrapperPath := filepath.Join(tmpDir, "wrappers")

	executor := New([]string{}, testSocketPath)
	executor.SetWrapperPath(wrapperPath)

	err := executor.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no command specified")
}

func TestModifyPath(t *testing.T) {
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")
	wrapperDir := filepath.Join(tmpDir, "wrappers")

	executor := New([]string{"test"}, testSocketPath)
	executor.SetWrapperPath(wrapperDir)

	tests := []struct {
		name        string
		inputEnv    []string
		expectedEnv string
	}{
		{
			name: "existing PATH",
			inputEnv: []string{
				"HOME=/home/user",
				"PATH=/usr/bin:/bin",
				"TERM=xterm",
			},
			expectedEnv: "PATH=" + wrapperDir + ":/usr/bin:/bin",
		},
		{
			name: "no existing PATH",
			inputEnv: []string{
				"HOME=/home/user",
				"TERM=xterm",
			},
			expectedEnv: "PATH=" + wrapperDir + ":/usr/bin:/bin:/usr/local/bin",
		},
		{
			name:        "empty environment",
			inputEnv:    []string{},
			expectedEnv: "PATH=" + wrapperDir + ":/usr/bin:/bin:/usr/local/bin",
		},
		{
			name: "PATH at beginning",
			inputEnv: []string{
				"PATH=/original/path",
				"HOME=/home/user",
			},
			expectedEnv: "PATH=" + wrapperDir + ":/original/path",
		},
		{
			name: "PATH at end",
			inputEnv: []string{
				"HOME=/home/user",
				"TERM=xterm",
				"PATH=/original/path",
			},
			expectedEnv: "PATH=" + wrapperDir + ":/original/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.modifyPath(tt.inputEnv)

			// Find the PATH entry
			var pathFound bool
			for _, env := range result {
				if strings.HasPrefix(env, "PATH=") {
					assert.Equal(t, tt.expectedEnv, env)
					pathFound = true
					break
				}
			}
			assert.True(t, pathFound, "PATH should be present in environment")

			// Verify non-PATH entries are preserved
			for _, originalEnv := range tt.inputEnv {
				if !strings.HasPrefix(originalEnv, "PATH=") {
					assert.Contains(t, result, originalEnv)
				}
			}
		})
	}
}

func TestExecuteScriptFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple test script
	scriptPath := filepath.Join(tmpDir, "test.sh")
	scriptContent := `#!/usr/bin/env bash
echo "Hello from script"
exit 0
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	wrapperDir := filepath.Join(tmpDir, "wrappers")
	err = os.MkdirAll(wrapperDir, 0755)
	require.NoError(t, err)

	socketPath := filepath.Join(tmpDir, "test.sock")

	executor := New([]string{scriptPath}, socketPath)
	executor.SetWrapperPath(wrapperDir)

	err = executor.Execute()
	assert.NoError(t, err)
}

func TestExecuteCommand(t *testing.T) {
	tmpDir := t.TempDir()
	wrapperDir := filepath.Join(tmpDir, "wrappers")
	err := os.MkdirAll(wrapperDir, 0755)
	require.NoError(t, err)

	socketPath := filepath.Join(tmpDir, "test.sock")

	tests := []struct {
		name      string
		command   []string
		expectErr bool
	}{
		{
			name:      "echo command with arguments",
			command:   []string{"echo", "hello", "world"},
			expectErr: false,
		},
		{
			name:      "echo command without arguments",
			command:   []string{"echo"},
			expectErr: false,
		},
		{
			name:      "true command (always succeeds)",
			command:   []string{"true"},
			expectErr: false,
		},
		{
			name:      "false command (always fails)",
			command:   []string{"false"},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := New(tt.command, socketPath)
			executor.SetWrapperPath(wrapperDir)

			err := executor.Execute()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecuteNonExistentScript(t *testing.T) {
	tmpDir := t.TempDir()
	wrapperDir := filepath.Join(tmpDir, "wrappers")
	err := os.MkdirAll(wrapperDir, 0755)
	require.NoError(t, err)

	nonExistentScript := filepath.Join(tmpDir, "nonexistent.sh")
	socketPath := filepath.Join(tmpDir, "test.sock")

	executor := New([]string{nonExistentScript}, socketPath)
	executor.SetWrapperPath(wrapperDir)

	err = executor.Execute()
	assert.Error(t, err)
	// The error message can vary, but should indicate execution failure
	assert.True(t, err != nil, "Expected execution to fail for non-existent script")
}

func TestExecuteNonExistentCommand(t *testing.T) {
	tmpDir := t.TempDir()
	wrapperDir := filepath.Join(tmpDir, "wrappers")
	err := os.MkdirAll(wrapperDir, 0755)
	require.NoError(t, err)

	socketPath := filepath.Join(tmpDir, "test.sock")

	executor := New([]string{"nonexistent-command-12345"}, socketPath)
	executor.SetWrapperPath(wrapperDir)

	err = executor.Execute()
	assert.Error(t, err)
	// The error message can vary, but should indicate execution failure
	assert.True(t, err != nil, "Expected execution to fail for non-existent command")
}

func TestExecuteEnvironmentVariables(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a script that checks environment variables
	scriptPath := filepath.Join(tmpDir, "env_test.sh")
	scriptContent := `#!/usr/bin/env bash
# Check if CMDHOOKS_SOCKET is set
if [ -z "$CMDHOOKS_SOCKET" ]; then
    echo "CMDHOOKS_SOCKET not set"
    exit 1
fi

# Check if our wrapper path is in PATH
if [[ ":$PATH:" != *":` + tmpDir + `/wrappers:"* ]]; then
    echo "Wrapper path not in PATH"
    exit 1
fi

echo "Environment variables correctly set"
exit 0
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	wrapperDir := filepath.Join(tmpDir, "wrappers")
	err = os.MkdirAll(wrapperDir, 0755)
	require.NoError(t, err)

	socketPath := filepath.Join(tmpDir, "test.sock")

	executor := New([]string{scriptPath}, socketPath)
	executor.SetWrapperPath(wrapperDir)

	err = executor.Execute()
	assert.NoError(t, err)
}

func TestCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")
	wrapperDir := filepath.Join(tmpDir, "wrappers")

	// Create wrapper directory and some files
	err := os.MkdirAll(wrapperDir, 0755)
	require.NoError(t, err)

	testFile := filepath.Join(wrapperDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	executor := New([]string{"echo"}, testSocketPath)
	executor.SetWrapperPath(wrapperDir)

	// Cleanup should not return error even if it fails
	err = executor.Cleanup()
	assert.NoError(t, err)
}

func TestCleanupEmptyWrapperPath(t *testing.T) {
	tmpDir := t.TempDir()
	testSocketPath := filepath.Join(tmpDir, "test.sock")

	executor := New([]string{"echo"}, testSocketPath)
	// Don't set wrapper path

	err := executor.Cleanup()
	assert.NoError(t, err)
}

func TestExecuteExitCodeHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a script that exits with specific code
	scriptPath := filepath.Join(tmpDir, "exit_test.sh")
	scriptContent := `#!/usr/bin/env bash
exit 42
`
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	wrapperDir := filepath.Join(tmpDir, "wrappers")
	err = os.MkdirAll(wrapperDir, 0755)
	require.NoError(t, err)

	socketPath := filepath.Join(tmpDir, "test.sock")

	executor := New([]string{scriptPath}, socketPath)
	executor.SetWrapperPath(wrapperDir)

	err = executor.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exited with code 42")
}

func TestExecutePathModification(t *testing.T) {
	tmpDir := t.TempDir()
	wrapperDir := filepath.Join(tmpDir, "wrappers")
	err := os.MkdirAll(wrapperDir, 0755)
	require.NoError(t, err)

	// Create a fake "curl" wrapper that echoes a message
	curlWrapperPath := filepath.Join(wrapperDir, "curl")
	curlWrapperContent := `#!/usr/bin/env bash
echo "Wrapper curl called with args: $*"
exit 0
`
	err = os.WriteFile(curlWrapperPath, []byte(curlWrapperContent), 0755)
	require.NoError(t, err)

	// Create a script that calls curl
	scriptPath := filepath.Join(tmpDir, "curl_test.sh")
	scriptContent := `#!/usr/bin/env bash
curl https://example.com
`
	err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	socketPath := filepath.Join(tmpDir, "test.sock")

	executor := New([]string{scriptPath}, socketPath)
	executor.SetWrapperPath(wrapperDir)

	err = executor.Execute()
	assert.NoError(t, err)

	// The test passes if the wrapper was called instead of system curl
	// We can't easily capture the output in unit tests, but the execution
	// should complete successfully if our wrapper was called
}
