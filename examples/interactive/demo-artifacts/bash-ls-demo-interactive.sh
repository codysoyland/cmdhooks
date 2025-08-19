#!/usr/bin/env bash

# Interactive Bash + LS Demo - Run this manually for interactive testing
# This example shows how cmdhooks intercepts both bash and ls commands

echo "=== CmdHooks Interactive bash + ls Demo ==="
echo "This demo monitors bash and ls commands and requires approval"
echo "You'll be prompted to approve commands during execution"
echo ""

go run . --command "bash test-script.sh" --verbose
