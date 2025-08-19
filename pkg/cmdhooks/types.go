package cmdhooks

import (
	"github.com/codysoyland/cmdhooks/pkg/executor"
	"github.com/codysoyland/cmdhooks/pkg/hook"
	"github.com/codysoyland/cmdhooks/pkg/interceptor"
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
	Verbose     bool
	SocketPath  string
	WrapperPath []string
	Hook        hook.Hook
}

// Option represents a functional option for configuration
type Option func(*Config) error
