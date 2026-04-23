# Contributing to Ouroboros

Thank you for your interest in contributing to Ouroboros! This document provides guidelines and instructions for contributing.

## Development Setup

### Prerequisites

- **Go 1.24+** (tested on 1.24.x and 1.25.x)
- **Docker** (for sandbox execution)
- **Make** (optional, but recommended)

### Initial Setup

1. Clone the repository:
   ```bash
   git clone https://github.com/arthurlw/ouroboros.git
   cd ouroboros
   ```

2. Build the Docker executor image:
   ```bash
   make image
   # OR: docker build -f Dockerfile.executor -t ouroboros-executor:latest .
   ```

3. Build the binary:
   ```bash
   make build
   # OR: go build -o bin/ouroboros ./cmd/ouroboros
   ```

4. Set up your LLM provider (choose one):
   ```bash
   # Anthropic (recommended)
   export ANTHROPIC_API_KEY="your-key"

   # OR Groq (fast, free tier)
   export GROQ_API_KEY="your-key"

   # OR OpenAI
   export OPENAI_API_KEY="your-key"

   # OR Ollama (local, no API key needed)
   # Just have Ollama running at localhost:11434
   ```

5. Test your setup:
   ```bash
   make run GOAL="calculate fibonacci of 10"
   ```

## Development Workflow

### Running Tests

```bash
# Run all tests
make test

# Run with race detector
make test-race

# Generate coverage report
make coverage
```

### Code Quality

Before submitting a PR, ensure your code passes all checks:

```bash
# Run linters
make lint

# Format code
make fmt

# Run all checks
make release-check
```

### Commit Message Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` - New features
- `fix:` - Bug fixes
- `refactor:` - Code refactoring
- `test:` - Adding or updating tests
- `docs:` - Documentation changes
- `chore:` - Maintenance tasks

Examples:
```
feat: add Anthropic Claude provider support
fix: correct timeout detection in sandbox
refactor: extract LLM logic into internal/llm package
test: add AST-based sanitizer test cases
docs: update README with installation instructions
```

## How to Add a New LLM Provider

Adding support for a new LLM provider is straightforward:

1. Create a new file in `internal/llm/` (e.g., `yourprovider.go`)

2. Implement the `Provider` interface:
   ```go
   package llm

   type YourProvider struct {
       APIKey      string
       DefaultModel string
       BaseURL     string
       HTTPClient  *http.Client
   }

   func NewYourProvider(apiKey string) *YourProvider {
       return &YourProvider{
           APIKey:      apiKey,
           DefaultModel: "your-model-name",
           BaseURL:     "https://api.yourprovider.com/v1/...",
           HTTPClient:  &http.Client{Timeout: 60 * time.Second},
       }
   }

   func (y *YourProvider) Name() string {
       return "yourprovider"
   }

   func (y *YourProvider) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (Response, error) {
       // Implement API call
       // Return Response with Content, PromptTokens, OutputTokens, etc.
   }
   ```

3. Add to `factory.go`:
   ```go
   if key := os.Getenv("YOURPROVIDER_API_KEY"); key != "" {
       slog.Info("🧠 Using Provider: YOURPROVIDER")
       baseProvider = NewYourProvider(key)
   }
   ```

4. Add tests and documentation

5. Submit a PR!

## Testing Guidelines

### Unit Tests

- Every new feature must include unit tests
- Aim for >70% coverage on `internal/agent` and `internal/sandbox`
- Use table-driven tests where appropriate
- Test both success and failure paths

Example:
```go
func TestMyFeature(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid case", "input", "output", false},
        {"error case", "bad", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFeature(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("MyFeature() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("MyFeature() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Integration Tests

For Docker-dependent tests, use build tags:
```go
//go:build integration

package sandbox_test

func TestDockerIntegration(t *testing.T) {
    // Test requiring Docker
}
```

Run with: `go test -tags=integration ./...`

## Architecture Overview

```
ouroboros/
├── cmd/ouroboros/        # Main entry point
├── internal/
│   ├── agent/            # Core agent logic (planner, critic, generator)
│   ├── llm/              # LLM provider implementations
│   ├── sandbox/          # Docker sandbox execution
│   └── skills/           # Tool registry (MCP-compatible)
├── skills_library/
│   ├── verified/         # Verified, working tools
│   └── staging/          # Temporary build area
└── Dockerfile.executor   # Sandbox execution environment
```

### Key Components

- **Planner**: Breaks down goals into atomic steps
- **Generator**: Produces Go code + tests
- **Critic**: Verifies compilation and test results
- **Sandbox**: Executes code in isolated Docker containers
- **Registry**: Manages persistent tool library

## Pull Request Process

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make your changes
4. Add tests
5. Run `make release-check` to ensure everything passes
6. Commit using conventional commits
7. Push and create a Pull Request
8. Wait for CI to pass and respond to review feedback

### PR Title Format

Use the same convention as commits:
```
feat: add support for Anthropic Claude
fix: resolve timeout detection bug
docs: improve installation instructions
```

### What to Include

- **Description**: What does this PR do?
- **Motivation**: Why is this change needed?
- **Testing**: How was this tested?
- **Screenshots/Logs**: If applicable

## Code Review Guidelines

As a reviewer:
- Be respectful and constructive
- Focus on code quality, not personal preferences
- Ask questions to understand intent
- Approve when ready, request changes if needed

As a contributor:
- Respond to feedback promptly
- Don't take criticism personally
- Ask for clarification if needed

## Getting Help

- **Issues**: [GitHub Issues](https://github.com/arthurlw/ouroboros/issues)
- **Discussions**: [GitHub Discussions](https://github.com/arthurlw/ouroboros/discussions)
- **Email**: (maintainer contact if applicable)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
