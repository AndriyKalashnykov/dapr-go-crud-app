#!/usr/bin/env bash
# Lint every ```mermaid fenced block in *.md by parsing it with the official
# Mermaid CLI (minlag/mermaid-cli — same engine GitHub renders with).
# Exits non-zero on any parse error.
set -euo pipefail

cd "$(dirname "$0")/.."

# MERMAID_CLI_VERSION is set by the Makefile recipe; default for direct runs.
MMDC_TAG="${MERMAID_CLI_VERSION:-11.4.2}"

# shellcheck disable=SC2207
files=($(grep -lE '^```mermaid' ./*.md 2>/dev/null || true))
if [ "${#files[@]}" -eq 0 ]; then
  echo "No Mermaid blocks found"
  exit 0
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

extracted=0
for f in "${files[@]}"; do
  base=$(basename "$f" .md)
  awk -v base="$base" -v dir="$tmpdir" '
    /^```mermaid$/ { flag = 1; n++; out = dir "/" base "-" n ".mmd"; next }
    /^```/         { flag = 0; next }
    flag           { print > out }
  ' "$f"
done

shopt -s nullglob
for mmd in "$tmpdir"/*.mmd; do
  echo "Linting $(basename "$mmd")"
  docker run --rm -u "$(id -u):$(id -g)" -v "$tmpdir":/data \
    "minlag/mermaid-cli:${MMDC_TAG}" \
    -i "/data/$(basename "$mmd")" -o "/data/$(basename "$mmd").svg" >/dev/null
  extracted=$((extracted + 1))
done

echo "Mermaid lint passed (${extracted} blocks)"
