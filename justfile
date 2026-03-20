default:
    @just --list

# Run validation checks
check:
    bash scripts/check.sh

# Build (validates structure)
build: check

# Development info
dev:
    @echo "Edit hermitclaw script, run 'just check' to validate."
