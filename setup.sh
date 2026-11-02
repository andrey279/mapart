#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"

go version
go mod download
go mod verify
mkdir -p build input output
go build -o build/mapart .
go vet ./...
go test ./...

if git rev-parse --git-dir >/dev/null 2>&1; then
    git config core.hooksPath .githooks
    chmod +x .githooks/pre-commit
fi
printf '\nSetup complete. See BUILD.md for GUI builds and make help for commands.\n'
