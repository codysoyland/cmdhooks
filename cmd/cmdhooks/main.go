package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/codysoyland/cmdhooks/pkg/wrapper"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		runCommand()
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "cmdhooks - Command hook system for intercepting and controlling command execution\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  cmdhooks run [-v] <command> [args...]\n")
	fmt.Fprintf(os.Stderr, "  cmdhooks help\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  run     Execute a command with hook evaluation (used internally by wrapper scripts)\n")
	fmt.Fprintf(os.Stderr, "  help    Show this help message\n\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	fmt.Fprintf(os.Stderr, "  -v      Enable verbose output\n")
}

func runCommand() {
	runFlags := flag.NewFlagSet("run", flag.ExitOnError)
	verbose := runFlags.Bool("v", false, "Enable verbose output")

	runFlags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: cmdhooks run [-v] <command> [args...]\n")
		fmt.Fprintf(os.Stderr, "\nExecute a command with hook evaluation (used internally by wrapper scripts)\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		runFlags.PrintDefaults()
	}

	if err := runFlags.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}

	args := runFlags.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no command specified\n\n")
		runFlags.Usage()
		os.Exit(1)
	}

	var wrapperOpts []wrapper.WrapperOption
	if *verbose {
		wrapperOpts = append(wrapperOpts, wrapper.WithVerbose(true))
	}

	// The wrapper.Run function will automatically detect the socket path
	// from the CMDHOOKS_SOCKET environment variable
	if err := wrapper.Run(args, wrapperOpts...); err != nil {
		log.Fatal(err)
	}
}
