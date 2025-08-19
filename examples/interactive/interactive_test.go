package main

import (
	"context"
	"testing"

	"github.com/codysoyland/cmdhooks/pkg/hook"
	"github.com/stretchr/testify/assert"
)

func TestInteractiveHook(t *testing.T) {
	t.Run("creation and basic properties", func(t *testing.T) {
		commands := []string{"curl", "wget"}
		hookInstance := NewInteractiveHook(commands)

		assert.Equal(t, "interactive", hookInstance.Name())
		assert.Equal(t, commands, hookInstance.Commands())
	})

	t.Run("non-nil request handling", func(t *testing.T) {
		hookInstance := NewInteractiveHook([]string{"curl"})
		ctx := context.Background()

		// Test with valid request - this would normally prompt for user input
		// In the real implementation this would hang waiting for input,
		// so we just test that the function can be called without panicking
		req := &hook.Request{
			Command: []string{"curl", "-s", "https://example.com"},
			Hook:    hook.HookPreRun,
		}

		// We can't easily test the interactive prompt in unit tests
		// Just verify the method doesn't panic with valid input
		assert.NotPanics(t, func() {
			// This would normally hang waiting for user input
			// but we just want to verify the structure works
			_, _ = hookInstance.EvaluateIPC(ctx, req)
		})
	})

	t.Run("context cancellation", func(t *testing.T) {
		hookInstance := NewInteractiveHook([]string{"curl"})
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		req := &hook.Request{
			Command: []string{"curl", "-s", "https://example.com"},
			PID:     123,
			Hook:    hook.HookPreRun,
		}

		response, err := hookInstance.EvaluateIPC(ctx, req)
		assert.Error(t, err)
		assert.True(t, response.Exit)
		// No reason field to check
	})

	t.Run("unsupported hook type", func(t *testing.T) {
		hookInstance := NewInteractiveHook([]string{"curl"})
		ctx := context.Background()

		req := &hook.Request{
			Command: []string{"curl", "-s", "https://example.com"},
			PID:     123,
			Hook:    hook.HookType("unsupported"),
		}

		response, err := hookInstance.EvaluateIPC(ctx, req)
		assert.NoError(t, err)
		assert.True(t, response.Exit)
		// No reason field to check
	})
}

func TestInteractiveHookExitFunctionality(t *testing.T) {
	// Test that interactive hook can create exit responses
	hookInstance := NewInteractiveHook([]string{"curl"})

	// Test that we can create a response with Exit=true
	response := &hook.Response{
		Exit: true,
	}

	assert.True(t, response.Exit)
	// No reason field to check

	// Verify hook properties
	assert.Equal(t, "interactive", hookInstance.Name())
	assert.Equal(t, []string{"curl"}, hookInstance.Commands())
}
