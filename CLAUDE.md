# CLAUDE.md — TuTU Engine Project Brain

## What is TuTU?

TuTU Engine is an open-source, distributed AI computing platform in Go. One binary, zero config, zero cloud dependency. Users run LLMs locally, contribute idle GPU to a P2P network for credits, and access an OpenAI-compatible API.

## Quick Commands

```bash
make build          # Build binary → bin/tutu.exe
make test           # Run all tests with coverage (no -race flag — SQLite constraint)
make lint           # golangci-lint run
make clean          # Remove artifacts and cache
make deps           # go mod download && go mod tidy
make cover          # HTML coverage report
make serve          # Build + start API server
```

## Architecture

Clean Architecture (Hexagonal) with strict unidirectional dependencies:

```
cmd/tutu/main.go              → Entry point, calls cli.Execute()
    │
    ▼
Interface Layer                → CLI (Cobra), REST API (Chi), MCP Gateway (JSON-RPC 2.0)
    │
    ▼
Application Services           → Business orchestration (credit, engagement, executor, tutufile)
    │
    ▼
Domain Layer                   → Pure types, ZERO external imports (Golden Rule)
    │
    ▼
Infrastructure Layer           → Implementations (engine, sqlite, gossip, p2p, nat, metrics, etc.)
```

**Golden Rule**: `internal/domain/` has ZERO external dependencies. It defines interfaces; infra implements them.

## Package Map

| Package | Path | Purpose |
|---------|------|---------|
| **Entry** | `cmd/tutu/` | `main()` → `cli.Execute()` |
| **CLI** | `internal/cli/` | Cobra commands: run, pull, create, list, show, ps, stop, rm, serve, progress, agent |
| **API** | `internal/api/` | Chi HTTP server — OpenAI `/v1/*` + Ollama `/api/*` compatible |
| **MCP** | `internal/mcp/` | MCP Gateway — JSON-RPC 2.0, Streamable HTTP, SLA tiers |
| **Credit** | `internal/app/credit/` | Double-entry bookkeeping ledger |
| **Engagement** | `internal/app/engagement/` | Gamification: streaks, XP, achievements, quests |
| **Executor** | `internal/app/executor/` | Model inference orchestration |
| **TuTufile** | `internal/app/tutufile.go` | Dockerfile-like model packaging format parser |
| **Domain** | `internal/domain/` | Pure types: Model, Credit, Peer, Task, MCP, Engagement, interfaces, errors |
| **Engine** | `internal/infra/engine/` | Process pool with ref counting, LRU eviction, idle reaping |
| **SQLite** | `internal/infra/sqlite/` | Embedded DB (modernc.org/sqlite), WAL mode |
| **Gossip** | `internal/infra/gossip/` | SWIM protocol — O(log N) membership convergence |
| **P2P** | `internal/infra/p2p/` | Peer distribution, distributed task scheduling |
| **NAT** | `internal/infra/nat/` | NAT traversal (STUN/TURN/UPnP) |
| **Federation** | `internal/infra/federation/` | Cross-region mesh networking |
| **Metrics** | `internal/infra/metrics/` | Prometheus counters, gauges, histograms |
| **Autoscale** | `internal/infra/autoscale/` | Dynamic resource allocation |
| **Anomaly** | `internal/infra/anomaly/` | ML-based malicious node detection |
| **Healing** | `internal/infra/healing/` | Self-recovery orchestration |
| **Reputation** | `internal/infra/reputation/` | EigenTrust-variant peer trust scoring |
| **Governance** | `internal/infra/governance/` | Democratic quadratic voting, proposals |
| **Marketplace** | `internal/infra/marketplace/` | Model catalog and discovery |
| **ML Scheduler** | `internal/infra/mlscheduler/` | Distributed ML job scheduling |
| **Fine-tune** | `internal/infra/finetune/` | LoRA/QLoRA training coordinator |
| **DSA** | `internal/infra/dsa/` | Bloom filter, hash ring, heap |
| **Daemon** | `internal/daemon/` | Background daemon lifecycle |
| **Health** | `internal/health/` | 5-check health suite (SQLite, disk, model integrity, API, network) |
| **Security** | `internal/security/` | Cryptographic signing and verification |

## Key Domain Types

```go
// internal/domain/model.go
ModelInfo, ModelRef, Manifest, Layer, InferenceRequest, Token

// internal/domain/credit.go
LedgerEntry, EntryType("DEBIT"/"CREDIT"), TransactionType("EARN"/"SPEND"/"BOND"/...)

// internal/domain/engagement.go
Streak, UserLevel, AchievementDef, UserStats

// internal/domain/peer.go
Peer, PeerState("ALIVE"/"SUSPECT"/"DEAD")

// internal/domain/task.go
Task, TaskStatus("QUEUED"/"ASSIGNED"/"EXECUTING"/"COMPLETED"/"FAILED"), TaskType

// internal/domain/mcp.go
MCPClient, MCPTool, SLATier("realtime"/"standard"/"batch"/"spot"), SLAConfig

// internal/domain/interfaces.go
InferenceEngine, ModelStore, ModelManager (implemented by infra)

// internal/domain/errors.go
Domain-specific sentinel errors
```

