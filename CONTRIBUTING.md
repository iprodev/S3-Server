# Contributing to S3 Storage System

First off, thank you for considering contributing to this project! It's people like you that make open source great.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [How Can I Contribute?](#how-can-i-contribute)
- [Development Setup](#development-setup)
- [Coding Standards](#coding-standards)
- [Testing Guidelines](#testing-guidelines)
- [Documentation](#documentation)
- [Pull Request Process](#pull-request-process)
- [Community](#community)

## Code of Conduct

This project and everyone participating in it is governed by our [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check the existing issues to avoid duplicates. When creating a bug report, include as many details as possible:

**Use this template for bug reports:**

```markdown
**Describe the bug**
A clear and concise description of what the bug is.

**To Reproduce**
Steps to reproduce the behavior:
1. Start server with '...'
2. Execute command '...'
3. Send request '...'
4. See error

**Expected behavior**
What you expected to happen.

**Actual behavior**
What actually happened.

**Environment:**
 - OS: [e.g. Ubuntu 22.04]
 - Go Version: [e.g. 1.21.0]
 - Deployment: [e.g. Docker, Kubernetes, Systemd, Local]
 - S3 Server Version: [e.g. v1.0.0]

**Logs**
```
Paste relevant logs here (use -log_level debug for verbose output)
```

**Additional context**
Add any other context about the problem here.
```

### Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. When creating an enhancement suggestion:

**Use this template:**

```markdown
**Is your feature request related to a problem?**
A clear description of what the problem is. Ex. I'm always frustrated when [...]

**Describe the solution you'd like**
A clear and concise description of what you want to happen.

**Describe alternatives you've considered**
A clear description of any alternative solutions or features you've considered.

**Additional context**
Add any other context, screenshots, or examples about the feature request here.

**Would you be willing to implement this feature?**
Yes/No - If yes, we can guide you through the process!
```

### Your First Code Contribution

Unsure where to begin? Look for issues labeled:

- `good first issue` - Simple issues perfect for newcomers
- `help wanted` - Issues where we need community help
- `documentation` - Documentation improvements

### Pull Requests

1. Fork the repo and create your branch from `main`
2. Follow our [Development Setup](#development-setup) guide
3. Make your changes following our [Coding Standards](#coding-standards)
4. Add tests if you've added code that should be tested
5. Ensure all tests pass
6. Update documentation as needed
7. Submit your pull request!

## Development Setup

### Prerequisites

- Go 1.21 or higher
- Git
- Make (optional, but recommended)

### Initial Setup

```bash
# Clone your fork
git clone https://github.com/iProDev/S3-Server.git
cd s3-server

# Add upstream remote
git remote add upstream https://github.com/iProDev/S3-Server.git

# Install dependencies
go mod download

# Build the project
go build -o s3_server ./cmd/s3_server

# Or use the provided script
./build_and_setup.sh
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with race detection
go test -race ./...

# Run specific test file
go test -v ./cmd/s3_server/features_test.go

# Or use the test script
./test_new_features.sh
```

### Running Locally

```bash
# Start storage nodes
./start_nodes.sh

# Start gateway (in another terminal)
./start_gateway.sh

# Test the setup
curl http://localhost:8080/health
```

### Development Workflow

1. **Create a branch:**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes**
   - Write code
   - Add tests
   - Update docs

3. **Test your changes:**
   ```bash
   go test ./...
   go test -race ./...
   ./test_new_features.sh
   ```

4. **Commit your changes:**
   ```bash
   git add .
   git commit -m "Add feature: description of feature"
   ```

5. **Keep your branch updated:**
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

6. **Push to your fork:**
   ```bash
   git push origin feature/your-feature-name
   ```

7. **Create a Pull Request**

## Coding Standards

### Go Code Style

We follow standard Go conventions:

- **Use `gofmt`** - Format all code with `gofmt` before committing
- **Use `go vet`** - Run `go vet` to catch common mistakes
- **Use `golint`** - Run golint for style checks (suggestions, not requirements)

```bash
# Format code
gofmt -w .

# Check for issues
go vet ./...

# Lint (install with: go install golang.org/x/lint/golint@latest)
golint ./...
```

### Code Organization

- Keep functions small and focused (under 50 lines when possible)
- Use meaningful variable names
- Add comments for exported functions, types, and complex logic
- Group related functionality in the same file
- Avoid global variables when possible

### Naming Conventions

- **Variables**: `camelCase` for local, `PascalCase` for exported
- **Functions**: `PascalCase` for exported, `camelCase` for private
- **Files**: `snake_case.go`
- **Interfaces**: `PascalCase`, often ending in `er` (e.g., `Reader`, `Handler`)
- **Constants**: `PascalCase` for exported, `camelCase` for private

### Error Handling

- Always check and handle errors
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Return errors, don't panic (except for truly unrecoverable situations)
- Log errors at appropriate levels

**Good:**
```go
func processFile(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("failed to read file %s: %w", path, err)
    }
    // ... process data
    return nil
}
```

**Bad:**
```go
func processFile(path string) {
    data, _ := os.ReadFile(path) // Never ignore errors!
    // ... process data
}
```

### Logging

Use structured logging with appropriate levels:

```go
logger.Info("Starting server", "port", port)
logger.Debug("Request received", "method", method, "path", path)
logger.Error("Failed to process", "error", err)
```

### Comments

- Use complete sentences with proper punctuation
- Export godoc-style comments for all public APIs
- Explain the "why" not the "what" in complex code
- Update comments when code changes

**Good:**
```go
// AuthManager handles authentication and authorization for S3 requests.
// It validates HMAC-SHA256 signatures and manages access credentials.
type AuthManager struct {
    credentials map[string]*Credential
    mu          sync.RWMutex
}

// ValidateSignature checks if the provided signature matches the expected
// signature for the given request. This prevents unauthorized access by
// ensuring the client has the correct secret key.
func (am *AuthManager) ValidateSignature(accessKey, signature, stringToSign string) bool {
    // Implementation...
}
```

## Testing Guidelines

### Test Coverage

- Aim for at least 80% test coverage
- Test all exported functions
- Test error cases and edge cases
- Test concurrent access where relevant

### Test Structure

Use table-driven tests for multiple scenarios:

```go
func TestAuthManager_ValidateSignature(t *testing.T) {
    tests := []struct {
        name           string
        accessKey      string
        signature      string
        stringToSign   string
        expectedResult bool
    }{
        {
            name:           "valid signature",
            accessKey:      "test-key",
            signature:      "correct-signature",
            stringToSign:   "GET\n/bucket/key\n20230101",
            expectedResult: true,
        },
        {
            name:           "invalid signature",
            accessKey:      "test-key",
            signature:      "wrong-signature",
            stringToSign:   "GET\n/bucket/key\n20230101",
            expectedResult: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Test Best Practices

- Use meaningful test names that describe what is being tested
- Test one thing per test function
- Use subtests (`t.Run`) for related test cases
- Clean up resources in `defer` statements or `t.Cleanup()`
- Don't test internal implementation details
- Make tests deterministic (no random behavior)

### Integration Tests

Integration tests should:
- Test complete workflows
- Use realistic test data
- Be independent of each other
- Clean up after themselves
- Be skippable in CI if needed (use build tags or flags)

## Documentation

### Code Documentation

- Document all exported functions, types, and constants
- Use godoc format for API documentation
- Include examples in documentation where helpful
- Keep docs up to date with code changes

### User Documentation

When adding features, update:
- `README.md` - High-level overview
- `NEW_FEATURES.md` - Detailed feature documentation
- `QUICKSTART.md` - Quick start guides
- Example code in `examples/`
- API documentation

### Commit Messages

Write clear, descriptive commit messages:

**Format:**
```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Examples:**
```
feat(auth): add support for IAM role authentication

Implement IAM role-based authentication for EC2 instances.
This allows EC2 instances to authenticate without storing
credentials.

Closes #123
```

```
fix(gateway): handle nil pointer in request validation

Added nil check before accessing request headers to prevent
panic when malformed requests are received.

Fixes #456
```

## Pull Request Process

### Before Submitting

- [ ] Code follows project style guidelines
- [ ] All tests pass (`go test ./...`)
- [ ] New code has tests
- [ ] Documentation is updated
- [ ] Commits are clean and well-described
- [ ] Branch is up to date with main

### Submitting

1. **Push your branch** to your fork
2. **Create a pull request** to `main`
3. **Fill out the PR template** completely
4. **Link related issues** (e.g., "Closes #123")
5. **Wait for review** - be patient and responsive

### During Review

- **Respond to feedback** promptly
- **Make requested changes** in new commits
- **Don't force push** during review (makes it hard to see changes)
- **Ask questions** if feedback is unclear
- **Be respectful** and professional

### After Approval

- Maintainers will merge your PR
- Your contribution will be in the next release
- You'll be added to the contributors list!

## Community

### Communication

- **GitHub Issues** - Bug reports and feature requests
- **GitHub Discussions** - Questions and general discussion
- **Pull Requests** - Code contributions

### Getting Help

- Check existing documentation first
- Search closed issues for similar problems
- Ask questions in GitHub Discussions
- Be specific and provide context

### Recognition

Contributors are recognized in:
- GitHub contributors list
- Release notes
- Project README

## Development Tips

### Useful Commands

```bash
# Build optimized binary
go build -ldflags="-s -w" -o s3_server ./cmd/s3_server

# Run with debug logging
./s3_server -log_level debug

# Check for race conditions
go test -race ./...

# Profile CPU usage
go test -cpuprofile=cpu.prof -bench=.

# Profile memory usage
go test -memprofile=mem.prof -bench=.

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Performance Testing

```bash
# Benchmark tests
go test -bench=. -benchmem ./...

# Run performance tests
./test_performance.sh

# Benchmark with profiling
go test -bench=. -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

### Debugging

```bash
# Build with debug symbols
go build -gcflags="all=-N -l" -o s3_server ./cmd/s3_server

# Run with delve debugger
dlv debug ./cmd/s3_server -- -mode gateway -listen :9000
```

## License

By contributing, you agree that your contributions will be licensed under the same license as the project (MIT License).

## Questions?

Don't hesitate to ask! We're here to help:
- Open an issue with the `question` label
- Start a discussion in GitHub Discussions
- Check the documentation in the repo

## Thank You!

Your contributions make this project better for everyone. Thank you for being part of our community! 🎉

---

**Happy Contributing!** 🚀
