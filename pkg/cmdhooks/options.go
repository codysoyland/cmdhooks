package cmdhooks

import (
	"github.com/codysoyland/cmdhooks/pkg/hook"
)

// WithHook sets the hook for request evaluation
func WithHook(h hook.Hook) Option {
	return func(c *Config) error {
		c.Hook = h
		return nil
	}
}

// WithVerbose enables or disables verbose output
func WithVerbose(v bool) Option {
	return func(c *Config) error {
		c.Verbose = v
		return nil
	}
}

// WithSocketPath sets the Unix socket path for IPC
func WithSocketPath(path string) Option {
	return func(c *Config) error {
		c.SocketPath = path
		return nil
	}
}

// WithWrapperPath sets the command path for wrapper generation
func WithWrapperPath(wrapperPath []string) Option {
	return func(c *Config) error {
		c.WrapperPath = wrapperPath
		return nil
	}
}
