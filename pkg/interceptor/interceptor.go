// Package interceptor provides request interception capabilities for command execution
// via Unix domain socket IPC communication with wrapper binaries.
package interceptor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/codysoyland/cmdhooks/pkg/hook"
)

const (
	// MaxIPCMessageBytes is the maximum size allowed for a single IPC message
	// to guard against unbounded memory usage. Requests exceeding this size
	// are rejected with an error.
	MaxIPCMessageBytes = 64 * 1024 // 64 KiB
)

// Interceptor handles IPC communication and request evaluation
type Interceptor struct {
	socketPath string
	verbose    bool
	hook       hook.Hook
	listener   net.Listener
	stop       chan struct{}
	exitSignal chan struct{} // Channel to signal process tree termination
	wg         sync.WaitGroup
	// evaluateTimeout bounds hook evaluations inside the interceptor.
	// Defaults to 10m if zero.
	evaluateTimeout time.Duration
}

// New creates a new interceptor instance
func New(socketPath string, verbose bool, h hook.Hook) *Interceptor {
	// Note: nil hook is allowed. Interceptor will default-allow in IPC stage
	// when no IPCHook is provided (including nil). cmdhooks.New enforces
	// non-nil hooks for library entrypoints, but keeping this robust avoids
	// surprising panics when interceptor is used directly in tests/tools.
	return &Interceptor{
		socketPath:      socketPath,
		verbose:         verbose,
		hook:            h,
		stop:            make(chan struct{}),
		exitSignal:      make(chan struct{}),
		evaluateTimeout: 10 * time.Minute,
	}
}

// ExitSignal returns a channel that will receive a signal when exit is requested
func (i *Interceptor) ExitSignal() <-chan struct{} {
	return i.exitSignal
}

// Start starts the interceptor and begins listening for connections
func (i *Interceptor) Start() error {
	// Remove any existing socket file
	os.Remove(i.socketPath)

	// Create Unix domain socket listener
	listener, err := net.Listen("unix", i.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket listener: %w", err)
	}
	i.listener = listener

	// Set permissions to be restrictive
	if err := os.Chmod(i.socketPath, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	i.wg.Add(1)
	go i.listen()

	return nil
}

// Stop stops the interceptor and cleans up resources
func (i *Interceptor) Stop() {
	select {
	case <-i.stop:
		// Already stopped
		return
	default:
		close(i.stop)
	}

	if i.listener != nil {
		i.listener.Close()
	}
	i.wg.Wait()
	os.Remove(i.socketPath)
}

// SetHook changes the hook used for request evaluation
func (i *Interceptor) SetHook(h hook.Hook) {
	i.hook = h
}

// Hook returns the current hook
func (i *Interceptor) Hook() hook.Hook {
	return i.hook
}

// SetEvaluateTimeout overrides the default evaluation timeout.
func (i *Interceptor) SetEvaluateTimeout(d time.Duration) {
	if d > 0 {
		i.evaluateTimeout = d
	}
}

// listen accepts and handles incoming connections
func (i *Interceptor) listen() {
	defer i.wg.Done()

	for {
		select {
		case <-i.stop:
			return
		default:
		}

		conn, err := i.listener.Accept()
		if err != nil {
			select {
			case <-i.stop:
				return
			default:
				if i.verbose {
					log.Printf("Failed to accept connection: %v", err)
				}
				continue
			}
		}

		// Handle each connection in a goroutine
		i.wg.Add(1)
		go i.handleConnection(conn)
	}
}

// handleConnection processes a single IPC connection
func (i *Interceptor) handleConnection(conn net.Conn) {
	defer i.wg.Done()
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	// Guard against overly large IPC messages
	scanner.Buffer(make([]byte, 0, 64*1024), MaxIPCMessageBytes)
	writer := bufio.NewWriter(conn)

	// Read and parse request
	req, err := readRequest(scanner)
	if err != nil {
		if i.verbose {
			log.Printf("Request read/parse error: %v", err)
		}
		errResp := &hook.Response{
			Exit: true,
		}
		if writeErr := writeResponse(writer, errResp); writeErr != nil {
			if i.verbose {
				log.Printf("Failed to write error response: %v", writeErr)
			}
		}
		return
	}

	// Process request
	resp, err := i.processRequest(req)
	if err != nil {
		if i.verbose {
			log.Printf("Request processing error: %v", err)
		}
		errResp := &hook.Response{
			Exit: true,
		}
		if writeErr := writeResponse(writer, errResp); writeErr != nil {
			if i.verbose {
				log.Printf("Failed to write error response: %v", writeErr)
			}
		}
		return
	}

	// Write response
	if err := writeResponse(writer, resp); err != nil {
		if i.verbose {
			log.Printf("Failed to write response: %v", err)
		}
	}
}

// readRequest reads and unmarshals a JSON request from the scanner
func readRequest(scanner *bufio.Scanner) (*hook.Request, error) {
	if !scanner.Scan() {
		return nil, fmt.Errorf("failed to read request: %v", scanner.Err())
	}
	var req hook.Request
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %v", err)
	}
	return &req, nil
}

// writeResponse marshals and writes a JSON response to the writer
func writeResponse(writer *bufio.Writer, resp *hook.Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %v", err)
	}
	if _, err := fmt.Fprintf(writer, "%s\n", string(data)); err != nil {
		return fmt.Errorf("failed to write response: %v", err)
	}
	return writer.Flush()
}

// processRequest handles the business logic of processing a request and returning a response
func (i *Interceptor) processRequest(req *hook.Request) (*hook.Response, error) {
	hookRequest := &hook.Request{
		Command:  req.Command,
		PID:      req.PID,
		Hook:     hook.HookType(req.Hook),
		ExitCode: req.ExitCode,
		Duration: req.Duration,
		Metadata: req.Metadata,
	}

	timeout := i.evaluateTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var response *hook.Response
	var err error

	// Check if hook implements IPCHook
	switch h := i.hook.(type) {
	case hook.IPCHook:
		response, err = h.EvaluateIPC(ctx, hookRequest)
		if err != nil {
			response = &hook.Response{
				Exit: true,
			}
		}
	default:
		// For hooks that do not implement IPCHook, default to allow.
		// This ensures LocalHook-only setups are not blocked by IPC stage.
		if i.verbose {
			log.Printf("Non-IPCHook provided; default-allowing request: %v", req.Command)
		}
		response = &hook.Response{Exit: false}
	}

	resp := &hook.Response{
		Exit: response.Exit,
	}

	// Signal exit if requested
	if response.Exit {
		select {
		case <-i.exitSignal:
			// Already closed
		default:
			close(i.exitSignal)
		}
	}

	if i.verbose {
		if response.Exit {
			log.Printf("Request EXIT: %v", req.Command)
		} else {
			log.Printf("Request CONTINUING: %v", req.Command)
		}
	}

	return resp, nil
}
