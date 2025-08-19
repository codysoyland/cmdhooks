# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

CmdHooks is a Go library and CLI tool for intercepting and controlling commands during script execution. It provides a pluggable hook system for request evaluation and can be embedded into existing Go applications. Think of it like admission control for command execution.

Important security note: This tool is not designed to protect you against malicious commands or scripts. Its goal is to provide greater transparency and control over command execution. Always review and validate commands before execution.

## Build & Development Commands

```bash
# Run all tests
make test

# Run tests with coverage report
make test-coverage
make test-coverage-html  # Generates HTML coverage report

# Code quality
make fmt                 # Format code
make lint                # Run golangci-lint
```

## Testing Strategy

- Use table-driven tests for functions with multiple scenarios
- Unit tests focus on individual components and use mocks for dependencies
- Use `testify` assertions for cleaner test code

## Code Architecture

### Core Components

- **pkg/cmdhooks**: Main library interface with `Execute()` and `New()` functions
- **pkg/hook**: Pluggable hook system implementing the `Hook` interface
- **pkg/interceptor**: IPC request interception engine
- **pkg/executor**: Script execution environment
- **pkg/wrapper**: Command wrapper generation for curl/wget interception

### Hook System Design

The library uses a pluggable architecture where hooks implement:
```go
type Hook interface {
    // Name returns a human-readable name for this hook
    Name() string

    // Commands returns the list of commands this hook handles
    Commands() []string
}

type LocalHook interface {
    Hook
    // EvaluateLocal runs within the wrapper process
    EvaluateLocal(ctx context.Context, req *Request) (*Response, error)
}

type IPCHook interface {
    Hook
    // EvaluateIPC runs in the host process and communicates over IPC
    EvaluateIPC(ctx context.Context, req *Request) (*Response, error)
}
```

### Library Usage

The `pkg/cmdhooks` package provides the main entry point for executing scripts with command interception:

```go
package main

import (
    "github.com/codysoyland/cmdhooks/pkg/cmdhooks"
    "github.com/codysoyland/cmdhooks/pkg/hook"
)

func main() {
    // Create a hook instance (this example shows a hypothetical LogOnlyHook)
    logHook := &LogOnlyHook{
        name:     "audit-logger",
        commands: []string{"curl", "wget", "ssh"},
    }

    // Create CmdHooks instance with functional options
    ch, err := cmdhooks.New(
        cmdhooks.WithHook(logHook),
        cmdhooks.WithVerbose(true),
        cmdhooks.WithSocketPath("/tmp/cmdhooks.sock"), // Optional: for IPC hooks
    )
    if err != nil {
        panic(err)
    }
    defer ch.Close()

    // Execute a script with command interception
    err = ch.Execute("bash", "-c", "curl https://example.com/script.sh | bash")
    if err != nil {
        panic(err)
    }
}
```

#### Hook Implementation Examples

**LocalHook Implementation (runs in wrapper process):**
```go
type MyLocalHook struct {
    name     string
    commands []string
}

func (h *MyLocalHook) Name() string {
    return h.name
}

func (h *MyLocalHook) Commands() []string {
    return h.commands
}

func (h *MyLocalHook) EvaluateLocal(ctx context.Context, req *hook.Request) (*hook.Response, error) {
    // Fast local evaluation logic
    if strings.Contains(req.Command[0], "dangerous") {
        return &hook.Response{Exit: true}, nil
    }
    
    // Continue with metadata for IPC
    return &hook.Response{
        Metadata: map[string]interface{}{
            "local_check": "passed",
            "timestamp":   time.Now().Unix(),
        },
    }, nil
}
```

**IPCHook Implementation (runs in interceptor process):**
```go
type MyIPCHook struct {
    name     string
    commands []string
}

func (h *MyIPCHook) Name() string {
    return h.name
}

func (h *MyIPCHook) Commands() []string {
    return h.commands
}

func (h *MyIPCHook) EvaluateIPC(ctx context.Context, req *hook.Request) (*hook.Response, error) {
    // Access metadata from LocalHook (if present)
    if localCheck, ok := req.Metadata["local_check"]; ok {
        log.Printf("Local hook result: %v", localCheck)
    }
    
    // Perform centralized policy evaluation
    if isBlockedDomain(extractDomain(req.Command)) {
        return &hook.Response{Exit: true}, nil
    }
    
    return &hook.Response{}, nil
}
```

