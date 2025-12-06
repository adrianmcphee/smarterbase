# Contributing to SmarterBase

Thank you for your interest in contributing to SmarterBase! This document provides guidelines for contributing to the project.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/smarterbase.git
   cd smarterbase
   ```
3. **Create a branch** for your changes:
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Setup

### Prerequisites

- Go 1.21 or later
- Git

### Building

```bash
# Build the binary
make build

# Run the server
make run
```

### Running Tests

```bash
# Run all tests
make test

# Run with race detection
make test-race

# Quick dev cycle
make dev
```

### Code Style

- Follow standard Go formatting: `go fmt`
- Run the linter: `go vet`
- Write clear, descriptive commit messages

## Project Structure

```
smarterbase/
├── cmd/smarterbase/     # CLI entry point
├── internal/
│   ├── protocol/        # PostgreSQL wire protocol
│   ├── executor/        # SQL execution
│   └── storage/         # JSONL file storage
├── e2e/                 # End-to-end tests
└── docs/                # Documentation
```

## Making Changes

### Before You Start

- Check existing issues to avoid duplicates
- For large changes, open an issue first to discuss
- Make sure tests pass before submitting

### What to Contribute

**We welcome:**
- Bug fixes
- Performance improvements
- Documentation improvements
- Additional SQL support
- Test coverage improvements

**Please discuss first:**
- Major architectural changes
- Breaking changes to SQL compatibility
- New dependencies

## Submitting Changes

1. **Push to your fork**:
   ```bash
   git push origin feature/your-feature-name
   ```

2. **Open a Pull Request**:
   - Use a clear, descriptive title
   - Reference any related issues
   - Describe what changed and why

3. **Code Review**:
   - Address review feedback promptly
   - Keep discussions professional

## Pull Request Checklist

- [ ] Tests pass locally (`make test`)
- [ ] Code is formatted (`go fmt ./...`)
- [ ] No lint errors (`go vet ./...`)
- [ ] Documentation updated if needed
- [ ] Commit messages are clear

## Reporting Issues

### Bug Reports

Include:
- Go version (`go version`)
- OS and architecture
- Minimal reproduction case
- Expected vs actual behavior
- Error messages

### Feature Requests

Include:
- Use case description
- Proposed SQL syntax or behavior
- Why existing features don't work

## License

By contributing, you agree that your contributions will be licensed under the BSL 1.1 License.

---

Thank you for contributing to SmarterBase!
