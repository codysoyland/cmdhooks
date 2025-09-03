// Package cmdhooks provides a library for intercepting and controlling command execution
// during script execution with pluggable hook policies.
package cmdhooks

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/codysoyland/cmdhooks/pkg/executor"
	"github.com/codysoyland/cmdhooks/pkg/hook"
	"github.com/codysoyland/cmdhooks/pkg/interceptor"
)

// validateCommand checks if a command slice is valid (non-empty)
func validateCommand(cmd []string) error {
	if len(cmd) == 0 {
		return fmt.Errorf("command cannot be empty")
	}
	return nil
}

// Execute runs a script with the provided options (simple API)
// This is the primary function for most use cases.
func Execute(cmd []string, opts ...Option) error {
	ch, err := New(opts...)
	if err != nil {
		return err
	}
	defer ch.Close()

	return ch.Execute(cmd)
}

// New creates a new CmdHooks instance
func New(opts ...Option) (*CmdHooks, error) {
	// Create default config
	config := &Config{
		Verbose:    false,
		SocketPath: "",
		Hook:       nil,
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(config); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	if config.Hook == nil {
		return nil, fmt.Errorf("must provide hook")
	}

	// Always create interceptor for consistent behavior
	if config.SocketPath == "" {
		// Use a unique, securely generated temp filename for the Unix socket.
		// We create and immediately remove the file so net.Listen can bind the path.
		f, err := os.CreateTemp("", "cmdhooks-*.sock")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp socket path: %w", err)
		}
		path := f.Name()
		_ = f.Close()
		_ = os.Remove(path)
		config.SocketPath = path
	}
    i := interceptor.New(config.SocketPath, config.Verbose, config.Hook)
    // Apply timeout as provided; zero/negative means no timeout.
    i.SetEvaluateTimeout(config.InterceptorTimeout)

	return &CmdHooks{
		config:      config,
		interceptor: i,
		hook:        config.Hook,
	}, nil
}

// ExecuteScript executes a script with CmdHooks interception
func (c *CmdHooks) Execute(cmd []string) error {
	if err := validateCommand(cmd); err != nil {
		return err
	}

	// Setup execution environment
	sb, cleanup, err := c.setupExecutor(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	if c.config.Verbose {
		log.Printf("[INFO] Starting script execution: %s", cmd[0])
	}

	// Monitor execution with exit signal handling
	return c.execute(sb)
}

// SetHook changes the hook used for request evaluation
func (c *CmdHooks) SetHook(h hook.Hook) {
	c.hook = h
	c.config.Hook = h
	c.interceptor.SetHook(h)
}

// GetHook returns the current hook
func (c *CmdHooks) GetHook() hook.Hook {
	return c.hook
}

// Close cleans up resources
func (c *CmdHooks) Close() error {
	c.interceptor.Stop()

	if c.executor != nil {
		if err := c.executor.Cleanup(); err != nil {
			if c.config.Verbose {
				log.Printf("[ERROR] Failed to cleanup executor: %v", err)
			}
		}
	}

	// Clean up socket file
	if c.config.SocketPath != "" {
		os.Remove(c.config.SocketPath)
	}

	return nil
}

// createWrappers creates temporary wrapper binaries for commands specified by the hook
func (c *CmdHooks) createWrappers() (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "cmdhooks-wrappers-*")
	if err != nil {
		return "", nil, err
	}

	cleanup := func() {
		// Ignore errors when cleaning up temp directory
		// macOS sometimes creates system files we can't delete
		if cleanupErr := os.RemoveAll(tmpDir); cleanupErr != nil {
			// Log the error but don't fail - this is usually just macOS system files
			fmt.Fprintf(os.Stderr, "Warning: failed to cleanup temp directory %s: %v\n", tmpDir, cleanupErr)
		}
	}

	// Determine wrapper command path - use configured path or default to cmdhooks binary
	var wrapperCmd []string
	if len(c.config.WrapperPath) > 0 {
		wrapperCmd = c.config.WrapperPath
	} else {
		// Default to using installed cmdhooks binary
		cmdHooksPath, err := exec.LookPath("cmdhooks")
		if err != nil {
			cleanup()
			return "", nil, fmt.Errorf("cmdhooks binary not found in PATH. Please install it or use WithWrapperPath() to specify a custom wrapper: %w", err)
		}
		wrapperCmd = []string{cmdHooksPath, "run"}
	}

	// Get commands from hook
	commands := c.hook.Commands()
	if len(commands) == 0 {
		// If no commands specified, don't create any wrappers
		return tmpDir, cleanup, nil
	}

	// Create wrapper script for each command
	for _, command := range commands {
		wrapperPath := filepath.Join(tmpDir, command)
		// Build exec command with safe shell quoting: wrapperCmd + command + "$@"
		quotedWrapper := make([]string, 0, len(wrapperCmd))
		for _, part := range wrapperCmd {
			quotedWrapper = append(quotedWrapper, shellQuote(part))
		}
		quotedCommand := shellQuote(command)
		wrapperExec := strings.Join(append(quotedWrapper, quotedCommand), " ") + " \"$@\""

		wrapperScript := fmt.Sprintf(`#!/usr/bin/env bash
# CmdHooks wrapper for %s (defaults to 'cmdhooks run', configurable via WithWrapperPath)
exec %s
`, command, wrapperExec)

		if err := os.WriteFile(wrapperPath, []byte(wrapperScript), 0600); err != nil {
			cleanup()
			return "", nil, err
		}

		// Make wrapper executable
		if err := os.Chmod(wrapperPath, 0700); err != nil {
			cleanup()
			return "", nil, err
		}
	}

	return tmpDir, cleanup, nil
}

