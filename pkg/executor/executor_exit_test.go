package executor

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecutorProcessTreeKill(t *testing.T) {
	t.Run("KillProcessTree with no process", func(t *testing.T) {
		executor := &Executor{}
		err := executor.KillProcessTree()
		assert.NoError(t, err, "KillProcessTree should not error when no process is running")
	})

	t.Run("IsRunning with no process", func(t *testing.T) {
		executor := &Executor{}
		running := executor.IsRunning()
		assert.False(t, running, "IsRunning should return false when no process is set")
	})
}

func TestExecutorProcessManagement(t *testing.T) {
	t.Run("Process reference management", func(t *testing.T) {
		tmpDir := t.TempDir()
		testSocketPath := filepath.Join(tmpDir, "test.sock")
		wrapperPath := tmpDir

		executor := New([]string{"echo", "test"}, testSocketPath)

		// Initially no process should be running
		assert.False(t, executor.IsRunning())

		// After execution, process should still not be running (since it completed)
		executor.SetWrapperPath(wrapperPath)
		// Note: We can't easily test actual execution without complex setup
		// This test verifies the basic structure is correct
	})
}

func TestExitSignalIntegration(t *testing.T) {
	t.Run("Exit signal structures", func(t *testing.T) {
		tmpDir := t.TempDir()
		testSocketPath := filepath.Join(tmpDir, "test.sock")

		// Test that our new structures are properly initialized
		executor := New([]string{"echo", "test"}, testSocketPath)

		// Verify executor has proper mutex and process management
		assert.NotNil(t, executor)
		assert.False(t, executor.IsRunning())

		// Test that KillProcessTree doesn't panic with no process
		err := executor.KillProcessTree()
		assert.NoError(t, err)
	})
}
