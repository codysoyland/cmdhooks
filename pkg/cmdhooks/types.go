package cmdhooks

import (
	"github.com/codysoyland/cmdhooks/pkg/executor"
	"github.com/codysoyland/cmdhooks/pkg/hook"
	"github.com/codysoyland/cmdhooks/pkg/interceptor"
	"time"
)

// CmdHooks represents the main library instance
type CmdHooks struct {
	config      *Config
	interceptor *interceptor.Interceptor
	executor    *executor.Executor
	hook        hook.Hook
}

// Config holds all configuration options
type Config struct {
	Verbose    bool
	SocketPath string
	// WrapperPath defines the executable and arguments used by generated
	// wrappers to invoke the real command handler. Provide binary and
	// subcommand/args, for example: ["cmdhooks", "run"] or
	// ["go", "run", "./cmd/cmdhooks"]. Must be non-empty.
	WrapperPath []string
	Hook        hook.Hook
	// InterceptorTimeout bounds IPC evaluation inside the interceptor process.
	// Defaults to 10m if zero.
	InterceptorTimeout time.Duration
}

// Option represents a functional option for configuration
type Option func(*Config) error
