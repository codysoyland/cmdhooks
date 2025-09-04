package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/codysoyland/cmdhooks/pkg/cmdhooks"
	"github.com/codysoyland/cmdhooks/pkg/hook"
	"github.com/codysoyland/cmdhooks/pkg/wrapper"
)

// LocalOnlyHook implements only hook.LocalHook and runs entirely in the wrapper process.
// It demonstrates fast, in-process evaluation without any IPCHook implementation.
type LocalOnlyHook struct {
	name     string
	commands []string
}

func (h *LocalOnlyHook) Name() string { return h.name }
func (h *LocalOnlyHook) Commands() []string { return h.commands }

// EvaluateLocal performs a simple policy:
// - If any arg equals "DENY", request exit (block)
// - Otherwise, allow and attach metadata proving local evaluation ran
func (h *LocalOnlyHook) EvaluateLocal(ctx context.Context, req *hook.Request) (*hook.Response, error) {
	if len(req.Command) == 0 {
		return &hook.Response{Exit: false}, nil
	}
	for _, a := range req.Command[1:] {
		if a == "DENY" {
			return &hook.Response{Exit: true}, nil
		}
	}
	return &hook.Response{Metadata: map[string]any{"local_only": true}}, nil
}

var (
	command = flag.String("command", "", "Command to execute (wrapped via bash -c)")
	verbose = flag.Bool("verbose", false, "Enable verbose output")
)

func main() {
	prog := filepath.Base(os.Args[0])

	// Detect wrapper run mode: our wrappers call this binary with subcommand "run".
    if len(os.Args) > 1 && os.Args[1] == "run" {
        // LocalHook-only inside wrapper process (do not wrap bash here)
        localHook := &LocalOnlyHook{name: "local-only", commands: []string{"echo", "ls", "curl", "wget"}}

		// Build wrapper options: autodetect socket + verbose from env, plus CLI flag
		var wopts []wrapper.WrapperOption
		if sp := strings.TrimSpace(os.Getenv("CMDHOOKS_SOCKET")); sp != "" {
			wopts = append(wopts, wrapper.WithSocketPath(sp))
		}
		if v := strings.TrimSpace(os.Getenv("CMDHOOKS_VERBOSE")); v != "" && strings.ToLower(v) != "false" && v != "0" {
			wopts = append(wopts, wrapper.WithVerbose(true))
		}
		if *verbose {
			wopts = append(wopts, wrapper.WithVerbose(true))
		}

		w := wrapper.NewWrapperCommand(localHook, wopts...)
		if err := w.Run(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}

	flag.Parse()
	if *command == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s --command '<cmd>' [--verbose]\n", prog)
		fmt.Fprintf(os.Stderr, "   or: %s run <command> [args...]  # internal wrapper mode\n", prog)
		fmt.Fprintf(os.Stderr, "\nThis example demonstrates a LocalHook-only setup. The interceptor runs without an IPCHook\n")
		fmt.Fprintf(os.Stderr, "and will default-allow at the IPC stage. Run with --verbose to see IPC default-allow logs.\n")
		os.Exit(1)
	}

    // LocalHook-only: no IPCHook. Do not include bash in monitored commands for this demo.
    localHook := &LocalOnlyHook{name: "local-only", commands: []string{"echo", "ls", "curl", "wget"}}

	// Use this example binary as the wrapper target so the LocalHook runs in the wrapper process.
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}

	opts := []cmdhooks.Option{
		cmdhooks.WithHook(localHook),
		cmdhooks.WithWrapperPath([]string{exe, "run"}),
	}
	if *verbose {
		opts = append(opts, cmdhooks.WithVerbose(true))
	}

	ch, err := cmdhooks.New(opts...)
	if err != nil {
		log.Fatal(err)
	}
	defer ch.Close()

	// Execute via bash -c for convenience
	if err := ch.Execute([]string{"bash", "-c", *command}); err != nil {
		log.Fatal(err)
	}
}
