// Package executor provides script execution environment with PATH manipulation
// and network request interception capabilities.
package executor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Executor manages script execution with network interception
type Executor struct {
	command     []string
	socketPath  string
	wrapperPath string
	verbose     bool         // Verbose mode flag
	process     *exec.Cmd    // The running process
	mu          sync.RWMutex // Protects process access
}

// New creates a new executor instance
func New(command []string, socketPath string) *Executor {
	return &Executor{
		command:    command,
		socketPath: socketPath,
	}
}

// SetWrapperPath sets the directory containing wrapper binaries
func (s *Executor) SetWrapperPath(path string) {
	s.wrapperPath = path
}

// SetVerbose sets the verbose mode flag
func (s *Executor) SetVerbose(verbose bool) {
	s.verbose = verbose
}

// Execute runs the command in the executor environment
func (s *Executor) Execute() error {
	if s.wrapperPath == "" {
		return fmt.Errorf("wrapper path not set")
	}

	if len(s.command) == 0 {
		return fmt.Errorf("no command specified")
	}

	cmd := exec.Command(s.command[0], s.command[1:]...)

	// Build environment
	env := os.Environ()
	env = s.modifyPath(env)
	env = append(env, fmt.Sprintf("CMDHOOKS_SOCKET=%s", s.socketPath))
	if s.verbose {
		env = append(env, "CMDHOOKS_VERBOSE=true")
	}
	cmd.Env = env

	// Set up process group for proper tree killing
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group
	}

	// Connect standard streams
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Store process reference for termination
	s.mu.Lock()
	s.process = cmd
	s.mu.Unlock()

	err := cmd.Run()

	// Clear process reference after execution
	s.mu.Lock()
	s.process = nil
	s.mu.Unlock()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("execution exited with code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("failed to execute: %w", err)
	}

	return nil
}

// modifyPath prepends the wrapper directory to PATH
func (s *Executor) modifyPath(env []string) []string {
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			currentPath := strings.TrimPrefix(e, "PATH=")
			newPath := s.wrapperPath + ":" + currentPath
			env[i] = "PATH=" + newPath
			return env
		}
	}

	// If PATH not found, create it
	env = append(env, "PATH="+s.wrapperPath+":/usr/bin:/bin:/usr/local/bin")
	return env
}

// Cleanup removes temporary wrapper files and directories
func (s *Executor) Cleanup() error {
	if s.wrapperPath != "" {
		// Try to remove the wrapper directory, but don't fail if there are permission issues
		// macOS sometimes creates system files we can't delete
		if err := os.RemoveAll(s.wrapperPath); err != nil {
			// Return nil instead of the error - cleanup failures shouldn't break the application
			return nil
		}
	}
	return nil
}

// KillProcessTree terminates the executor process and all its children
func (s *Executor) KillProcessTree() error {
	s.mu.RLock()
	process := s.process
	s.mu.RUnlock()

	if process == nil || process.Process == nil {
		// Process not started or already finished
		return nil
	}

	pid := process.Process.Pid

	// First try graceful termination (SIGTERM) to the entire process group
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		// If we can't kill the group, try killing just the main process
		if killErr := process.Process.Kill(); killErr != nil {
			return fmt.Errorf("failed to kill process %d: %w", pid, killErr)
		}
		return nil
	}

	// Give the process group 5 seconds to terminate gracefully
	done := make(chan error, 1)
	go func() {
		done <- process.Wait()
	}()

	select {
	case <-done:
		// Process terminated gracefully
		return nil
	case <-time.After(5 * time.Second):
		// Timeout - force kill the entire process group
		if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
			// If group kill fails, force kill the main process
			return process.Process.Kill()
		}
		// Wait a bit more for forced termination
		select {
		case <-done:
			return nil
		case <-time.After(2 * time.Second):
			return fmt.Errorf("process %d failed to terminate after SIGKILL", pid)
		}
	}
}

// IsRunning returns true if the executor process is currently running
func (s *Executor) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.process == nil || s.process.Process == nil {
		return false
	}

	// Check if process is still alive
	err := s.process.Process.Signal(syscall.Signal(0))
	return err == nil
}
