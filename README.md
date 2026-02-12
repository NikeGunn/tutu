<p align="center">
  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/License-MIT-7C3AED?style=for-the-badge" alt="License">
  <img src="https://img.shields.io/badge/MCP-2025--03--26-06B6D4?style=for-the-badge" alt="MCP Protocol">
  <img src="https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-22C55E?style=for-the-badge" alt="Platform">
  <img src="https://img.shields.io/github/v/release/Tutu-Engine/tutuengine?style=for-the-badge&color=F59E0B" alt="Release">
</p>

<p align="center">
  <strong>Run AI Locally. Power a Global Supercomputer.</strong>
</p>

<p align="center">
  <a href="https://tutuengine.tech">Website</a> ·
  <a href="https://tutuengine.tech/docs.html">Documentation</a> ·
  <a href="https://github.com/Tutu-Engine/tutuengine/releases">Releases</a> ·
  <a href="https://github.com/Tutu-Engine/tutuengine/discussions">Discussions</a> ·
  <a href="CONTRIBUTING.md">Contributing</a>
</p>

---

# TuTu Engine

**TuTu Engine** is an open-source, agentic AI distributed computing platform written in Go. It enables anyone to run large language models locally with zero configuration, then optionally connect to a global peer-to-peer network that transforms idle GPUs worldwide into a unified AI supercomputer.

One binary. One command. Zero accounts. Zero cloud dependency.

```bash
# Install
curl -fsSL https://tutuengine.tech/install.sh | sh

# Run your first model
tutu run llama3

# That's it. AI is running on your machine.
```

---

## Table of Contents

