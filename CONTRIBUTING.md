# Contributing to TuTu Engine

Thank you for your interest in contributing to TuTu Engine! This document provides guidelines and standards for contributing to the project. We take quality seriously — every contribution shapes the world's largest distributed AI supercomputer.

---

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Project Architecture](#project-architecture)
- [Coding Standards](#coding-standards)
- [Git Workflow](#git-workflow)
- [Pull Request Process](#pull-request-process)
- [Testing Requirements](#testing-requirements)
- [Documentation](#documentation)
- [Issue Guidelines](#issue-guidelines)
- [Security Vulnerabilities](#security-vulnerabilities)
- [Release Process](#release-process)
- [Community](#community)
- [License](#license)

---

## Code of Conduct

By participating in this project, you agree to uphold our [Code of Conduct](CODE_OF_CONDUCT.md). We are committed to providing a welcoming and inclusive experience for everyone.

**Key principles:**
- Be respectful and constructive
- Welcome newcomers and help them learn
- Focus on what is best for the community
- Show empathy towards other community members

---

## Getting Started

### Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| **Go** | 1.22+ | Primary language |
| **Git** | 2.30+ | Version control |
| **Make** | Any | Build automation |
| **golangci-lint** | Latest | Code linting |

### Quick Setup

```bash
# 1. Fork the repository
# Visit https://github.com/Tutu-Engine/tutuengine and click "Fork"

# 2. Clone your fork
git clone https://github.com/<your-username>/tutuengine.git
cd tutuengine

# 3. Add upstream remote
git remote add upstream https://github.com/Tutu-Engine/tutuengine.git

# 4. Install dependencies
make deps

# 5. Build
make build

# 6. Run tests
make test

# 7. Verify it works
./bin/tutu --help
```

---

## Development Setup

### Environment Configuration

Create a development configuration at `~/.tutu/config.toml`:

```toml
[server]
host = "127.0.0.1"
port = 11434

[logging]
level = "debug"
format = "text"

[network]
enabled = false
```

### Editor Setup

We recommend **VS Code** with the following extensions:
- `golang.go` — Go language support
- `streetsidesoftware.code-spell-checker` — Catch typos
- `eamodio.gitlens` — Git history visualization

**VS Code settings (.vscode/settings.json):**
```json
{
  "go.lintTool": "golangci-lint",
  "go.lintFlags": ["--fast"],
  "go.formatTool": "goimports",
  "editor.formatOnSave": true,
  "editor.rulers": [100],
  "[go]": {
    "editor.codeActionsOnSave": {
      "source.organizeImports": "explicit"
    }
  }
}
```

### Makefile Commands

| Command | Description |
|---------|-------------|
| `make build` | Build the binary |
| `make test` | Run all tests with coverage |
| `make lint` | Run golangci-lint |
| `make clean` | Remove build artifacts |
| `make deps` | Download and tidy dependencies |
| `make cover` | Generate coverage report in browser |
| `make serve` | Build and start API server |

---

## Project Architecture

TuTu Engine follows **Clean Architecture** (hexagonal architecture) with strict dependency rules.

### Directory Structure

```
tutu/
├── cmd/tutu/              # Entry point — main.go only
│   └── main.go
├── internal/              # Private application code
│   ├── cli/               # CLI commands (Cobra)
│   │   ├── root.go        #   Root command and global flags
│   │   ├── run.go         #   tutu run
│   │   ├── pull.go        #   tutu pull
│   │   ├── create.go      #   tutu create
│   │   ├── list.go        #   tutu list
│   │   ├── show.go        #   tutu show
│   │   ├── ps.go          #   tutu ps
│   │   ├── stop.go        #   tutu stop
│   │   ├── rm.go          #   tutu rm
│   │   ├── serve.go       #   tutu serve
│   │   ├── progress.go    #   tutu progress
│   │   ├── agent.go       #   tutu agent *
│   │   └── helpers.go     #   Shared CLI utilities
│   ├── api/               # HTTP API layer (Chi router)
│   │   ├── server.go      #   Server setup, routing
│   │   ├── tutu_api.go    #   Ollama-compatible endpoints
│   │   ├── openai.go      #   OpenAI-compatible endpoints
│   │   └── engagement.go  #   Engagement API endpoints
│   ├── mcp/               # MCP Gateway (JSON-RPC 2.0)
│   │   ├── gateway.go     #   MCP request handler
│   │   ├── transport.go   #   Streamable HTTP transport
│   │   ├── jsonrpc.go     #   JSON-RPC protocol
│   │   ├── meter.go       #   Usage metering
│   │   └── sla.go         #   SLA tier management
│   ├── app/               # Application services
│   │   ├── tutufile.go    #   TuTufile parser
│   │   ├── executor/      #   Model inference engine
│   │   ├── credit/        #   Credit system service
│   │   └── engagement/    #   Gamification service
│   ├── domain/            # Domain models (ZERO dependencies)
│   │   ├── model.go       #   Core model types
│   │   ├── interfaces.go  #   Repository interfaces
│   │   ├── credit.go      #   Credit types
│   │   ├── engagement.go  #   Engagement types
│   │   ├── mcp.go         #   MCP types
│   │   ├── peer.go        #   P2P peer types
│   │   ├── task.go        #   Task types
│   │   ├── social.go      #   Social types
│   │   ├── region.go      #   Region types
│   │   ├── resource.go    #   Resource types
│   │   └── errors.go      #   Domain errors
│   ├── infra/             # Infrastructure implementations
│   │   ├── engine/        #   Process pool / inference
│   │   ├── sqlite/        #   Database layer
│   │   ├── scheduler/     #   Task scheduler
│   │   ├── p2p/           #   P2P networking
│   │   ├── gossip/        #   SWIM gossip protocol
│   │   ├── nat/           #   NAT traversal
│   │   ├── federation/    #   Cross-region federation
│   │   ├── network/       #   Network utilities
│   │   ├── metrics/       #   Prometheus metrics
│   │   ├── autoscale/     #   Auto-scaling
│   │   ├── anomaly/       #   Anomaly detection
│   │   ├── healing/       #   Self-healing
│   │   ├── selfheal/      #   Self-healing orchestration
│   │   ├── reputation/    #   Trust scoring
│   │   ├── governance/    #   Democratic governance
│   │   ├── democracy/     #   Voting system
│   │   ├── marketplace/   #   Model marketplace
│   │   ├── catalog/       #   Model catalog
│   │   ├── registry/      #   Model registry
│   │   ├── intelligence/  #   AI intelligence layer
│   │   ├── mlscheduler/   #   ML job scheduler
│   │   ├── finetune/      #   Fine-tuning engine
│   │   ├── flywheel/      #   Data flywheel
│   │   ├── observability/ #   Observability stack
│   │   ├── resource/      #   Resource management
│   │   ├── region/        #   Region management
│   │   ├── planetary/     #   Planetary routing
│   │   ├── universal/     #   Universal compute
│   │   ├── passive/       #   Passive income
│   │   └── dsa/           #   Data structures (bloom, heap, hashring)
│   ├── daemon/            #   Background daemon
│   ├── health/            #   Health checking
│   └── security/          #   Cryptographic primitives
├── website/               # Landing page and docs
├── Dockerfile             # Production container
├── Makefile               # Build automation
├── railway.json           # Railway deployment config
└── go.mod                 # Go module definition
```

### Dependency Rules

```
cmd → cli → api, app, daemon
             ↓
            app → domain (business logic)
             ↓
           infra → domain (implementations)

domain has ZERO external dependencies
infra NEVER imports from cli, api, or app
```

**The golden rule:** `domain/` contains only pure Go types and interfaces. No imports from other internal packages, no third-party imports.

---

## Coding Standards

### Go Style Guide

We follow the official [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments) and [Effective Go](https://golang.org/doc/effective_go).

#### Naming Conventions

| Item | Convention | Example |
|------|-----------|---------|
| Packages | Short, lowercase, singular | `credit`, `scheduler`, `p2p` |
| Interfaces | `-er` suffix when possible | `ModelRunner`, `CreditEarner` |
| Exported types | PascalCase | `ProcessPool`, `MCPGateway` |
| Unexported types | camelCase | `gossipState`, `peerEntry` |
| Constants | PascalCase or ALL_CAPS | `MaxRetries`, `DefaultPort` |
| Errors | `Err` prefix | `ErrModelNotFound` |
| Test functions | `Test` + function name | `TestCreditService_Earn` |

#### Code Formatting

```go
// ✅ Good: Clear, documented, handles errors
// ProcessInference routes an inference request to an available process.
func (p *Pool) ProcessInference(ctx context.Context, req InferenceRequest) (*InferenceResult, error) {
    if err := req.Validate(); err != nil {
        return nil, fmt.Errorf("invalid request: %w", err)
    }

    proc, err := p.acquire(ctx)
    if err != nil {
        return nil, fmt.Errorf("acquiring process: %w", err)
    }
    defer p.release(proc)

    return proc.Run(ctx, req)
}

// ❌ Bad: No docs, no error wrapping, unclear naming
func (p *Pool) Do(r Request) (*Result, error) {
    proc, err := p.get()
    if err != nil {
        return nil, err
    }
    return proc.Run(r)
}
```

#### Error Handling

```go
// ✅ Always wrap errors with context
if err != nil {
    return fmt.Errorf("loading model %s: %w", name, err)
}

// ✅ Use domain errors for business logic
if balance < cost {
    return domain.ErrInsufficientCredits
}

// ❌ Never discard errors silently
result, _ := doSomething()  // BAD
```

#### Package Organization

```go
// ✅ One responsibility per file
// credit.go      — Credit type definitions
// credit_test.go — Credit tests

// ✅ Tests in same package for white-box testing
package credit

// ✅ Test files follow _test.go convention
func TestCreditService_Earn(t *testing.T) { ... }
func TestCreditService_Spend(t *testing.T) { ... }
```

### Code Quality Checklist

Before submitting a PR, verify:

- [ ] `make build` succeeds with zero warnings
- [ ] `make test` passes with all tests green
- [ ] `make lint` reports zero issues
- [ ] New code has >80% test coverage
- [ ] All exported functions have doc comments
- [ ] Error messages are lowercase, no trailing punctuation
- [ ] No hardcoded secrets or credentials
- [ ] No `fmt.Println` debugging left in code
- [ ] Concurrent code uses proper synchronization
- [ ] Context propagation is correct

---

## Git Workflow

### Branch Naming

```
feature/   — New features          feature/mcp-streaming-support
bugfix/    — Bug fixes             bugfix/credit-race-condition
hotfix/    — Urgent production fix hotfix/security-patch-xss
docs/      — Documentation only    docs/api-reference-update
refactor/  — Code refactoring      refactor/scheduler-cleanup
test/      — Test improvements     test/engagement-coverage
chore/     — Maintenance tasks     chore/update-dependencies
```

### Commit Messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

**Types:**

| Type | Description |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation changes |
| `style` | Code style (formatting, no logic change) |
| `refactor` | Code refactoring |
| `perf` | Performance improvement |
| `test` | Adding or updating tests |
| `chore` | Build, CI, dependencies |
| `ci` | CI/CD configuration |
| `revert` | Revert a previous commit |

**Scopes:** `cli`, `api`, `mcp`, `credit`, `engagement`, `p2p`, `scheduler`, `daemon`, `domain`, `infra`, `docs`, `docker`

**Examples:**

```bash
feat(mcp): add streaming support for tool responses
fix(credit): prevent race condition in balance updates
docs(api): add OpenAI endpoint examples
perf(scheduler): reduce allocation in hot path
test(engagement): add streak edge case coverage
chore(deps): update go-chi to v5.2.5
```

### Workflow

```bash
# 1. Sync with upstream
git fetch upstream
git checkout main
git merge upstream/main

# 2. Create feature branch
git checkout -b feature/your-feature

# 3. Make changes, commit frequently
git add .
git commit -m "feat(scope): description"

# 4. Push and open PR
git push origin feature/your-feature
```

---

## Pull Request Process

### Before Opening a PR

1. **Rebase** on latest `main`:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Run the full check suite:**
   ```bash
   make test
   make lint
   make build
   ```

3. **Self-review** your changes:
   - Read the diff line by line
   - Remove debug code
   - Check for sensitive data

### PR Template

When opening a PR, include:

```markdown
## What

Brief description of what this PR does.

## Why

Why is this change needed? Link to issue if applicable.

## How

Technical approach and key decisions.

## Testing

- [ ] Unit tests added/updated
- [ ] Integration tests pass
- [ ] Manual testing performed

## Checklist

- [ ] Code follows project style guidelines
- [ ] Self-reviewed the code
- [ ] Added/updated documentation
- [ ] No breaking changes (or documented)
- [ ] Tests pass locally
```

### Review Process

1. **All PRs require at least 1 approval** from a maintainer
2. **CI must pass** — all tests, linting, and builds
3. **No merge conflicts** — rebase before merge
4. **Squash merge** for feature branches
5. Maintainers may request changes — address all feedback

### PR Size Guidelines

| Size | Lines Changed | Review Time |
|------|:------------:|:-----------:|
| **XS** | < 50 | Minutes |
| **S** | 50–200 | < 1 hour |
| **M** | 200–500 | < 4 hours |
| **L** | 500–1000 | < 1 day |
| **XL** | 1000+ | Split into smaller PRs |

**Prefer small, focused PRs.** Large PRs are harder to review and more likely to introduce bugs.

---

## Testing Requirements

### Test Coverage

| Package | Minimum Coverage |
|---------|:----------------:|
| `domain/` | 90% |
| `app/` | 85% |
| `api/` | 80% |
| `mcp/` | 80% |
| `infra/` | 75% |
| `cli/` | 70% |

### Writing Tests

```go
func TestCreditService_Earn(t *testing.T) {
    // Arrange
    svc := credit.NewService(credit.DefaultConfig())

    // Act
    earned, err := svc.Earn("user-1", 100, "gpu-contribution")

    // Assert
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if earned != 100 {
        t.Errorf("earned = %d, want 100", earned)
    }
}
```

### Test Naming Convention

```go
func Test<Type>_<Method>(t *testing.T)           // Happy path
func Test<Type>_<Method>_<Scenario>(t *testing.T) // Specific scenario
func Test<Type>_<Method>_Error(t *testing.T)      // Error case
```

### Table-Driven Tests

```go
func TestParseModel(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    Model
        wantErr bool
    }{
        {"simple name", "llama3", Model{Name: "llama3"}, false},
        {"with tag", "llama3:7b", Model{Name: "llama3", Tag: "7b"}, false},
        {"empty", "", Model{}, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseModel(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Running Tests

```bash
# All tests
make test

# Specific package
go test ./internal/app/credit/...

# With verbose output
go test -v -count=1 ./internal/mcp/...

# Coverage report
make cover
```

---

## Documentation

### Code Documentation

Every exported symbol must have a doc comment:

```go
// ProcessPool manages a pool of model inference processes.
// It uses LRU eviction to keep frequently-used models warm
// and supports concurrent access from multiple goroutines.
type ProcessPool struct { ... }

// Acquire returns an available process for the given model,
// starting a new one if necessary. It blocks until a process
// is available or the context is cancelled.
func (p *ProcessPool) Acquire(ctx context.Context, model string) (*Process, error) { ... }
```

### Website Documentation

Website files are in `website/`. When adding new features:

1. Update `docs.html` with technical documentation
2. Update `index.html` if it affects user-facing features
3. Test locally by opening files in a browser

---

## Issue Guidelines

### Bug Reports

Include:
- **TuTu version** (`tutu --version`)
- **OS and architecture** (`go env GOOS GOARCH`)
- **Steps to reproduce**
- **Expected vs actual behavior**
- **Logs** (with `--verbose` flag)

### Feature Requests

Include:
- **Problem statement** — What problem does this solve?
- **Proposed solution** — How should it work?
- **Alternatives considered** — What else did you think of?
- **Additional context** — Mockups, examples, references

### Labels

| Label | Description |
|-------|-------------|
| `bug` | Something isn't working |
| `enhancement` | New feature or improvement |
| `documentation` | Documentation improvements |
| `good first issue` | Good for newcomers |
| `help wanted` | Community help appreciated |
| `priority:high` | Urgent — blocks users |
| `priority:medium` | Important — next release |
| `priority:low` | Nice to have |

---

## Security Vulnerabilities

**Do NOT open public issues for security vulnerabilities.**

Please report security issues via:
1. Email: security@tutuengine.tech
2. GitHub Security Advisory (private)

See [SECURITY.md](SECURITY.md) for our full security policy.

---

## Release Process

Releases follow [Semantic Versioning](https://semver.org/):

```
MAJOR.MINOR.PATCH

MAJOR — Breaking API changes
MINOR — New features, backward-compatible
PATCH — Bug fixes, backward-compatible
```

### Release Checklist

1. Update version in `Makefile`
2. Update `CHANGELOG.md`
3. Create release branch: `release/vX.Y.Z`
4. Run full test suite
5. Create GitHub Release with tag `vX.Y.Z`
6. Build and publish binaries
7. Update documentation

---

## Community

### Where to Get Help

| Channel | Purpose |
|---------|---------|
| [GitHub Discussions](https://github.com/Tutu-Engine/tutuengine/discussions) | Questions, ideas, show & tell |
| [GitHub Issues](https://github.com/Tutu-Engine/tutuengine/issues) | Bug reports, feature requests |
| [Documentation](https://tutuengine.tech/docs.html) | Technical docs and guides |

### Recognition

All contributors are recognized in:
- GitHub contributor graph
- Release notes acknowledgments
- TuTu Engine Hall of Fame (coming soon)

---

## License

By contributing to TuTu Engine, you agree that your contributions will be licensed under the [MIT License](LICENSE).

---

<p align="center">
  <strong>Thank you for contributing to TuTu Engine!</strong><br>
  Every contribution, no matter how small, makes a difference.
</p>
