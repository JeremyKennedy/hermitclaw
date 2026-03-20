#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

errors=0

echo "=== go vet ==="
if CGO_ENABLED=0 go vet ./... 2>&1; then
  echo "  OK"
else
  echo "  FAILED"
  errors=$((errors + 1))
fi

echo ""
echo "=== go test ==="
if CGO_ENABLED=0 go test ./... 2>&1; then
  echo "  OK"
else
  echo "  FAILED"
  errors=$((errors + 1))
fi

echo ""
echo "=== go build ==="
if CGO_ENABLED=0 go build -o /dev/null . 2>&1; then
  echo "  OK"
else
  echo "  FAILED"
  errors=$((errors + 1))
fi

echo ""
echo "=== shellcheck ==="
if command -v shellcheck &>/dev/null; then
  if shellcheck -s bash install.sh; then
    echo "  install.sh: OK"
  else
    echo "  install.sh: FAILED"
    errors=$((errors + 1))
  fi
else
  echo "  shellcheck not found, skipping"
fi

echo ""
echo "=== structure ==="
for required in config.example.toml skills/welcome/SKILL.md LICENSE README.md CLAUDE.md; do
  if [ -f "$required" ]; then
    echo "  $required: OK"
  else
    echo "  $required: MISSING"
    errors=$((errors + 1))
  fi
done

echo ""
if [ "$errors" -gt 0 ]; then
  echo "FAILED: $errors error(s)"
  exit 1
else
  echo "ALL CHECKS PASSED"
fi
