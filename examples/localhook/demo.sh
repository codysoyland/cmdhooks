#!/usr/bin/env bash
set -euo pipefail

echo "=== LocalHook-only Demo ==="
echo "This runs only a LocalHook in the wrapper process."
echo "The interceptor has no IPCHook and will default-allow at IPC."
echo "Run with --verbose to see the default-allow message from the interceptor."
echo

pushd "$(dirname "$0")" >/dev/null

# Allow case: use external command (ls) so the wrapper intercepts
echo "-- Allow case (ls .) --"
go run . --verbose --command "ls ."
echo

# Deny case using LocalHook rule (argument equals DENY)
echo "-- Deny case (ls DENY) --"
set +e
go run . --verbose --command "ls DENY"
code=$?
set -e
echo "Exit code: $code (non-zero indicates LocalHook blocked execution)"

popd >/dev/null
