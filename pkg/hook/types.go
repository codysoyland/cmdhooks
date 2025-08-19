package hook

import (
	"time"
)

// HookType represents the type of hook being evaluated
type HookType string

const (
	HookPreRun  HookType = "pre_run"  // Before execution
	HookPostRun HookType = "post_run" // After execution
)

// Request represents a complete request to be evaluated by hooks
// This consolidates all request information in a single type
type Request struct {
	// Core request fields
	Command []string `json:"command"` // [0] = command, [1:] = args
	PID     int      `json:"pid"`

	// Hook context
	Hook HookType `json:"hook"`

	// Post-run fields (only populated for post_run hooks)
	ExitCode int           `json:"exit_code,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`

	// Additional metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Response represents the result of a hook evaluation
type Response struct {
	Exit     bool                   `json:"exit,omitempty"`     // If true, command the process tree to be killed
	Metadata map[string]interface{} `json:"metadata,omitempty"` // Metadata to be merged into subsequent requests
}
