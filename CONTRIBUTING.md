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

- Go 1.18 or later
- Git

### Running Tests

```bash
# Run all tests
go test -v

# Run with race detection
go test -v -race

# Run benchmarks
go test -bench=. -benchmem

# Check coverage
go test -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Code Style

- Follow standard Go formatting: `go fmt`
- Run the linter: `go vet`
- Write clear, descriptive commit messages
- Add comments for exported functions and types

## Making Changes

### Before You Start

- Check existing issues to avoid duplicates
- For large changes, open an issue first to discuss
- Make sure tests pass before submitting

### Code Guidelines

1. **Write Tests**
   - Add tests for new functionality
   - Update tests for changed functionality
   - Aim for >70% code coverage

2. **Documentation**
   - Update README.md if adding features
   - Add godoc comments for exported items
   - Include examples in comments where helpful

3. **Commit Messages**
   ```
   Short summary (50 chars or less)

   More detailed explanation if needed. Wrap at 72 characters.

   - Bullet points are okay
   - Reference issues: Fixes #123
   ```

### What to Contribute

**We welcome:**
- Bug fixes
- Performance improvements
- Documentation improvements
- New backend implementations
- Test coverage improvements
- Example code

**Please discuss first:**
- Major architectural changes
- Breaking changes to public API
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
   - Include test results if applicable

3. **Code Review**:
   - Address review feedback promptly
   - Keep discussions professional and constructive
   - Be patient - reviews take time

## Pull Request Checklist

- [ ] Tests pass locally (`go test -v -race`)
- [ ] Code is formatted (`go fmt`)
- [ ] No lint errors (`go vet`)
- [ ] Documentation updated if needed
- [ ] Commit messages are clear
- [ ] Branch is up to date with main

## Backend Implementation Guide

If you're adding a new storage backend:

1. **Implement the `Backend` interface**:
   ```go
   type Backend interface {
       Get(ctx context.Context, key string) ([]byte, error)
       Put(ctx context.Context, key string, data []byte) error
       Delete(ctx context.Context, key string) error
       // ... other methods
   }
   ```

2. **Add compliance tests**:
   ```go
   func TestYourBackend_Compliance(t *testing.T) {
       backend := NewYourBackend(...)
       RunBackendComplianceTests(t, backend)
   }
   ```

3. **Update documentation**:
   - Add example to README.md
   - Document any special configuration

## Reporting Issues

### Bug Reports

Include:
- Go version (`go version`)
- OS and architecture
- Minimal reproduction case
- Expected vs actual behavior
- Error messages and stack traces

### Feature Requests

Include:
- Use case description
- Proposed API or behavior
- Why existing features don't work
- Willingness to implement

## Questions?

- Check existing issues and discussions
- Open a new issue with the "question" label
- Be specific about what you're trying to do

## Code of Conduct

- Be respectful and inclusive
- Focus on what is best for the community
- Show empathy towards others
- Accept constructive criticism gracefully

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

---

Thank you for contributing to SmarterBase!
