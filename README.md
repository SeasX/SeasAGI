[�� English](README.md) | [�� 中文](README.zh-CN.md) | [🇯🇵 日本語](README.ja.md) | [🇰🇷 한국어](README.ko.md)

---

# SeasAGI

**Local LLM API Intelligent Gateway** — Open-source client + Server Open Core model, unified management of multiple LLM Provider channels, providing intelligent routing, session stickiness, cost optimization, full-chain observability, and team collaborative governance.

---

## Overview

SeasAGI is a layered AI gateway platform consisting of three sub-projects:

| Sub-project | License | Description |
|-------------|---------|-------------|
| `SeasAGI-Client/` | **GPL 3.0** fully open-source | Native desktop client (Wails + Go + React), local unified gateway, API Keys never uploaded |
| `SeasAGI-Server/` | **AGPL 3.0** open-source self-hostable | Community edition cloud control plane, basic auth / Combo CRUD / basic usage / basic relay forwarding |
| `SeasAGI-Server-Enterprise/` | Closed-source | Enterprise edition cloud, includes Stripe billing / multi-tenant governance / Combo governance approval / admin dashboard. See [Enterprise README](SeasAGI-Server-Enterprise/README.md) |

The client can **run independently** — all local gateway features work without a Server. The cloud is an optional value-add service providing remote accelerated channels, usage sync, team collaboration, and enterprise governance.

---

## Core Capabilities

### Client (SeasAGI-Client)

| Capability | Description |
|------------|-------------|
| **Unified API Gateway** | Local `localhost:4318`, OpenAI-compatible, seamlessly replace any API Client's Base URL |
| **Adaptive Intelligent Routing** | Fallback / Round-Robin / Sticky strategies, Combo step-chain orchestration (primary → backup → last-resort) |
| **Session Stickiness** | SHA1 hash-based session→step mapping, 30min TTL, prevents mid-conversation model switching |
| **Dynamic 429 Penalty Degradation** | PenaltyManager, 429 +3 (cap 10), success -1, 2min natural decay |
| **Cost Optimization Engine** | Per-model/channel grouped analysis + task-type differentiated suggestions + four quick-strategy one-click presets |
| **8 Executors + 6 Translators** | OpenAI / Azure / Anthropic / Gemini / Ollama / DeepSeek / Grok / Relay + automatic protocol adaptation |
| **Full-chain Observability** | Error classification distribution / request timeline / link diagnostics / Recharts visualization / cost estimation |
| **Auto Health-check & Fault Tolerance** | Key health check (5min probe + auto-disable) + per-key Cooldown + circuit breaker + exponential backoff |
| **Local-first · Privacy Secure** | API Keys stored only in Keychain, constant-time comparison prevents timing attacks, requests go direct to Provider |
| **Combo Optimization Workbench** | My Plans / Optimization Suggestions / Template Center / Execution Analysis — four tabs, Combo drag-and-drop sorting |
| **Playground Instant Test** | Built-in chat test UI, Combo selector + execution chain visualization + step fallback status |
| **i18n** | Chinese / English / Japanese / Korean |

### Server Community Edition (SeasAGI-Server)

| Capability | Description |
|------------|-------------|
| Basic Auth | Login / Register / Token refresh |
| Basic Channel Management | Platform channel CRUD |
| Basic Combo | User-level Combo CRUD + official template fetch |
| Basic Usage Statistics | User usage, per-model/channel grouping, timeline, error distribution |
| Basic Tenant Management | Members, invite links, custom channel sync, config snapshots |
| Basic Admin API | User / Plan / Channel / Combo / Relay Gateway management |
| Relay Basic Forwarding | Request passthrough, health check, rate limiting, trace |

---

## Architecture Overview

