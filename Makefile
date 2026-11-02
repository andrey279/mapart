# Makefile for mapartscript

.PHONY: all build build-cli build-gui build-windows clean test test-short test-coverage coverage benchmark vet check-format pre-commit help install-deps lint fmt run-cli run-gui export-github

# Default target
all: build

# Build both CLI and GUI
build: build-cli build-gui

# Build CLI version
build-cli:
	@echo "Building CLI version..."
	go build -o mapart .

# Build GUI version (cross-platform with Fyne)
build-gui:
	@echo "Building GUI version..."
	go build -tags gui -o mapart-gui .

# Build Windows GUI (Win32)
build-windows:
	@echo "Building Windows GUI..."
	GOOS=windows GOARCH=amd64 go build -o mapart-gui.exe -ldflags="-H windowsgui" .

# Install Fyne dependencies (Linux)
install-deps:
	@echo "Installing Fyne dependencies..."
	@if [ "$$(uname)" = "Linux" ]; then \
		if command -v apt-get >/dev/null; then \
			sudo apt-get update && sudo apt-get install -y libgl1-mesa-dev xorg-dev; \
		elif command -v dnf >/dev/null; then \
			sudo dnf install -y libX11-devel libXcursor-devel libXrandr-devel libXinerama-devel mesa-libGL-devel libXi-devel libXxf86vm-devel; \
		elif command -v pacman >/dev/null; then \
			sudo pacman -S xorg-server-devel mesa; \
		fi \
	fi

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

test-short:
	go test -short ./...

vet:
	go vet ./...

check-format:
	@test -z "$$(gofmt -l ./*.go)" || { gofmt -l ./*.go; exit 1; }

pre-commit: check-format vet test-short

coverage: test-coverage

benchmark:
	go test -run '^$$' -bench . -benchmem ./...

export-github:
	python3 scripts/prepare_github.py

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f mapart mapart.exe mapart-gui mapart-gui.exe
	rm -f coverage.out coverage.html
	rm -rf temp/ tmp/ dist/ build/

# Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint v2 not found; running go vet"; go vet ./...; \
	fi

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run CLI version
run-cli:
	@echo "Running CLI version..."
	./mapart -h

# Run GUI version
run-gui:
	@echo "Running GUI version..."
	./mapart-gui

# Show help
help:
	@echo "Mapartscript Build System"
	@echo ""
	@echo "Targets:"
	@echo "  make build          - Build both CLI and GUI versions"
	@echo "  make build-cli      - Build CLI version only"
	@echo "  make build-gui      - Build GUI version only (Linux/macOS)"
	@echo "  make build-windows  - Build Windows GUI version"
	@echo "  make install-deps   - Install system dependencies (Linux)"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make lint           - Run linter"
	@echo "  make fmt            - Format code"
	@echo "  make run-cli        - Run CLI version"
	@echo "  make run-gui        - Run GUI version"
	@echo "  make help           - Show this help message"

	@echo "  make pre-commit     - Check formatting, vet and short tests"
	@echo "  make export-github  - Export clean sources and ZIP to dist/github/"
