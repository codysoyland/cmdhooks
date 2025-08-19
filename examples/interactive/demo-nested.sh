#!/usr/bin/env bash

# Bash + LS Demo - Demonstrates interactive approval for nested command execution
# This example shows how cmdhooks intercepts both bash and ls commands

echo "=== CmdHooks Bash + LS Demo ==="
echo "This demo monitors bash and ls commands and requires approval"
echo "You'll be prompted to approve the bash script execution"
echo "Then you'll be prompted to approve the ls command within the script"
echo ""

echo "Running: bash test-script.sh"
echo "You will be prompted to approve:"
echo "1. The bash command (to run the script)"
echo "2. The ls command (inside the script)"
echo ""

go run . --command "./demo-artifacts/test-script.sh" --verbose

echo ""
echo "Demo completed!"