#### Execution Flow

1. **Instantiation**: Create hooks and CmdHooks instance with `cmdhooks.New()`
2. **Setup**: CmdHooks automatically creates wrappers, Unix socket, and modifies PATH
3. **Script Execution**: Call `Execute()` with your script command
4. **Command Interception**: Any intercepted commands trigger hook evaluation:
   - LocalHook evaluates first (if present)
   - If LocalHook doesn't exit, metadata is merged and sent to IPC
   - IPCHook evaluates with enriched context
5. **Response**: Commands continue if `Exit=false`, blocked if `Exit=true`
6. **Cleanup**: Call `Close()` to clean up resources

### Library vs CLI Structure

- **Library API**: Functional options pattern in `pkg/cmdhooks`
- **Examples**: Multiple CLI applications demonstrating different hook strategies
- **Internal**: Private implementation details not exposed in public API

## System Architecture - Command Interception & IPC

### High-Level Overview

CmdHooks intercepts command execution through PATH manipulation and evaluates commands using pluggable hooks before allowing or blocking execution.

### Detailed Setup & Execution Flow

**Setup Phase:**
1. CmdHooks.Execute() starts Unix socket for IPC communication
2. Creates temporary directory for wrapper scripts
3. Generates wrapper scripts for monitored commands
4. Modifies PATH to prepend wrapper directory

**Execution Phase:**
1. Script execution begins
2. When monitored commands are called, wrappers intercept
3. LocalHook evaluates first (fast, in-process)
4. If LocalHook allows, returned metadata is merged and sent via IPC
5. IPCHook evaluates with enriched context
6. Based on Exit field, commands continue or are blocked
7. Real commands execute if allowed
8. Post-run hooks may perform additional evaluation

**Cleanup Phase:**
1. Script completes execution
2. Temporary wrapper files are removed
3. Unix socket is closed and cleaned up

### IPC Communication Protocol

The wrapper processes communicate with the interceptor using JSON messages over Unix domain sockets:

**Request Flow:**
1. Wrapper connects to Unix socket
2. Sends JSON request with command details and metadata
3. Interceptor receives and parses request
4. Hook evaluates the request context
5. Response is sent back with exit response
6. Wrapper either continues with real command or exits

**Message Format:**
- Request: `{"command":["curl","url"],"pid":123,"hook":"pre_run","metadata":{...}}`
- Response: `{"exit":false,"metadata":{...}}`

If execution continues, post-run hooks may perform additional evaluation after command completion.

### Detailed Component Interaction

#### 1. **Wrapper Injection Mechanism**
The system creates wrapper shell scripts that masquerade as system commands:

```bash
# Example wrapper at /tmp/cmdhooks-wrappers-*/curl
#!/usr/bin/env bash
exec "/path/to/cmdhooks-binary" run curl "$@"
```

These wrappers are placed in a temporary directory that's prepended to PATH:
- Original PATH: `/usr/bin:/bin`
- Modified PATH: `/tmp/cmdhooks-wrappers-123:/usr/bin:/bin`

#### 2. **Request/Response Protocol**
The IPC uses JSON messages over Unix domain sockets:

**Request Structure:**
```json
{
  "tool": "curl",
  "args": ["https://api.example.com"],
  "pid": 12345,
  "hook": "pre-run",
  "metadata": {
    "url": "https://api.example.com",
    "timestamp": "2024-01-01T00:00:00Z"
  }
}
```

**Response Structure:**
```json
{
  "exit": false
}
```

#### 3. **Hook Evaluation Priority**
The system supports both local (in-process) and remote (via IPC) hook evaluation:

1. **Local Hooks** (wrapper process) - Fast, no IPC overhead
2. **Remote Hooks** (interceptor process) - Centralized policy enforcement

The evaluation follows this priority:
- Local hooks evaluate first
- If denied locally, execution stops
- If allowed locally, continues to IPC evaluation
- Final response based on combined evaluation

#### 4. **Security Considerations**

- **Socket Permissions**: Unix domain socket created with `0600` (owner read/write only)
- **Temporary Files**: Wrapper scripts created with `0700` permissions
- **Cleanup**: Automatic removal of temporary files and sockets on exit

## Development Guidelines

- Follow standard Go project layout and Effective Go principles
- Use functional options pattern for configuration
- Examples should be created in `examples/` directory
- Library code must be in `pkg/` directory