- [Why TuTu Engine?](#why-tutu-engine)
- [Architecture Overview](#architecture-overview)
- [Feature Comparison](#feature-comparison)
- [Core Capabilities](#core-capabilities)
- [Quick Start](#quick-start)
- [CLI Reference](#cli-reference)
- [MCP Server (Model Context Protocol)](#mcp-server-model-context-protocol)
- [Credit System & Economics](#credit-system--economics)
- [AI Fine-Tuning](#ai-fine-tuning)
- [Distributed Network](#distributed-network)
- [Engagement & Gamification](#engagement--gamification)
- [API Reference](#api-reference)
- [Deployment](#deployment)
- [Configuration](#configuration)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

---

## Why TuTu Engine?

The AI industry is dominated by centralized cloud providers that charge premium prices, lock in your data, and control access. TuTu Engine takes a fundamentally different approach:

| Problem | TuTu Engine Solution |
|---------|---------------------|
| Cloud AI is expensive ($0.002–$0.06/token) | Run locally for **$0.00/token** — forever free |
| Data leaves your machine | **100% offline** by default — your data never leaves |
| Vendor lock-in (OpenAI, Anthropic, Google) | **OpenAI-compatible API** — drop-in replacement |
| Idle GPUs worldwide sit unused | **Distributed network** turns idle GPUs into a supercomputer |
| No incentive to contribute compute | **Credit system** rewards GPU contributors |
| Complex setup and configuration | **One command** install, zero accounts needed |
| Models are siloed and incompatible | **TuTufile** — universal model packaging format |
| No standard for AI tool use | **MCP Gateway** — enterprise Model Context Protocol |

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         TuTu Engine                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐  │
│   │   CLI    │  │  REST    │  │   MCP    │  │   Daemon     │  │
│   │ (Cobra) │  │  API     │  │ Gateway  │  │  (Background)│  │
│   └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────┬───────┘  │
│        │              │              │               │          │
│   ┌────▼──────────────▼──────────────▼───────────────▼──────┐  │
│   │                  Application Layer                       │  │
│   │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────┐  │  │
│   │  │Executor  │  │ Credit   │  │Engagement│  │TuTuFile│  │  │
│   │  │(Inference│  │ Service  │  │  Engine  │  │ Parser │  │  │
│   │  │ Engine)  │  │          │  │          │  │        │  │  │
│   │  └──────────┘  └──────────┘  └──────────┘  └────────┘  │  │
│   └─────────────────────────────────────────────────────────┘  │
│                              │                                  │
│   ┌──────────────────────────▼──────────────────────────────┐  │
│   │                Infrastructure Layer                      │  │
│   │                                                          │  │
│   │  ┌────────┐ ┌────────┐ ┌────────┐ ┌──────────────────┐  │  │
│   │  │Process │ │SQLite  │ │Metrics │ │   P2P Network    │  │  │
│   │  │ Pool   │ │ Store  │ │(Prom.) │ │  ┌────────────┐  │  │  │
│   │  └────────┘ └────────┘ └────────┘ │  │   Gossip   │  │  │  │
│   │  ┌────────┐ ┌────────┐ ┌────────┐ │  │  Protocol  │  │  │  │
│   │  │Sched-  │ │Auto-   │ │Self-   │ │  ├────────────┤  │  │  │
│   │  │ uler   │ │ scale  │ │ Heal   │ │  │ NAT Trav.  │  │  │  │
│   │  └────────┘ └────────┘ └────────┘ │  ├────────────┤  │  │  │
│   │  ┌────────┐ ┌────────┐ ┌────────┐ │  │ Federation │  │  │  │
│   │  │Anomaly │ │Repu-   │ │Govern- │ │  ├────────────┤  │  │  │
│   │  │Detect  │ │ tation │ │ ance   │ │  │ Planetary  │  │  │  │
│   │  └────────┘ └────────┘ └────────┘ │  │  Routing   │  │  │  │
│   │  ┌────────┐ ┌────────┐ ┌────────┐ │  └────────────┘  │  │  │
│   │  │ML      │ │Fine-   │ │Market- │ └──────────────────┘  │  │
│   │  │Sched.  │ │ tune   │ │ place  │                       │  │
│   │  └────────┘ └────────┘ └────────┘                       │  │
│   └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

**Clean Architecture** — The codebase follows hexagonal architecture with strict dependency rules:

```
cmd/tutu/          → Entry point
internal/cli/      → CLI layer (Cobra commands)
internal/api/      → HTTP API layer (Chi router)
internal/mcp/      → MCP Gateway (JSON-RPC 2.0)
internal/app/      → Application services (business logic)
internal/domain/   → Domain models (zero dependencies)
internal/infra/    → Infrastructure (30+ packages)
internal/daemon/   → Background daemon orchestrator
internal/health/   → Health checking subsystem
internal/security/ → Cryptographic primitives
```

---

## Feature Comparison

### vs. Other AI Platforms

| Feature | TuTu Engine | Ollama | LM Studio | OpenAI API | Hugging Face |
|---------|:-----------:|:------:|:---------:|:----------:|:------------:|
| **Local inference** | ✅ | ✅ | ✅ | ❌ | ⚠️ |
| **Zero config setup** | ✅ | ✅ | ✅ | ❌ | ❌ |
| **OpenAI-compatible API** | ✅ | ✅ | ✅ | ✅ | ❌ |
| **100% offline** | ✅ | ✅ | ✅ | ❌ | ❌ |
| **Distributed computing** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **P2P network** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Credit economy** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **MCP Gateway** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Model fine-tuning** | ✅ | ❌ | ❌ | ✅ | ✅ |
| **Gamification/engagement** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Self-healing infrastructure** | ✅ | ❌ | ❌ | N/A | N/A |
| **Democratic governance** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Anomaly detection** | ✅ | ❌ | ❌ | N/A | N/A |
| **Custom model packaging** | ✅ (TuTufile) | ✅ (Modelfile) | ❌ | ❌ | ✅ |
| **Enterprise SLA tiers** | ✅ | ❌ | ❌ | ✅ | ✅ |
| **Free forever** | ✅ | ✅ | ⚠️ | ❌ | ⚠️ |

### vs. Distributed Computing Platforms

| Feature | TuTu Engine | BOINC | Folding@home | Golem | Akash |
|---------|:-----------:|:-----:|:------------:|:-----:|:-----:|
| **AI-native** | ✅ | ❌ | ❌ | ⚠️ | ⚠️ |
| **Zero configuration** | ✅ | ❌ | ✅ | ❌ | ❌ |
| **Credit economy** | ✅ | ✅ | ✅ | ✅ (tokens) | ✅ (tokens) |
| **No blockchain required** | ✅ | ✅ | ✅ | ❌ | ❌ |
| **Reputation system** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Local-first** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Model marketplace** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Fine-tuning support** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Gossip protocol** | ✅ (SWIM) | ❌ | ❌ | ❌ | ❌ |
| **Self-healing** | ✅ | ❌ | ❌ | ❌ | ❌ |

---

## Core Capabilities

### 🧠 Local AI Inference
Run LLMs locally with zero cloud dependency. TuTu manages model downloads, quantization, GPU/CPU scheduling, and an LRU process pool that keeps frequently-used models warm.

### 🌐 Distributed Supercomputer
Connect to the global P2P network and your idle GPU joins a planetary-scale AI supercomputer. SWIM gossip protocol, NAT traversal, federation across regions, and BitTorrent-style model distribution.

### 💰 Credit Economy
Double-entry bookkeeping credit system. Earn credits by contributing compute. Spend credits to use network resources. Anti-fraud protection with velocity checks and Benford's Law analysis.

### 🔌 MCP Gateway
Enterprise-grade Model Context Protocol server implementing the 2025-03-26 specification. Streamable HTTP transport, 4 SLA tiers (Free/Pro/Business/Enterprise), metered billing, JSON-RPC 2.0.

### 🎮 Engagement Engine
100-level progression system, 25+ achievements, weekly quests, daily streaks, smart notifications. Keeps contributors engaged and rewarded.

### 🤖 AI Fine-Tuning
Distributed LoRA and QLoRA fine-tuning across the network. Define training jobs, distribute across peers, earn credits for contributing training compute.

### 📦 TuTufile Packaging
Universal model packaging format. Define model parameters, system prompts, templates, adapters, and metadata in a single declarative file.

### 🏛️ Democratic Governance
On-chain-free democratic voting for network decisions. Quadratic voting, proposals, delegate system. The community governs the network.

---

## Quick Start

### Install

**macOS / Linux:**
```bash
curl -fsSL https://tutuengine.tech/install.sh | sh
```

**Windows (PowerShell):**
```powershell
irm https://tutuengine.tech/install.ps1 | iex
```

**Build from Source:**
```bash
git clone https://github.com/Tutu-Engine/tutuengine.git
cd tutuengine
make build
```

### First Run

```bash
# Run a model (auto-downloads if not present)
tutu run llama3

# Chat with the model
tutu run llama3 "What is distributed computing?"

# Start the API server
tutu serve

# List running models
tutu ps

# Show model info
tutu show llama3
```

### Use the API

```bash
# OpenAI-compatible chat completion
curl http://localhost:11434/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

```python
# Python (OpenAI SDK — drop-in replacement)
from openai import OpenAI

client = OpenAI(base_url="http://localhost:11434/v1", api_key="tutu")
response = client.chat.completions.create(
    model="llama3",
    messages=[{"role": "user", "content": "Explain quantum computing"}]
)
print(response.choices[0].message.content)
```

---

## CLI Reference

| Command | Description | Example |
|---------|-------------|---------|
| `tutu run <model>` | Run a model (download if needed) | `tutu run llama3 "Hello"` |
| `tutu pull <model>` | Download a model | `tutu pull mistral` |
| `tutu create <name>` | Create model from TuTufile | `tutu create mymodel -f TuTufile` |
| `tutu list` | List local models | `tutu list` |
| `tutu show <model>` | Show model details | `tutu show llama3` |
| `tutu ps` | Show running models | `tutu ps` |
| `tutu stop <model>` | Stop a running model | `tutu stop llama3` |
| `tutu rm <model>` | Remove a model | `tutu rm mistral` |
| `tutu serve` | Start API + MCP server | `tutu serve --port 11434` |
| `tutu progress` | Show engagement progress | `tutu progress` |
| `tutu agent status` | Show agent/network status | `tutu agent status` |
| `tutu agent join` | Join the distributed network | `tutu agent join` |
| `tutu agent earnings` | Show credit earnings | `tutu agent earnings` |
| `tutu agent donate` | Donate credits | `tutu agent donate 100` |

### Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--host` | Bind address | `127.0.0.1` |
| `--port` | Bind port | `11434` |
| `--verbose` | Enable verbose logging | `false` |

---

## MCP Server (Model Context Protocol)

TuTu Engine implements a full **MCP Gateway** following the [Model Context Protocol 2025-03-26](https://spec.modelcontextprotocol.io/) specification.

### What is MCP?

MCP is an open standard for connecting AI models to external tools, data sources, and services. Think of it as **USB-C for AI** — a universal connector that lets any AI model interact with any tool.

### How Companies Use TuTu's MCP Gateway

```
┌──────────────┐     ┌──────────────────┐     ┌──────────────┐
│  AI Client   │────▶│  TuTu MCP        │────▶│  External    │
│  (Claude,    │     │  Gateway          │     │  Services    │
│   ChatGPT,   │◀────│  ┌────────────┐  │◀────│  (DBs, APIs, │
│   Custom)    │     │  │ Tools      │  │     │   Files)     │
└──────────────┘     │  │ Resources  │  │     └──────────────┘
                     │  │ Metering   │  │
                     │  │ SLA Tiers  │  │
                     │  └────────────┘  │
                     └──────────────────┘
```

**Enterprise Use Cases:**

| Use Case | Description |
|----------|-------------|
| **AI Coding Assistants** | Connect your IDE AI to local databases, file systems, and CI/CD pipelines via MCP tools |
| **Customer Support Bots** | Give AI agents access to your CRM, knowledge base, and ticketing system through MCP resources |
| **Data Analysis Pipelines** | Let AI models query your data warehouse, run SQL, and generate reports via MCP tool calls |
| **DevOps Automation** | AI agents manage infrastructure through MCP-exposed Kubernetes, Docker, and cloud provider tools |
| **Document Processing** | Feed documents to AI models through MCP resources for summarization, extraction, and classification |

### MCP Endpoints

```bash
# Start TuTu with MCP enabled
tutu serve

# MCP endpoint (Streamable HTTP)
POST http://localhost:11434/mcp

# Initialize MCP session
curl -X POST http://localhost:11434/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"my-app","version":"1.0"}}}'
```

### Available MCP Tools

| Tool | Description |
|------|-------------|
| `tutu_run` | Run a model with given prompt |
| `tutu_list` | List available local models |
| `tutu_pull` | Download a model from registry |
| `tutu_status` | Get system and model status |

### SLA Tiers

| Tier | Rate Limit | Burst | Latency Target | Price |
|------|-----------|-------|-----------------|-------|
| **Free** | 10 req/min | 20 | Best effort | $0 |
| **Pro** | 100 req/min | 200 | < 500ms | Credits |
| **Business** | 1,000 req/min | 2,000 | < 200ms | Credits |
| **Enterprise** | 10,000 req/min | 20,000 | < 100ms | Credits |

---

## Credit System & Economics

TuTu's credit system is the economic backbone of the distributed network.

### How Credits Work

```
┌───────────────────────────────────────────────────────────┐
│                   Credit Lifecycle                         │
├───────────────────────────────────────────────────────────┤
│                                                           │
│   ┌──────────┐    ┌──────────┐    ┌──────────────────┐   │
│   │  EARN    │───▶│  WALLET  │───▶│    SPEND         │   │
│   │          │    │          │    │                    │   │
│   │ • GPU    │    │ • Balance│    │ • Use network AI  │   │
│   │   time   │    │ • Ledger │    │ • Fine-tune       │   │
│   │ • Models │    │ • Audit  │    │ • Priority queue  │   │
│   │ • Uptime │    │ • Trail  │    │ • MCP Pro tier    │   │
│   └──────────┘    └──────────┘    └──────────────────┘   │
│                                                           │
└───────────────────────────────────────────────────────────┘
```

### Earning Formula

```
Credits = GPU_Hours × Performance_Multiplier × Reliability_Bonus
```

| Factor | Multiplier | Description |
|--------|-----------|-------------|
| Base GPU hour | 1.0× | Standard earning rate |
| High-end GPU (A100, H100) | 2.5× | Premium hardware bonus |
| 99%+ uptime | 1.3× | Reliability bonus |
| First 30 days | 1.5× | Early adopter bonus |
| Referral bonus | +50 per referral | Invite new contributors |

### Spending Credits

| Action | Cost |
|--------|------|
| Network inference (per token) | 0.001 credits |
| Fine-tuning job (per hour) | 10 credits |
| MCP Pro tier (per month) | 50 credits |
| Priority queue access | 5 credits/request |
| Model marketplace purchase | Varies |

### Anti-Fraud Protection

- **Double-entry bookkeeping** — every transaction is balanced
- **Velocity checks** — abnormal earning patterns are flagged
- **Benford's Law analysis** — statistical fraud detection
- **Minimum balance enforcement** — prevents negative balances
- **Audit trails** — full transaction history

### Buying Credits

For users who don't want to contribute GPU time:

| Package | Credits | Price | Best For |
|---------|---------|-------|----------|
| Starter | 500 | Free | Getting started (included) |
| Developer | 5,000 | $9.99/mo | Individual developers |
| Team | 25,000 | $39.99/mo | Small teams |
| Enterprise | 100,000 | $149.99/mo | Organizations |
| Custom | Unlimited | Contact us | Large-scale deployments |

---

## AI Fine-Tuning

Fine-tune models using TuTu Engine's distributed infrastructure or locally on your own hardware.

### Local Fine-Tuning

```bash
# Create a fine-tuning TuTufile
cat > Tutufile <<EOF
FROM llama3
PARAMETER temperature 0.7
PARAMETER top_p 0.9
SYSTEM "You are a helpful customer support agent for Acme Corp."
ADAPTER ./my-lora-adapter
EOF

# Create the fine-tuned model
tutu create my-support-bot -f Tutufile
tutu run my-support-bot
```

### Distributed Fine-Tuning

Submit fine-tuning jobs to the TuTu network:

```bash
# Submit a LoRA fine-tuning job
tutu agent finetune \
  --base-model llama3 \
  --dataset ./training-data.jsonl \
  --method lora \
  --epochs 3 \
  --budget 100  # credits
```

### Supported Methods

| Method | VRAM Required | Speed | Quality | Cost |
|--------|:------------:|:-----:|:-------:|:----:|
| **Full fine-tune** | 48GB+ | Slow | Best | High |
| **LoRA** | 8GB+ | Fast | Great | Medium |
| **QLoRA** | 4GB+ | Fast | Good | Low |
| **Adapter merging** | 4GB+ | Instant | Good | Free |

---

## Distributed Network

### How the P2P Network Works

```
                    ┌─────────────────┐
                    │   Bootstrap     │
                    │    Nodes        │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
         ┌────▼────┐   ┌────▼────┐   ┌────▼────┐
         │ Region  │   │ Region  │   │ Region  │
         │ US-East │   │ EU-West │   │ AP-South│
         ├─────────┤   ├─────────┤   ├─────────┤
         │ ┌─────┐ │   │ ┌─────┐ │   │ ┌─────┐ │
         │ │Peer │ │   │ │Peer │ │   │ │Peer │ │
         │ │ A   │ │   │ │ D   │ │   │ │ G   │ │
         │ └─────┘ │   │ └─────┘ │   │ └─────┘ │
         │ ┌─────┐ │   │ ┌─────┐ │   │ ┌─────┐ │
         │ │Peer │ │   │ │Peer │ │   │ │Peer │ │
         │ │ B   │ │   │ │ E   │ │   │ │ H   │ │
         │ └─────┘ │   │ └─────┘ │   │ └─────┘ │
         │ ┌─────┐ │   │ ┌─────┐ │   │ ┌─────┐ │
         │ │Peer │ │   │ │Peer │ │   │ │Peer │ │
         │ │ C   │ │   │ │ F   │ │   │ │ I   │ │
         │ └─────┘ │   │ └─────┘ │   │ └─────┘ │
         └─────────┘   └─────────┘   └─────────┘
```

### Network Components

| Component | Technology | Purpose |
|-----------|-----------|---------|
| **Gossip Protocol** | SWIM | Member discovery, failure detection, state propagation |
| **NAT Traversal** | STUN/TURN/UPnP | 3-level NAT hole-punching for connectivity |
| **Federation** | Cross-region mesh | Connect independent TuTu clusters |
| **Planetary Routing** | Geo-aware DHT | Route requests to nearest capable peers |
| **Model Distribution** | BitTorrent-style | Chunk-based parallel model distribution |
| **Reputation System** | EigenTrust variant | Trust scoring for peer reliability |
| **Self-Healing** | Automatic recovery | Detect failures and redistribute workloads |
| **Anomaly Detection** | Statistical + ML | Identify malicious or misbehaving nodes |
| **Consistent Hashing** | Hash ring | Distribute load evenly across peers |
| **Democratic Governance** | Quadratic voting | Community-driven network decisions |

---

## Engagement & Gamification

TuTu Engine includes a full engagement system to reward and retain contributors.

### Level System

| Level Range | Title | Perks |
|:-----------:|:-----:|:------|
| 1–10 | Newcomer | Basic access, learning quests |
| 11–25 | Contributor | Priority queue, badge display |
| 26–50 | Builder | Beta features, voting rights |
| 51–75 | Expert | Governance participation, bonus multipliers |
| 76–100 | Legend | Network council, custom badges, max multipliers |

### Achievements (25+)

| Achievement | Requirement | Reward |
|-------------|-------------|--------|
| 🏁 First Run | Run your first model | 50 credits |
| 🌐 Network Pioneer | Join the distributed network | 100 credits |
| 🔥 Week Warrior | 7-day contribution streak | 200 credits |
| 💎 Diamond Contributor | 10,000 GPU hours contributed | 5,000 credits |
| 🏛️ Governance Leader | Submit 10 accepted proposals | 1,000 credits |
| 🧪 Fine-Tune Master | Complete 50 fine-tuning jobs | 2,500 credits |

### Weekly Quests

New quests generated every week:
- *Run 5 different models*
- *Contribute 24 hours of GPU time*
- *Help 3 network inference requests*
- *Create a custom TuTufile model*

### Streaks

Maintain daily streaks for bonus multipliers:

| Streak | Bonus |
|:------:|:-----:|
| 7 days | 1.1× earnings |
| 30 days | 1.25× earnings |
| 90 days | 1.5× earnings |
| 365 days | 2.0× earnings |

---

## API Reference

### OpenAI-Compatible Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/chat/completions` | Chat completion (streaming supported) |
| `POST` | `/v1/completions` | Text completion |
| `GET` | `/v1/models` | List available models |

### Ollama-Compatible Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/generate` | Generate text |
| `POST` | `/api/chat` | Chat conversation |
| `GET` | `/api/tags` | List models |
| `POST` | `/api/pull` | Pull a model |
| `POST` | `/api/create` | Create a model |
| `POST` | `/api/push` | Push a model |
| `DELETE` | `/api/delete` | Delete a model |
| `POST` | `/api/show` | Show model info |
| `POST` | `/api/copy` | Copy a model |

### MCP Endpoint

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/mcp` | MCP JSON-RPC 2.0 (Streamable HTTP) |

### System Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/` | Health check |
| `GET` | `/health` | Detailed health status |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/api/engagement/progress` | User progression |
| `GET` | `/api/earnings/stream` | SSE earnings stream |

---

## Deployment

### Railway (Recommended)

TuTu Engine is pre-configured for [Railway](https://railway.app) deployment:

1. Fork this repository
2. Connect to Railway
3. Railway auto-detects the Dockerfile
4. Set environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | `11434` |
| `TUTU_HOME` | Data directory | `/data` |
| `TUTU_LOG_LEVEL` | Log level | `info` |
| `TUTU_NETWORK_ENABLED` | Enable P2P | `false` |

The included `railway.json` handles all deployment configuration including health checks, restart policies, and resource limits.

### Docker

```bash
# Build
docker build -t tutuengine .

# Run
docker run -d \
  --name tutu \
  -p 11434:11434 \
  -v tutu-data:/data \
  tutuengine
```

### From Source

```bash
git clone https://github.com/Tutu-Engine/tutuengine.git
cd tutuengine
make build
./bin/tutu serve
```

---

## Configuration

TuTu Engine is configured via `~/.tutu/config.toml`:

```toml
[server]
host = "0.0.0.0"
port = 11434

[models]
dir = "~/.tutu/models"

[network]
enabled = false
bootstrap = ["bootstrap1.tutuengine.tech:9090"]
region = "auto"

[credits]
initial_balance = 500
earning_multiplier = 1.0

[mcp]
enabled = true
sla_tier = "free"

[engagement]
enabled = true
notifications = true

[logging]
level = "info"
format = "json"
```

---

## Roadmap

| Phase | Status | Description |
|:-----:|:------:|:------------|
| **Phase 0** | ✅ Complete | Core CLI, local inference, process pool |
| **Phase 1** | ✅ Complete | OpenAI-compatible API, model management |
| **Phase 2** | ✅ Complete | MCP Gateway, SLA tiers, metering |
| **Phase 3** | ✅ Complete | Credit system, double-entry bookkeeping |
| **Phase 4** | ✅ Complete | Engagement engine, gamification |
| **Phase 5** | ✅ Complete | P2P networking, gossip protocol, NAT traversal |
| **Phase 6** | ✅ Complete | Federation, marketplace, governance |
| **Phase 7** | ✅ Complete | Planetary scale, intelligence, fine-tuning |
| **Phase 8** | 🔜 Next | Public beta, mobile apps, browser extension |

---

## Contributing

We welcome contributions! Please read our [Contributing Guide](CONTRIBUTING.md) for details on:

- Code of conduct
- Development setup
- Pull request process
- Coding standards
- Testing requirements

---

## Security

For security vulnerabilities, please see our [Security Policy](SECURITY.md). Do **not** open public issues for security bugs.

---

## Community

- **GitHub Discussions** — [Ask questions, share ideas](https://github.com/Tutu-Engine/tutuengine/discussions)
- **Issues** — [Report bugs, request features](https://github.com/Tutu-Engine/tutuengine/issues)
- **Releases** — [Download latest version](https://github.com/Tutu-Engine/tutuengine/releases)

---

## License

TuTu Engine is open source under the [MIT License](LICENSE).

```
Copyright (c) 2026 TuTu Engine
```

---

<p align="center">
  <strong>Built with ❤️ by the TuTu Engine team</strong><br>
  <a href="https://tutuengine.tech">tutuengine.tech</a>
</p>
