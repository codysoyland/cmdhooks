package wrapper

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "maps"
    "net"
    "os"
    "os/exec"
    "strings"
    "sync"
    "time"

	"github.com/codysoyland/cmdhooks/pkg/hook"
)

const (
    // MaxIPCMessageBytes caps IPC messages to prevent unbounded memory usage.
    // Responses exceeding this size are treated as an error.
    MaxIPCMessageBytes = 64 * 1024 // 64 KiB
)

// pathMutex protects PATH environment variable manipulation to prevent race conditions
var pathMutex sync.Mutex

// validateCommand checks if a command slice is valid (non-empty)
func validateCommand(cmd []string) error {
	if len(cmd) == 0 {
		return fmt.Errorf("command cannot be empty")
	}
	return nil
}

// WrapperCommand implements the "run" subcommand that acts as a wrapper
type WrapperCommand struct {
	Hook       hook.Hook // Single hook for command evaluation
	SocketPath string
	Verbose    bool
}

// WrapperOption is a functional option for configuring WrapperCommand
type WrapperOption func(*WrapperCommand)

// WithSocketPath sets the IPC socket path for fallback evaluation
func WithSocketPath(path string) WrapperOption {
	return func(w *WrapperCommand) {
		w.SocketPath = path
	}
}

// WithVerbose enables/disables verbose output
func WithVerbose(verbose bool) WrapperOption {
	return func(w *WrapperCommand) {
		w.Verbose = verbose
	}
}

// Run executes a command with pre- and post-run hook evaluation
func Run(cmd []string, opts ...WrapperOption) error {
	// Auto-detect socket path from environment
	if socketPath := os.Getenv("CMDHOOKS_SOCKET"); socketPath != "" {
		opts = append(opts, WithSocketPath(socketPath))
	}

	// Auto-detect verbose mode from environment
	if v := strings.TrimSpace(os.Getenv("CMDHOOKS_VERBOSE")); v != "" && strings.ToLower(v) != "false" && v != "0" {
		opts = append(opts, WithVerbose(true))
	}

	// Use nil hook - wrapper will rely on socket-based IPC
	w := NewWrapperCommand(nil, opts...)
	return w.Run(cmd)
}

