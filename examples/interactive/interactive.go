package main

import (
	"bufio"
	"context"
	"log"
	"os"
	"strings"

	"github.com/codysoyland/cmdhooks/pkg/hook"
)

// InteractiveHook prompts the user for approval on each request
type InteractiveHook struct {
	name     string
	commands []string
}

// NewInteractiveHook creates a new interactive hook
func NewInteractiveHook(commands []string) *InteractiveHook {
	return &InteractiveHook{
		name:     "interactive",
		commands: commands,
	}
}

// Name returns the hook name
func (p *InteractiveHook) Name() string {
	return p.name
}

// Commands returns the list of commands this hook handles
func (p *InteractiveHook) Commands() []string {
	return p.commands
}

// EvaluateIPC evaluates a request for a specific hook
func (p *InteractiveHook) EvaluateIPC(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	switch req.Hook {
	case hook.HookPreRun:
		return p.evaluatePreRun(ctx, req)
	case hook.HookPostRun:
		return p.evaluatePostRun(ctx, req)
	default:
		return &hook.Response{
			Exit: true,
		}, nil
	}
}

// evaluatePreRun prompts the user for pre-run approval
func (p *InteractiveHook) evaluatePreRun(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	if req == nil {
		return &hook.Response{
			Exit: true,
		}, nil
	}

	// Check context for cancellation
	select {
	case <-ctx.Done():
		return &hook.Response{
			Exit: true,
		}, ctx.Err()
	default:
	}

	// Prompt user for pre-run approval
	return p.promptUserPreRun(req), nil
}

// evaluatePostRun shows the response to the user and asks for final approval
func (p *InteractiveHook) evaluatePostRun(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	if req == nil {
		return &hook.Response{
			Exit: true,
		}, nil
	}

	// Check context for cancellation
	select {
	case <-ctx.Done():
		return &hook.Response{
			Exit: true,
		}, ctx.Err()
	default:
	}

	// Show response and get user response
	return p.promptUserPostRun(req), nil
}

// promptUserPreRun displays the pre-run approval prompt
func (p *InteractiveHook) promptUserPreRun(req *hook.Request) *hook.Response {
	log.Printf("\n[PRE-RUN APPROVAL REQUIRED]\n")
	cmd := "unknown"
	args := []string{}
	if len(req.Command) > 0 {
		cmd = req.Command[0]
		args = req.Command[1:]
	}
	log.Printf("Command: %s", cmd)
	log.Printf("Args: %v", args)
	log.Printf("Continue? [Y/n]: ")

	return p.getUserInput()
}

// promptUserPostRun displays the post-run approval prompt with response data
func (p *InteractiveHook) promptUserPostRun(req *hook.Request) *hook.Response {
	log.Printf("\n[POST-RUN APPROVAL REQUIRED]\n")
	cmd := "unknown"
	args := []string{}
	if len(req.Command) > 0 {
		cmd = req.Command[0]
		args = req.Command[1:]
	}
	log.Printf("Command: %s", cmd)
	log.Printf("Args: %v", args)
	log.Printf("Exit Code: %d", req.ExitCode)
	log.Printf("Duration: %v", req.Duration)

	log.Printf("Continue? [Y/n]: ")

	return p.getUserInput()
}

// getUserInput handles the actual user input reading
func (p *InteractiveHook) getUserInput() *hook.Response {
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return &hook.Response{
			Exit: true,
		}
	}

	response = strings.TrimSpace(strings.ToLower(response))

	switch response {
	case "n", "no":
		return &hook.Response{
			Exit: true,
		}
	default:
		// Default to yes for any other input
		return &hook.Response{}
	}
}
