package cmdhooks

import (
	"fmt"
	"github.com/codysoyland/cmdhooks/pkg/hook"
	"strings"
	"time"
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

// WithWrapperPath sets the command used by generated wrapper scripts.
// Expectation: provide the binary and its subcommand/args, e.g.:
//   - []string{"cmdhooks", "run"}
//   - []string{"go", "run", "./cmd/cmdhooks"}
//
// The slice must be non-empty and contain no empty elements.
func WithWrapperPath(wrapperPath []string) Option {
	return func(c *Config) error {
		if len(wrapperPath) == 0 {
			return fmt.Errorf("WithWrapperPath: wrapper command cannot be empty; provide binary and subcommand (e.g., ['cmdhooks','run'] or ['go','run','./cmd/cmdhooks'])")
		}
		for i, part := range wrapperPath {
			if strings.TrimSpace(part) == "" {
				return fmt.Errorf("WithWrapperPath: argument %d is empty; provide non-empty command parts", i)
			}
		}
		c.WrapperPath = wrapperPath
		return nil
	}
}

// WithInterceptorTimeout configures the IPC evaluation timeout (e.g., 5*time.Second).
func WithInterceptorTimeout(d time.Duration) Option {
	return func(c *Config) error {
		c.InterceptorTimeout = d
		return nil
	}
}