// NewWrapperCommand creates a WrapperCommand with functional options
func NewWrapperCommand(hook hook.Hook, opts ...WrapperOption) *WrapperCommand {
	w := &WrapperCommand{
		Hook: hook,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Run executes a command with pre- and post-run hook evaluation
func (w *WrapperCommand) Run(command []string) error {
	if err := validateCommand(command); err != nil {
		return err
	}

	cmd := command[0]
	args := command[1:]

	if w.Verbose {
		log.Printf("Wrapper: %s %v", cmd, args)
	}

	// Create basic metadata
	metadata := make(map[string]any)

	if w.Verbose {
		log.Printf("Evaluating hooks for %s...", cmd)
	}

	// Pre-run hook evaluation
	if err := w.executePreRun(command, metadata); err != nil {
		return err
	}

	// Execute the actual command
	startTime := time.Now()
	exitCode, stdoutFile, stderrFile, err := w.executeCommand(cmd, args)
	duration := time.Since(startTime)

	// Ensure files are cleaned up
	if stdoutFile != "" {
		defer os.Remove(stdoutFile)
	}
	if stderrFile != "" {
		defer os.Remove(stderrFile)
	}

	// Post-run hook evaluation
	if postErr := w.executePostRun(command, metadata, exitCode, duration, stdoutFile, stderrFile); postErr != nil {
		return postErr
	}

	// Output results and handle exit
	w.outputResults(stdoutFile, stderrFile, exitCode)

	return err
}

// hookHandlesCommand checks if a hook handles the given command
func (w *WrapperCommand) hookHandlesCommand(hookCommands []string, requestCommand string) bool {
	for _, cmd := range hookCommands {
		if cmd == requestCommand || cmd == "*" {
			return true
		}
	}
	return false
}

// evaluateLocalHook evaluates the local hook if present and handles the command
func (w *WrapperCommand) evaluateLocalHook(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	localHook, ok := w.Hook.(hook.LocalHook)
	if !ok {
		return nil, nil
	}

	// Check if this hook handles this command
	if len(req.Command) == 0 || !w.hookHandlesCommand(localHook.Commands(), req.Command[0]) {
		return nil, nil
	}

	response, err := localHook.EvaluateLocal(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("local hook %s error: %w", localHook.Name(), err)
	}

	if w.Verbose {
		log.Printf("Local hook %s evaluated", localHook.Name())
	}

	return response, nil
}

// evaluateIPCHook evaluates hooks via IPC, merging metadata from local response
func (w *WrapperCommand) evaluateIPCHook(ctx context.Context, req *hook.Request, localResponse *hook.Response) (*hook.Response, error) {
	if w.SocketPath == "" {
		return nil, nil
	}

	if w.Verbose {
		log.Printf("Using IPC evaluation")
	}

	// Start with the original request metadata and merge local hook metadata
	mergedMetadata := make(map[string]interface{})
	if req.Metadata != nil {
		maps.Copy(mergedMetadata, req.Metadata)
	}

	// Merge metadata from local hook response if available (overrides conflicts)
	if localResponse != nil && localResponse.Metadata != nil {
		maps.Copy(mergedMetadata, localResponse.Metadata)
	}

	ipcReq := hook.Request{
		Command:  req.Command,
		PID:      req.PID,
		Hook:     req.Hook,
		ExitCode: req.ExitCode,
		Duration: req.Duration,
		Metadata: mergedMetadata,
	}

	resp, err := runHook(w.SocketPath, ipcReq)
	if err != nil {
		return nil, fmt.Errorf("IPC hook evaluation failed: %w", err)
	}
	return resp, nil
}

// evaluateHooks evaluates both local and IPC hooks when available
func (w *WrapperCommand) evaluateHooks(req *hook.Request) (*hook.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Evaluate local hook first
	localResponse, err := w.evaluateLocalHook(ctx, req)
	if err != nil {
		return nil, err
	}

	// If local hook requests exit, return immediately
	if localResponse != nil && localResponse.Exit {
		return localResponse, nil
	}

	// Evaluate IPC hook
	ipcResponse, err := w.evaluateIPCHook(ctx, req, localResponse)
	if err != nil {
		return nil, err
	}
	if ipcResponse != nil {
		return ipcResponse, nil
	}

	// If we had a local response but no IPC, return the local response
	if localResponse != nil {
		return localResponse, nil
	}

	// No hook available - continue by default
	return &hook.Response{}, nil
}

// SetSocketPath sets the IPC socket path for IPC evaluation
func (w *WrapperCommand) SetSocketPath(path string) {
	w.SocketPath = path
}

// SetVerbose enables/disables verbose output
func (w *WrapperCommand) SetVerbose(verbose bool) {
	w.Verbose = verbose
}

// executeCommand executes the command and captures output and exit code
// Returns filenames for stdout/stderr instead of file handles to avoid memory usage
func (w *WrapperCommand) executeCommand(cmd string, args []string) (int, string, string, error) {
	// Get clean PATH without wrapper directory
	cleanPath := w.getCleanPath()

	// Find the real command using clean PATH to avoid recursive wrapper calls
	// Use mutex to prevent race conditions with PATH environment variable
	pathMutex.Lock()
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", cleanPath)
	realCmd, err := exec.LookPath(cmd)
	os.Setenv("PATH", origPath)
	pathMutex.Unlock()

	if err != nil {
		return 1, "", "", fmt.Errorf("command not found: %s", cmd)
	}

	// Use the absolute path to the real command to avoid wrapper recursion
	execCmd := exec.Command(realCmd, args...)
	execCmd.Stdin = os.Stdin

	// Set up environment with wrapper PATH so child processes can be intercepted
	// Note: We use the original PATH (with wrapper dir) for child processes
	execCmd.Env = w.getCleanEnvironment(origPath)

	// Create temporary files for stdout and stderr to avoid memory limits
	stdoutFile, err := os.CreateTemp("", "cmdhooks-stdout-*")
	if err != nil {
		return 1, "", "", fmt.Errorf("failed to create stdout temp file: %w", err)
	}
	stdoutFile.Close() // Close immediately, we only need the filename

	stderrFile, err := os.CreateTemp("", "cmdhooks-stderr-*")
	if err != nil {
		os.Remove(stdoutFile.Name())
		return 1, "", "", fmt.Errorf("failed to create stderr temp file: %w", err)
	}
	stderrFile.Close() // Close immediately, we only need the filename

	// Reopen files for writing (exec.Command needs writable files)
	stdoutWrite, err := os.OpenFile(stdoutFile.Name(), os.O_WRONLY, 0600)
	if err != nil {
		os.Remove(stdoutFile.Name())
		os.Remove(stderrFile.Name())
		return 1, "", "", fmt.Errorf("failed to reopen stdout file: %w", err)
	}
	defer stdoutWrite.Close()

	stderrWrite, err := os.OpenFile(stderrFile.Name(), os.O_WRONLY, 0600)
	if err != nil {
		os.Remove(stdoutFile.Name())
		os.Remove(stderrFile.Name())
		return 1, "", "", fmt.Errorf("failed to reopen stderr file: %w", err)
	}
	defer stderrWrite.Close()

	execCmd.Stdout = stdoutWrite
	execCmd.Stderr = stderrWrite

	err = execCmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return exitCode, stdoutFile.Name(), stderrFile.Name(), nil
}

// executePreRun handles pre-run hook evaluation
func (w *WrapperCommand) executePreRun(command []string, metadata map[string]any) error {
	req := &hook.Request{
		Command:  command,
		PID:      os.Getpid(),
		Hook:     hook.HookPreRun,
		Metadata: metadata,
	}

	response, err := w.evaluateHooks(req)
	if err != nil {
		return fmt.Errorf("pre-run hook evaluation error: %w", err)
	}

	if response.Exit {
		if w.Verbose {
			log.Printf("✗ Process termination requested")
		}
		return fmt.Errorf("process termination requested")
	}

	if w.Verbose {
		log.Printf("✓ Pre-run continuing")
	}

	return nil
}

// executePostRun handles post-run hook evaluation
func (w *WrapperCommand) executePostRun(command []string, metadata map[string]any, exitCode int, duration time.Duration, stdoutFile, stderrFile string) error {
	// Pass filenames to hooks instead of reading data into memory
	if stdoutFile != "" {
		metadata["stdout_file"] = stdoutFile
	}
	if stderrFile != "" {
		metadata["stderr_file"] = stderrFile
	}

	metadata["execution_duration"] = duration

	request := &hook.Request{
		Command:  command,
		PID:      os.Getpid(),
		Hook:     hook.HookPostRun,
		Metadata: metadata,
		ExitCode: exitCode,
		Duration: duration,
	}

	response, err := w.evaluateHooks(request)
	if err != nil {
		return fmt.Errorf("post-run hook evaluation error: %w", err)
	}

	if response.Exit {
		if w.Verbose {
			log.Printf("✗ Process termination requested")
		}
		return fmt.Errorf("process termination requested")
	}

	if w.Verbose {
		log.Printf("✓ Post-run continuing")
	}

	return nil
}

// outputResults writes captured stdout/stderr to user and exits with original code
func (w *WrapperCommand) outputResults(stdoutFile, stderrFile string, exitCode int) {
	// Copy captured output to user's stdout/stderr
	if stdoutFile != "" {
		if file, err := os.Open(stdoutFile); err == nil {
			_, _ = io.Copy(os.Stdout, file)
			file.Close()
		}
	}
	if stderrFile != "" {
		if file, err := os.Open(stderrFile); err == nil {
			_, _ = io.Copy(os.Stderr, file)
			file.Close()
		}
	}

	// Exit with original exit code
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

// getCleanPath returns PATH without the cmdhooks wrapper directory
func (w *WrapperCommand) getCleanPath() string {
	currentPath := os.Getenv("PATH")
	pathDirs := strings.Split(currentPath, string(os.PathListSeparator))

	var cleanDirs []string
	for _, dir := range pathDirs {
		// Skip directories that contain our wrappers
		if !strings.Contains(dir, "cmdhooks-wrappers") {
			cleanDirs = append(cleanDirs, dir)
		}
	}

	return strings.Join(cleanDirs, string(os.PathListSeparator))
}

// getCleanEnvironment creates environment with clean PATH
func (w *WrapperCommand) getCleanEnvironment(cleanPath string) []string {
	env := os.Environ()
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + cleanPath
			return env
		}
	}
	// If PATH not found, add it
	return append(env, "PATH="+cleanPath)
}

// runHook sends a request to the IPC socket and returns the hook response
func runHook(socketPath string, req hook.Request) (*hook.Response, error) {
    conn, err := net.Dial("unix", socketPath)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to socket: %w", err)
    }
    defer conn.Close()

	// Send request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	if _, err := fmt.Fprintf(conn, "%s\n", string(data)); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

    // Read response with a bounded scanner
    scanner := bufio.NewScanner(conn)
    scanner.Buffer(make([]byte, 0, 64*1024), MaxIPCMessageBytes)
    if !scanner.Scan() {
        if err := scanner.Err(); err != nil {
            return nil, fmt.Errorf("failed to read response: %w", err)
        }
        return nil, fmt.Errorf("no response from socket")
    }

	var resp hook.Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}
