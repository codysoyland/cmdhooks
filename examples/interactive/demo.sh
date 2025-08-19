#!/usr/bin/env bash

# Interactive Approval Demo - Demonstrates interactive approval for common Unix commands
# This example prompts for approval before executing monitored commands

echo "=== CmdHooks Interactive Approval Demo ==="
echo "This demo monitors common Unix commands and requires approval"
echo "You'll be prompted to approve/deny the git command"
echo ""

echo "Running: git status"
echo "You will be prompted to approve this git command:"
echo ""

go run . --command "git show | grep Author" --verbose

echo ""
echo "Demo completed!"
