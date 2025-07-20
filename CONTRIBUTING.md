# Contributing to GoFlow

Thank you for your interest in contributing to GoFlow! This document provides guidelines and information for contributors.

## 🤝 How to Contribute

We welcome contributions from the community! Here are the main ways you can contribute:

- 🐛 **Bug Reports**: Report bugs and issues
- 💡 **Feature Requests**: Suggest new features and improvements
- 📝 **Documentation**: Improve documentation and examples
- 🔧 **Code Contributions**: Submit pull requests with code changes
- 🧪 **Testing**: Add tests or improve test coverage

## 🚀 Development Setup

### Prerequisites

- **Go**: Version 1.24 or higher
- **Git**: For version control
- **PostgreSQL**: For integration tests (optional - Docker can be used)
- **Docker**: For running PostgreSQL in containers (optional)

### Getting Started

1. **Fork the repository**
   ```bash
   # Fork on GitHub, then clone your fork
   git clone https://github.com/YOUR_USERNAME/goflow.git
   cd goflow
   ```

2. **Set up the development environment**
   ```bash
   # Install dependencies
   make deps
   
   # Start local database (uses Docker)
   make init_db
   
   # Run tests to ensure everything works
   make test
   ```

3. **Create a feature branch**
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/your-bug-fix
   ```

## 📋 Development Workflow

### 1. Make Your Changes

- Write your code following the [Code Style Guidelines](#code-style-guidelines)
- Add tests for new functionality
- Update documentation as needed
- Ensure all tests pass

### 2. Test Your Changes

```bash
# Run all tests
make test

# Run tests with coverage
go test -race -count=1 -shuffle=on -coverprofile=coverage.out ./...

# Run linter
make lint

# Format code
make fmt

# Run integration tests (requires PostgreSQL)
make integration_tests
```

### 3. Commit Your Changes

Follow the [Conventional Commits](https://www.conventionalcommits.org/) specification. This is **critical** for automated versioning:

```bash
# Examples of good commit messages:
git commit -m "feat: add retry mechanism for failed tasks"
git commit -m "fix: resolve race condition in worker pool"
git commit -m "docs: update README with library usage examples"
git commit -m "test: add unit tests for task timeout handling"
git commit -m "refactor: simplify workflow execution logic"
git commit -m "feat!: breaking change in API interface"
```

**Commit Types and Version Impact:**
- `feat`: New feature (minor version bump)
- `fix`: Bug fix (patch version bump)
- `chore`: Maintenance tasks (patch version bump)
- `docs`: Documentation changes (no version bump)
- `style`: Code style changes (no version bump)
- `refactor`: Code refactoring (no version bump)
- `test`: Adding or updating tests (no version bump)

**Breaking Changes:**
- Use `!` after the type: `feat!: breaking change`
- Or include `BREAKING CHANGE:` in the commit body
- This triggers a major version bump

### 4. Push and Create a Pull Request

```bash
git push origin feature/your-feature-name
```

Then create a Pull Request on GitHub with:
- Clear description of changes
- Reference to any related issues
- Screenshots (if UI changes)
- Test results

## 📝 Code Style Guidelines

### Go Code Style

- Follow [Effective Go](https://golang.org/doc/effective_go.html) guidelines
- Use `gofmt` for code formatting (run `make fmt`)
- Follow Go naming conventions:
  - `camelCase` for variables and functions
  - `PascalCase` for exported names
  - `UPPER_CASE` for constants

### Code Quality

- Write clear, readable code with meaningful variable names
- Add comments for complex logic
- Keep functions small and focused
- Use interfaces for better testability
- Handle errors explicitly

### Example

```go
// Good
func (s *WorkflowService) ExecuteFlow(ctx context.Context, workflowID int64, flowName string) (TaskResult, error) {
    if workflowID <= 0 {
        return nil, fmt.Errorf("invalid workflow ID: %d", workflowID)
    }
    
    // Validate flow exists
    if _, exists := s.flows[flowName]; !exists {
        return nil, fmt.Errorf("flow '%s' not found", flowName)
    }
    
    // Execute workflow logic...
    return result, nil
}

// Avoid
func (s *WorkflowService) ExecuteFlow(ctx context.Context, id int64, name string) (interface{}, error) {
    if id <= 0 {
        return nil, errors.New("bad id")
    }
    // ... rest of function
}
```

## 🧪 Testing Guidelines

### Test Requirements

- **Unit Tests**: Required for all new functionality
- **Integration Tests**: Required for database and API changes
- **Test Coverage**: Aim for at least 80% coverage
- **Race Condition Tests**: Use `-race` flag for concurrent code

### Writing Tests

```go
func TestWorkflowService_ExecuteFlow(t *testing.T) {
    // Arrange
    store := storage.NewMockStore()
    logger := log.GetLogger()
    svc := service.NewWorkflowService(context.Background(), store, logger)
    
    // Register test task
    svc.RegisterTask("test_task", func(ctx context.Context, args ...service.TaskResult) (service.TaskResult, error) {
        return "test_result", nil
    }, nil)
    
    // Act
    workflowID, err := svc.CreateWorkflow("test-workflow")
    assert.NoError(t, err)
    
    result, err := svc.ExecuteFlow(context.Background(), workflowID, "test_task")
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, "test_result", result)
}
```

### Running Tests

```bash
# Run all tests
make test

