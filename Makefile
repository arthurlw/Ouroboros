.PHONY: help build test lint clean install run image docker-build vet fmt coverage

# Default target
help:
	@echo "Ouroboros - Makefile Commands"
	@echo ""
	@echo "Development:"
	@echo "  make build        - Build the ouroboros binary"
	@echo "  make test         - Run all tests"
	@echo "  make test-race    - Run tests with race detector"
	@echo "  make coverage     - Generate test coverage report"
	@echo "  make lint         - Run linters (go vet + staticcheck)"
	@echo "  make fmt          - Format code with gofmt"
	@echo "  make vet          - Run go vet"
	@echo ""
	@echo "Docker:"
	@echo "  make image        - Build executor Docker image"
	@echo "  make docker-build - Build Docker image (alias for image)"
	@echo ""
	@echo "Execution:"
	@echo "  make run GOAL=\"...\"  - Run ouroboros with a goal"
	@echo "  make install      - Install ouroboros binary to GOPATH/bin"
	@echo ""
	@echo "Cleanup:"
	@echo "  make clean        - Remove build artifacts and staging files"
	@echo ""
	@echo "Release:"
	@echo "  make release      - Build release binaries for multiple platforms"

# Build the main binary
build:
	@echo "🔨 Building ouroboros..."
	@go build -o bin/ouroboros ./cmd/ouroboros
	@echo "✅ Build complete: bin/ouroboros"

# Run all tests
test:
	@echo "🧪 Running tests..."
	@go test ./... -v

# Run tests with race detector
test-race:
	@echo "🏁 Running tests with race detector..."
	@go test ./... -race -v

# Generate coverage report
coverage:
	@echo "📊 Generating coverage report..."
	@go test ./... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: coverage.html"

# Run go vet
vet:
	@echo "🔍 Running go vet..."
	@go vet ./...

# Format code
fmt:
	@echo "🎨 Formatting code..."
	@gofmt -s -w .
	@echo "✅ Code formatted"

# Run linters
lint: vet
	@echo "🔍 Running staticcheck..."
	@command -v staticcheck >/dev/null 2>&1 || { echo "Installing staticcheck..."; go install honnef.co/go/tools/cmd/staticcheck@latest; }
	@staticcheck ./...
	@echo "✅ Lint complete"

# Build Docker executor image
image:
	@echo "🐳 Building executor Docker image..."
	@docker build -f Dockerfile.executor -t ouroboros-executor:latest .
	@echo "✅ Docker image built: ouroboros-executor:latest"

docker-build: image

# Install binary to GOPATH/bin
install: build
	@echo "📦 Installing ouroboros..."
	@go install ./cmd/ouroboros
	@echo "✅ Installed to $(shell go env GOPATH)/bin/ouroboros"

# Run ouroboros with a goal
run: build
	@if [ -z "$(GOAL)" ]; then \
		echo "❌ Error: GOAL is required. Usage: make run GOAL=\"your goal here\""; \
		exit 1; \
	fi
	@echo "🐍 Running Ouroboros..."
	@./bin/ouroboros -goal "$(GOAL)"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning..."
	@rm -rf bin/
	@rm -rf skills_library/staging/*
	@rm -f coverage.out coverage.html
	@echo "✅ Clean complete"

# Build release binaries for multiple platforms
release:
	@echo "📦 Building release binaries..."
	@mkdir -p bin/release
	@echo "  - Linux amd64..."
	@GOOS=linux GOARCH=amd64 go build -o bin/release/ouroboros-linux-amd64 ./cmd/ouroboros
	@echo "  - Linux arm64..."
	@GOOS=linux GOARCH=arm64 go build -o bin/release/ouroboros-linux-arm64 ./cmd/ouroboros
	@echo "  - macOS amd64..."
	@GOOS=darwin GOARCH=amd64 go build -o bin/release/ouroboros-darwin-amd64 ./cmd/ouroboros
	@echo "  - macOS arm64 (Apple Silicon)..."
	@GOOS=darwin GOARCH=arm64 go build -o bin/release/ouroboros-darwin-arm64 ./cmd/ouroboros
	@echo "  - Windows amd64..."
	@GOOS=windows GOARCH=amd64 go build -o bin/release/ouroboros-windows-amd64.exe ./cmd/ouroboros
	@echo "✅ Release binaries built in bin/release/"

# Verify everything is ready for release
release-check: lint test-race
	@echo "✅ All checks passed - ready for release!"