## API Surface

### OpenAI-Compatible (`/v1/`)
- `POST /v1/chat/completions` — Chat (streaming)
- `POST /v1/completions` — Text completion
- `GET  /v1/models` — List models
- `POST /v1/embeddings` — Embeddings

### Ollama-Compatible (`/api/`)
- `POST /api/generate` — Generate text
- `POST /api/chat` — Chat
- `GET  /api/tags` — List models
- `POST /api/show` — Model details
- `POST /api/pull` — Download model
- `POST /api/create` — Create from Modelfile
- `POST /api/delete` — Delete model
- `POST /api/ps` — Running processes

### MCP Gateway (`POST /mcp`)
JSON-RPC 2.0: `initialize`, `tools/list`, `tools/call`, `resources/list`, `resources/read`

### System
- `GET /health` — Health check
- `GET /metrics` — Prometheus
- `GET /api/status` — Status
- `GET /api/version` — Version
- `GET /api/engagement/progress` — Gamification progress
- `GET /api/earnings/stream` — SSE earnings feed

## Configuration

Location: `~/.tutu/config.toml` (or `$TUTU_HOME/config.toml`)

Key sections: `[node]`, `[api]`, `[models]`, `[inference]`, `[logging]`, `[network]`, `[resources]`, `[security]`, `[telemetry]`, `[mcp]`, `[agent]`

Defaults: host `127.0.0.1`, port `11434`, GPU layers auto, context 4096, batch 512

## Tech Stack

| Component | Library | Why |
|-----------|---------|-----|
| Language | Go 1.24+ | Static binary, concurrency |
| CLI | `spf13/cobra` | Standard Go CLI framework |
| HTTP | `go-chi/chi/v5` | Lightweight composable router |
| Database | `modernc.org/sqlite` | Pure Go, zero CGO, WAL mode |
| Metrics | `prometheus/client_golang` | Industry standard |
| Config | `BurntSushi/toml` | Human-friendly config |
| UUID | `google/uuid` | Standard generation |

## Code Conventions

### Architecture Rules
- Domain layer has ZERO external imports — ever
- Infra implements domain interfaces via constructor injection
- All async operations accept `context.Context` for cancellation
- Streaming via channels: `<-chan domain.Token`

### Error Handling
- Always wrap errors with context: `fmt.Errorf("doing X: %w", err)`
- Use domain sentinel errors for business logic
- Never swallow errors silently

### Naming
- Packages: lowercase, singular (`credit`, `scheduler`)
- Types: PascalCase (`ModelInfo`, `LedgerEntry`)
- Errors: `Err` prefix (`ErrNotFound`, `ErrInsufficientCredits`)
- Test files: `*_test.go` in same package (white-box)
- Test functions: `Test<Type>_<Method>` or `Test<Type>_<Method>_<Scenario>`

### Concurrency Patterns
- `sync.Mutex` for critical sections (model pool)
- `sync.RWMutex` for read-heavy state (health checker)
- `atomic` operations for reference counting
- `defer` for cleanup — always
- Channels for event streaming and synchronization

### Key Design Patterns
- **Pool + Ref Counting**: `Acquire()` → use → `defer Release()` — O(1) ops, zero leaks
- **Double-Entry Ledger**: Every credit txn creates balanced DEBIT + CREDIT entries
- **Table-Driven Tests**: For input variations
- **Arrange-Act-Assert**: For clarity in test structure
- **Temp DB in Tests**: `t.TempDir()` + `t.Cleanup()` for isolation

## Testing

```bash
make test                          # All tests with coverage
go test ./internal/domain/...      # Single package
go test -run TestCreditService ./internal/app/credit/...  # Single test
```

Coverage targets: domain 90%, app 85%, api 80%, mcp 80%, infra 75%, cli 70%

Test DB pattern:
```go
func newTestDB(t *testing.T) *sqlite.DB {
    db, _ := sqlite.Open(t.TempDir())
    t.Cleanup(func() { db.Close() })
    return db
}
```

No `-race` flag — modernc.org/sqlite has known race detector false positives.

## Deployment

- **Railway**: Pre-configured `railway.json` + `railway.toml`, Docker multi-stage build
- **Docker**: `golang:1.24-bookworm` builder → `distroless/static-debian12:nonroot` runtime
- **Env vars**: `PORT` (default 8080), `TUTU_HOME`, `TUTU_LOG_LEVEL`, `TUTU_NETWORK_ENABLED`

## Git Conventions

- Feature branches, conventional commits: `feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, `test:`
- All PRs require CI pass + maintainer approval
- PR size guide: XS (<50 lines), S (50-200), M (200-500), L (500-1000), XL (1000+) — prefer small
