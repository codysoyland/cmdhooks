package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/codysoyland/cmdhooks/pkg/cmdhooks"
	"github.com/codysoyland/cmdhooks/pkg/wrapper"
)

var (
	command = flag.String("command", "", "Command to execute (will be wrapped with bash -c)")
	verbose = flag.Bool("verbose", false, "Enable verbose output")
)

func main() {
	progName := filepath.Base(os.Args[0])

	// Check for "run" subcommand (wrapper mode)
	isRunMode := len(os.Args) > 1 && os.Args[1] == "run"

	if isRunMode {
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: %s run <command> [args...]\\n", progName)
			os.Exit(1)
		}
		// In wrapper mode, use the wrapper.Run function directly
		// This will use the existing socket from CMDHOOKS_SOCKET environment variable
		var wrapperOpts []wrapper.WrapperOption
		if *verbose {
			wrapperOpts = append(wrapperOpts, wrapper.WithVerbose(true))
		}
		if err := wrapper.Run(os.Args[2:], wrapperOpts...); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Parse flags (works for both modes)
	flag.Parse()

	// Create interactive hook
	interactiveHook := createInteractiveHook()

	// Set up CmdHooks options
	var opts []cmdhooks.Option
	opts = append(opts, cmdhooks.WithHook(interactiveHook))

	// Override to use this binary as the wrapper (for self-contained example)
	// Normally, applications would rely on the installed 'cmdhooks' binary
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	opts = append(opts, cmdhooks.WithWrapperPath([]string{exePath, "run"}))

	if *verbose {
		opts = append(opts, cmdhooks.WithVerbose(true))
	}

	// Create single CmdHooks instance
	ch, err := cmdhooks.New(opts...)
	if err != nil {
		log.Fatal(err)
	}
	defer ch.Close()

	// Command execution mode
	if *command == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s --command <command> [--verbose]\\n", progName)
		fmt.Fprintf(os.Stderr, "   or: %s run <command> [args...]\\n", progName)
		fmt.Fprintf(os.Stderr, "\\nThis example uses interactive approval for common Unix commands.\\n")
		fmt.Fprintf(os.Stderr, "You'll be prompted to approve/deny each monitored command execution.\\n")
		fmt.Fprintf(os.Stderr, "Example: %s --command 'curl https://example.com && chmod +x script.sh'\\n", progName)
		os.Exit(1)
	}

	if *verbose {
		fmt.Printf("Monitoring commands: %v\\n", interactiveHook.Commands())
		fmt.Printf("Executing command: %s\\n", *command)
		fmt.Println("You'll be prompted to approve/deny each monitored command.")
	}

	// Use Execute() method for command execution mode
	if err := ch.Execute([]string{"bash", "-c", *command}); err != nil {
		log.Fatal(err)
	}
}

// createInteractiveHook creates an interactive hook configured for common Unix commands
func createInteractiveHook() *InteractiveHook {
    // Pre-configure with most common Unix commands that might need approval
    commonCommands := []string{
        "curl", "wget", "ssh", "git", "ls",
    }

    return NewInteractiveHook(commonCommands)
}
