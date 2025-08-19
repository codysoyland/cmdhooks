// Package hook provides a pluggable system for evaluating network requests
// during script execution with various built-in and custom hook implementations.
package hook

import (
	"context"
)

// Hook defines the base interface with common functionality
type Hook interface {
	// Name returns a human-readable name for this hook
	Name() string

	// Commands returns the list of commands this hook handles
	Commands() []string
}

// LocalHook embeds Hook and adds local evaluation capability
type LocalHook interface {
	Hook
	// EvaluateLocal runs within the wrapper process
	EvaluateLocal(ctx context.Context, req *Request) (*Response, error)
}

// IPCHook embeds Hook and adds IPC-based evaluation capability
type IPCHook interface {
	Hook
	// EvaluateIPC runs in the host process and communicates over IPC
	EvaluateIPC(ctx context.Context, req *Request) (*Response, error)
}
