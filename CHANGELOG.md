# Changelog

All notable changes to TuTu Engine will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-02-12

### Added

- **Core CLI** — `tutu run`, `tutu pull`, `tutu create`, `tutu list`, `tutu show`, `tutu ps`, `tutu stop`, `tutu rm`, `tutu serve`, `tutu progress`
- **Agent Commands** — `tutu agent status`, `tutu agent join`, `tutu agent earnings`, `tutu agent donate`
- **OpenAI-Compatible API** — `/v1/chat/completions`, `/v1/completions`, `/v1/models`
- **Ollama-Compatible API** — `/api/generate`, `/api/chat`, `/api/tags`, `/api/pull`, `/api/create`, `/api/delete`, `/api/show`, `/api/copy`, `/api/push`
- **MCP Gateway** — Full Model Context Protocol 2025-03-26 implementation with Streamable HTTP transport, 4 SLA tiers, usage metering, JSON-RPC 2.0
- **Credit System** — Double-entry bookkeeping, earning formula with multipliers, anti-fraud protection (velocity checks, Benford's Law), referral bonuses
- **Engagement Engine** — 100-level progression, 25+ achievements, weekly quests, daily streaks, smart notifications
- **P2P Networking** — SWIM gossip protocol, 3-level NAT traversal (STUN/TURN/UPnP), BitTorrent-style model distribution
- **Federation** — Cross-region mesh networking, federated model catalog
- **Distributed Fine-Tuning** — LoRA and QLoRA support, ML job scheduler, distributed training across peers
- **Democratic Governance** — Quadratic voting, proposal system, delegate mechanisms
- **Anomaly Detection** — Statistical and ML-based malicious node detection
- **Self-Healing** — Automatic failure detection and workload redistribution
- **Reputation System** — EigenTrust-variant trust scoring for peer reliability
- **Auto-Scaling** — Dynamic resource allocation based on demand
- **Model Marketplace** — Share and discover custom models and adapters
- **Planetary Routing** — Geo-aware DHT for routing requests to nearest capable peers
- **Process Pool** — LRU-based inference process management with warm model caching
- **TuTufile Parser** — Dockerfile-like model packaging format
- **Health Checking** — Component-level health monitoring with detailed status reporting
- **Prometheus Metrics** — Full observability stack with `/metrics` endpoint
- **SQLite Storage** — Zero-dependency embedded database for all state
- **Multi-Stage Dockerfile** — Distroless runtime image for minimal attack surface
- **Railway Deployment** — Pre-configured `railway.json` with health checks and auto-restart
- **Website** — Landing page, documentation, install scripts for macOS/Linux/Windows
- **MIT License** — Open source forever

### Infrastructure

- Clean architecture (hexagonal) with strict dependency rules
- 30+ infrastructure packages
- Consistent hashing (hash ring) for load distribution
- Bloom filters for efficient membership testing
- Priority heap for task scheduling

[0.1.0]: https://github.com/Tutu-Engine/tutuengine/releases/tag/v0.1.0