```text
┌─────────────────────────────────────────────────────────────┐
│                    SeasAGI-Client (GPL 3.0)                  │
│                                                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌─────────────┐ │
│  │  Local   │  │  Router  │  │   Cost   │  │  Full-chain │ │
│  │ Gateway  │  │  Engine  │  │Optimizer │  │  Observable │ │
│  │ :4318/v1 │  │Combo/    │  │Suggestions│  │Log/Diag/    │ │
│  │ OpenAI   │  │Fallback  │  │Quick Strat│  │Chart/Cost   │ │
│  └──────────┘  └──────────┘  └──────────┘  └─────────────┘ │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌─────────────┐ │
│  │8 Executor│  │6 Translator│ │ Keychain │  │ Playground  │ │
│  │ Provider │  │Protocol  │  │Secure    │  │Instant Test │ │
│  │          │  │Adaptation│  │Storage   │  │             │ │
│  └──────────┘  └──────────┘  └──────────┘  └─────────────┘ │
└──────────────────────┬──────────────────────────────────────┘
                       │ API Protocol
┌──────────────────────┴──────────────────────────────────────┐
│                SeasAGI-Server  /  -Enterprise               │
│                                                             │
│  ┌───────────────────────┐  ┌─────────────────────────────┐ │
│  │  Community (AGPL 3.0) │  │  Enterprise (Closed-source) │ │
│  │                       │  │                             │ │
│  │  Basic Auth · Channel │  │  Billing · Plan Gatekeeping │ │
│  │  Combo CRUD · Usage   │  │  Multi-tenant · Combo Gov.  │ │
│  │  Relay Fwd · Deploy   │  │  Advanced Metrics · BYOK    │ │
│  │                       │  │  Admin Dashboard · SSO/SCIM │ │
│  └───────────────────────┘  └─────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │  relay-gateway: Relay data plane, multi-modal fwd +    │ │
│  │                 health check + trace                    │ │
│  └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

---

## Open-source / Closed-source Boundary

| Component | License | Open-source Scope |
|-----------|---------|-------------------|
| `SeasAGI-Client/` | **GPL 3.0** | Fully open-source, including local gateway, Provider executors, routing strategies, security modules, frontend UI |
| `SeasAGI-Server/` | **AGPL 3.0** | Community edition open-source, basic control plane + basic relay, self-hostable |
| `SeasAGI-Server-Enterprise/` | **Closed-source (BSL / EULA)** | Enterprise edition, carries billing, multi-tenant governance, Combo governance, admin dashboard |

---

## Repository Structure

```text
SeasAGI/
├── SeasAGI-Client/                Open-source client (GPL 3.0)
│   ├── src-app/                    Wails main source (Go + React)
│   ├── scripts/                    Build scripts
│   └── homebrew/                   Homebrew distribution formula
├── SeasAGI-Server/                Open-source server community edition (AGPL 3.0)
│   ├── platform-api/               Community control plane
│   ├── relay-gateway/              Community relay data plane
│   ├── deploy/                     Deploy scripts + systemd + nginx
│   └── scripts/                    Build scripts
├── web-docs/                      Official website & docs
│   ├── src-web/                    Static website
│   └── ...
├── build-all.sh                   One-click build script
├── SeasAGI.v5.md                  One-page summary
```

---

## Quick Start

### Download Client

Download the latest release for your platform from [GitHub Releases](https://github.com/SeasX/SeasAGI/releases).

### Configure Channels

1. Install and launch SeasAGI Client
2. Add your Provider API Key in "Channel Management"
3. Change any API Client's Base URL to `http://127.0.0.1:4318/v1`
4. Enjoy intelligent routing!

### Self-host Server Community Edition

```bash
# Build
bash SeasAGI-Server/scripts/build.sh

# Install to Linux server (requires root)
sudo bash SeasAGI-Server/deploy/install.sh

# Configure environment variables
vi /etc/seasagi/platform-api.env
# Set JWT_SECRET, JWT_REFRESH_SECRET, ADMIN_SECRET

# Start services
sudo systemctl enable --now seasagi-platform-api
sudo systemctl enable --now seasagi-relay-gateway
```

---

## Build

### One-click Build (All Editions)

```bash
# Build Client + Server Community + Server Enterprise
bash ./build-all.sh

# Multi-platform Client
bash ./build-all.sh --platform darwin/amd64,darwin/arm64,linux/amd64
```

### Individual Build

```bash
# Client
bash SeasAGI-Client/scripts/build.sh

# Server Community Edition
bash SeasAGI-Server/scripts/build.sh

# Server Enterprise Edition
bash SeasAGI-Server-Enterprise/scripts/build.sh
```

### Build Artifacts

| Artifact | Path |
|----------|------|
| Client app | `SeasAGI-Client/src-app/build/bin/` |
| Client DMG | `SeasAGI-Client/build/` |
| Server Community binaries | `SeasAGI-Server/build/` |
| Server Enterprise binaries | `SeasAGI-Server-Enterprise/build/` |

---

## Tech Stack

| Layer | Technology |
|-------|------------|
| Frontend | React + TypeScript + Vite + Zustand + @dnd-kit + Zod + Recharts |
| Desktop Framework | Wails v2 (Go + WebView) |
| Client Backend | Go (local gateway / routing engine / cost optimization / Provider executors) |
| Server Framework | Go + Gin |
| Database | SQLite |
| Payments | Stripe (Enterprise) |
| Deployment | systemd + nginx |
| Client Distribution | Homebrew Cask / DMG / Binary |

---

## Documentation

- [One-page Summary](SeasAGI.v5.md)
- [System Architecture Design Doc](web-docs/系统架构设计文档.v5.md)
- [Open-source Analysis](开源分析.md)
- [Client README](SeasAGI-Client/README.md)
- [Server Community README](SeasAGI-Server/README.md)
- [Server Enterprise README](SeasAGI-Server-Enterprise/README.md)

---

## License

- `SeasAGI-Client/` — **GPL 3.0**, fully open-source
- `SeasAGI-Server/` — **AGPL 3.0**, community edition open-source self-hostable
- `SeasAGI-Server-Enterprise/` — **Closed-source (BSL / EULA)**, for commercial deployment

---

## Community

- [GitHub Issues](https://github.com/SeasX/SeasAGI/issues) — Report bugs / Submit suggestions
- [GitHub Releases](https://github.com/SeasX/SeasAGI/releases) — Download latest version

---