// shellQuote returns a shell-safe single-quoted string. It wraps the input in single
// quotes and escapes existing single quotes using the POSIX-safe pattern: '
// becomes '\” inside the quoted string.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	// Replace every single quote ' with '\''
	// This closes the existing quote, inserts an escaped single quote, and reopens the quote.
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// setupExecutor prepares the execution environment
func (c *CmdHooks) setupExecutor(cmd []string) (*executor.Executor, func(), error) {
	// Start interceptor
	if err := c.interceptor.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start interceptor: %w", err)
	}

	// Create executor
	sb := executor.New(cmd, c.config.SocketPath)
	sb.SetVerbose(c.config.Verbose)
	c.executor = sb

	// Create wrapper binaries
	wrapperDir, cleanup, err := c.createWrappers()
	if err != nil {
		c.interceptor.Stop()
		return nil, nil, fmt.Errorf("failed to create wrappers: %w", err)
	}

	sb.SetWrapperPath(wrapperDir)

	// Return cleanup function that handles both interceptor and wrappers
	fullCleanup := func() {
		cleanup()
		c.interceptor.Stop()
	}

	return sb, fullCleanup, nil
}

// execute handles concurrent execution with exit signal monitoring
func (c *CmdHooks) execute(sb *executor.Executor) error {
	// Execute command or script concurrently while monitoring for exit signals
	execDone := make(chan error, 1)
	go func() {
		execDone <- sb.Execute()
	}()

	// Monitor for completion or exit signal
	select {
	case err := <-execDone:
		// Normal completion
		if err != nil {
			return fmt.Errorf("execution failed: %w", err)
		}
		if c.config.Verbose {
			log.Printf("[INFO] Execution completed")
		}
		return nil

	case <-c.interceptor.ExitSignal():
		// Exit signal received - kill process tree
		log.Printf("[INFO] Exit signal received - terminating process tree")

		if err := sb.KillProcessTree(); err != nil {
			log.Printf("[ERROR] Failed to kill process tree: %v", err)
		}

		// Wait for execution to finish (should be quick after kill)
		select {
		case <-execDone:
			// Execution finished after kill
		case <-time.After(5 * time.Second):
			// Timeout waiting for execution to finish
			log.Printf("[ERROR] Timeout waiting for process termination")
		}

		return fmt.Errorf("execution terminated by user request")
	}
}
