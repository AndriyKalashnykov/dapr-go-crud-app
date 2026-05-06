#!/usr/bin/env bash
# image-smoke-test.sh — boot each container briefly and verify it doesn't
# crash. Used by `make image-smoke-test` and the CI docker job.
#
# Usage: image-smoke-test.sh <image-ref> [<image-ref> ...]
#   Each <image-ref> is a fully-qualified image name (e.g. ko.local/app:scan).
#   The container name is derived from the leaf path component.
#
# The check is intentionally minimal: each container is started, given 5
# seconds to either boot cleanly OR crash, then forcibly removed. A
# binary that crash-loops within 5s fails the gate; one that boots and
# starts polling Dapr/Redis indefinitely passes (Dapr-client services
# can't be exercised against a real sidecar in a flat `docker run`).
set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "Usage: $0 <image-ref> [<image-ref> ...]" >&2
  exit 2
fi

EXIT=0
for ref in "$@"; do
  # Derive container name from the leaf path: ko.local/app:scan → app
  leaf="${ref##*/}"
  name="smoke-${leaf%%:*}"
  docker rm -f "$name" >/dev/null 2>&1 || true

  echo "==> smoke ${ref}"
  if ! docker run -d --name="$name" "$ref" >/dev/null; then
    echo "FAIL: ${ref} failed to start"
    EXIT=1
    continue
  fi

  # Give it 5s to crash-loop if it's going to.
  sleep 5
  if docker ps --filter "name=$name" --filter "status=running" --format '{{.Names}}' | grep -qx "$name"; then
    echo "PASS: ${ref} still running after 5s"
  else
    # Container stopped — distinguish clean exit (CLI tool / printed usage
    # and exited 0) from panic / segfault / unhandled error (non-zero exit).
    exit_code=$(docker inspect -f '{{.State.ExitCode}}' "$name" 2>/dev/null || echo "?")
    if [ "$exit_code" = "0" ]; then
      echo "PASS: ${ref} exited cleanly (code 0)"
    else
      echo "FAIL: ${ref} exited within 5s (code ${exit_code})"
      docker logs "$name" 2>&1 | tail -30
      EXIT=1
    fi
  fi
  docker rm -f "$name" >/dev/null 2>&1 || true
done

exit "$EXIT"