# Run specific package tests
go test ./pkg/service

# Run tests with verbose output
go test -v ./pkg/service

# Run tests with coverage
go test -cover ./pkg/service

# Run benchmarks
go test -bench=. ./pkg/service
```

## 📚 Documentation Guidelines

### Code Documentation

- Add comments for exported functions and types
- Use [godoc](https://golang.org/pkg/go/doc/) style comments
- Include examples for complex functions

```go
// WorkflowService manages workflow instances and their flow executions.
// A workflow is a specific instance of a pipeline run, persisted with a unique ID.
// A flow is a reusable definition of tasks and dependencies, executed within a workflow.
type WorkflowService struct {
    // ... fields
}

// ExecuteFlow runs a flow for a given workflow.
// It returns the result of the flow execution or an error if the execution fails.
func (s *WorkflowService) ExecuteFlow(ctx context.Context, workflowID int64, flowName string) (TaskResult, error) {
    // ... implementation
}
```

### README and Documentation

- Update README.md for user-facing changes
- Add examples for new features
- Update API documentation
- Include migration guides for breaking changes

## 🐛 Bug Reports

When reporting bugs, please include:

1. **Clear description** of the issue
2. **Steps to reproduce** the problem
3. **Expected behavior** vs actual behavior
4. **Environment details**:
   - Go version
   - Operating system
   - Database version (if applicable)
5. **Code examples** or error messages
6. **Screenshots** (if UI-related)

### Bug Report Template

```markdown
## Bug Description
[Clear description of the issue]

## Steps to Reproduce
1. [Step 1]
2. [Step 2]
3. [Step 3]

## Expected Behavior
[What should happen]

## Actual Behavior
[What actually happens]

## Environment
- Go version: [version]
- OS: [operating system]
- Database: [version, if applicable]

## Additional Information
[Any other relevant information]
```

## 💡 Feature Requests

When suggesting new features:

1. **Clear description** of the feature
2. **Use case** and motivation
3. **Proposed implementation** (if you have ideas)
4. **Examples** of how it would be used
5. **Consideration** of backward compatibility

## 🔄 Pull Request Process

### Before Submitting

1. **Ensure tests pass**
   ```bash
   make test
   make lint
   make fmt
   make vet
   ```

2. **Update documentation** for any new features or API changes

3. **Add examples** for new functionality

4. **Check for breaking changes** and document them

### Pull Request Guidelines

- **Title**: Clear, descriptive title
- **Description**: Detailed explanation of changes
- **Related Issues**: Link to any related issues
- **Breaking Changes**: Document any breaking changes
- **Testing**: Describe how you tested the changes

### Pull Request Template

```markdown
## Description
[Description of changes]

## Type of Change
- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update

## Testing
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing completed

## Checklist
- [ ] Code follows the style guidelines
- [ ] Self-review of code completed
- [ ] Comments added for complex code
- [ ] Documentation updated
- [ ] No breaking changes (or breaking changes documented)

## Related Issues
Closes #[issue number]
```

## 🏷️ Release Process

### Automated Versioning

GoFlow uses automated semantic versioning based on conventional commits. The system automatically:

1. **Analyzes commits** since the last release
2. **Determines version bump** based on commit types:
   - `feat:` → Minor version bump
   - `fix:` → Patch version bump
   - `BREAKING CHANGE:` → Major version bump
3. **Updates version files** (VERSION, go.mod, CHANGELOG.md)
4. **Creates git tags** and GitHub releases
5. **Publishes to Go module proxy**

### Manual Release Process

For manual releases or when you want to control the release timing:

```bash
# Check current version
make version

# See what the next version would be
make version-next

# Bump version (updates files only)
make version-bump

# Create release (bump, tag, push)
make version-release
```

### Release Checklist

- [ ] All tests pass (`make test`)
- [ ] Code is linted (`make lint`)
- [ ] Code is vetted (`make vet`)
- [ ] Documentation is up to date
- [ ] Conventional commits are used
- [ ] Version files are updated
- [ ] Git tag is created
- [ ] GitHub release is created

### Versioning Rules

We follow [Semantic Versioning](https://semver.org/) (SemVer):

- **MAJOR** (X.0.0): Breaking changes
- **MINOR** (0.X.0): New features (backward compatible)
- **PATCH** (0.0.X): Bug fixes (backward compatible)

## 🤝 Community Guidelines

### Code of Conduct

- Be respectful and inclusive
- Help others learn and grow
- Provide constructive feedback
- Follow the project's coding standards

### Communication

- Use GitHub Issues for bug reports and feature requests
- Use GitHub Discussions for questions and general discussion
- Be patient and helpful with new contributors

## 🆘 Getting Help

If you need help with contributing:

1. **Check existing issues** and discussions
2. **Read the documentation** and examples
3. **Ask questions** in GitHub Discussions
4. **Open an issue** for bugs or feature requests

## 🙏 Recognition

Contributors will be recognized in:
- GitHub contributors list
- Release notes
- Project documentation

Thank you for contributing to GoFlow! 🚀 