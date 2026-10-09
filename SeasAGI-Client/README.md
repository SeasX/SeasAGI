[🇬🇧 English](README.md) | [🇨🇳 中文](README.zh-CN.md) | [🇯🇵 日本語](README.ja.md) | [🇰🇷 한국어](README.ko.md)

---

# SeasAGI Client

[![Build status](https://ci.appveyor.com/api/projects/status/github/SeasX/SeasAGI?svg=true)](https://ci.appveyor.com/project/SeasX/SeasAGI)
[![macOS](https://img.shields.io/badge/platform-macOS-blue)](https://github.com/SeasX/SeasAGI)
[![Linux](https://img.shields.io/badge/platform-Linux-blue)](https://github.com/SeasX/SeasAGI)
[![Windows](https://img.shields.io/badge/platform-Windows-blue)](https://github.com/SeasX/SeasAGI)
[![License](https://img.shields.io/badge/license-GPL%20v3-green)](LICENSE)

Client sub-project, containing the desktop mainline source code and client build scripts.

## Directory Structure

```text
SeasAGI-Client/
├── README.md
├── homebrew/
├── scripts/
│   ├── build.sh
│   ├── build-macos.sh
│   ├── build-linux.sh
│   ├── build-windows.sh
│   └── dev.sh
└── src-app/
    ├── app.go
    ├── main.go
    ├── oauth_desktop.go
    ├── wails.json
    ├── frontend/
    ├── internal/
    └── build/
```

## Responsibilities

- `src-app`: Wails v2 desktop client mainline source
- `src-app/frontend`: React + Vite + TypeScript frontend (with @dnd-kit drag-and-drop + Zod validation)
- `src-app/internal`: Local gateway, routing, OAuth, local access token, logging, optimization, usage tracking, and other Go modules
- `scripts/build.sh`: Client build entry point, auto-dispatches by current OS
- `scripts/build-macos.sh`: macOS build + DMG packaging
- `scripts/build-linux.sh`: Linux (Ubuntu/CentOS) build
- `scripts/build-windows.sh`: Windows build
- `scripts/dev.sh`: Wails local development entry point
- `homebrew/seasagi.rb`: macOS Homebrew distribution formula

## v5 Core Capabilities

### Combo Optimization Workbench (Unified Entry)

The left navigation has been consolidated into a single "Combo Optimization" entry, with 4 tabs in the workbench:

- **My Plans**: Three-layer Combo view (local/official/team) + source labels + cloud sync (Fetch/Push/Update/Delete)
- **Optimization Suggestions**: Smart multi-step fallback suggestions + structural diff preview + one-click apply as Combo
- **Template Center**: Official/team template search, refresh, and apply
- **Execution Analysis**: Combo hit rate / fallback rate / average attempt count analysis panel

### Model-level Combo

- Combo is fundamentally a "failure fallback orchestration": primary model → backup model → last-resort model (primary/backup/last_resort three-step role semantics)
- Step chain editing + drag-and-drop sorting (@dnd-kit) + step-level channel_id + model binding
- Fallback / Round-Robin strategies + Sticky Uses configuration
- Full cloud sync: `FetchCloudCombos` / `PushCloudCombo` / `UpdateCloudCombo` / `DeleteCloudCombo`
- Local migrator `migrateModelCombos()` auto-backfills old data to a unified schema (combo_id/logical_name/display_name/status/source/version)
- Playground instant test: Combo selector + execution chain visualization + step fallback status display (step_role badge)

### Smart Optimization & Combo Collaboration

- Optimizer supports multi-step fallback suggestions (primary/backup/last_resort)
- `PreviewComboOptimization` implements structural diff preview (current steps vs suggested steps)
- `ApplyComboOptimization` supports creating/overwriting Combos
- `ApplyRecommendation` supports one-click Combo generation
- Recommendations explicitly distinguish: model replacement suggestions / fallback chain enhancement suggestions / runtime parameter suggestions

### Routing & Strategies

- Fallback / Round-Robin / Sticky three basic strategies
- Combo step chain (explicit channel + model combination)
- Session sticky routing: SHA1 hash-based session→step mapping from first user message, 30min TTL
- Dynamic 429 penalty degradation: PenaltyManager, 429 +3 (cap 10), success -1, 2min natural decay
- Fallback sort presets: intelligence / speed / budget one-click reorder
- Combo step drag-and-drop sorting (@dnd-kit)

### Per-Step Provider/Channel Candidate Pool (OpenRouter Inspired)

- Each Step supports multiple candidates instead of a single fixed channel
- Intra-step sorting rules: auto-reorder candidates by stability/cost/latency/throughput
- Request-level dynamic constraints: `max_price`, `max_latency_ms`, `data_policy`, `allow_cross_provider_fallback`
- Tool-calling request independent routing strategy (prioritize providers with high tool success rate)
- Quick-strategy aliases: stability-first / cost-first / speed-first / tool-first
- Task-type awareness (task_profile): differentiated execution for chat / tools / json / long_context / batch_low_cost

### Provider & Fault Tolerance

- 9 Provider executors: OpenAI / Azure / Anthropic / Gemini / Ollama / DeepSeek / Grok / Vertex / Platform Relay
- 6 format translators: OpenAI Chat / OpenAI Responses / Anthropic / Gemini / Vertex AI / Passthrough
- Multi-key rotation + exponential backoff + circuit breaker + 7 error rules
- Key health check: HealthChecker, 5min periodic probe, 3 consecutive failures auto-mark unhealthy
- Key-level Cooldown: CooldownManager, per-key 120s TTL, triggered on 429
- Provider-level health scores participate in runtime sorting decisions
- Enterprise BYOK / Platform shared capacity dual-layer fallback strategy
- Multi-modal content flattening: text-only array content auto-merged to string

### Combo Runtime Instrumentation & Metrics

- `ComboRouteMetrics`: request count / fallback count / Step1 hit rate / last-step hit rate
- `step_role` propagated to RouteStep logs (primary/backup/last_resort)
- `GetComboRouteMetrics` exposed to frontend
- `ExecAnalysisPage` displays hit rate overview bar chart + average attempt count + Combo detail panel

### Security

- Keychain integrated secure storage
- Constant-time Key comparison (crypto/subtle.ConstantTimeCompare)
- Zod frontend form validation
- Tunnel remote access (Cloudflare Tunnel + Tailscale Funnel)
- OAuth 2.0 PKCE (4 Providers + Token auto-refresh)

### MITM Proxy — One-Click API Interception

- **One-click toggle**: Enable/disable MITM proxy from Settings → MITM tab, no manual certificate setup required
- **Automatic CA trust**: Auto-generates and installs root CA on macOS/Linux/Windows, dynamic per-domain certificate signing (23h TTL cache)
- **System proxy integration**: Automatically sets OS-level HTTP/HTTPS proxy, transparently intercepts all matching traffic
- **Domain allowlist**: Configure which API domains to intercept (defaults: OpenAI, Anthropic, Gemini, DeepSeek, Grok, OpenRouter), add/remove domains at runtime
- **Connectivity check**: Built-in per-domain test button verifies interception status, reachability, and latency
- **Local gateway routing**: Intercepted HTTPS traffic is transparently forwarded to the local SeasAGI gateway for unified routing, optimization, and observability
- **Live intercept log**: Real-time table of recent 20 intercepted requests (method, host, path, status code, latency), auto-refreshing every 3 seconds
- **CLI compatibility hints**: Auto-detects user's shell (bash/zsh/fish/PowerShell/cmd) and provides copy-paste `export` / `unset` commands for terminal tools that don't respect system proxy
- **Crash recovery**: Residual system proxy auto-cleanup on restart, health probe goroutine auto-stops on proxy failure
- **Passthrough safety**: Non-matching domains are transparently tunneled with 60s timeout, zero interference with normal browsing

#### Known limitations (not intercepted)

One-click interception is built on the system proxy (HTTP/HTTPS over TCP) and a local CA. The following traffic falls outside its scope and cannot be monitored or governed:

- **HTTP/3 (QUIC / UDP 443)**: the system proxy only handles TCP, so UDP-based traffic is not intercepted
- **h2 / gRPC long-lived connections**: only standard HTTP/HTTPS requests are parsed and governed; binary gRPC streams are not rewritten
- **Certificate-pinned clients**: apps that validate certificates internally reject the local CA and bypass MITM
- **Processes that ignore the system proxy**: CLIs/tools with their own network stack or an explicit proxy must set environment variables manually (see "CLI compatibility hints")
- **Traffic outside the interception allowlist**: only allowlisted domains are forwarded to the local gateway

> WebSocket (ws/wss) traffic reuses the same gateway main path and is fully intercepted and metered.

### Experience

- Playground instant test page
- Token Market: list unused API keys for sale (fixed price / discount), browse marketplace, QR code scan trading, order management, settlement & earnings
- RTK Token compression (9 output types auto-detection)
- Caveman concise output (4 styles)
- Reasoning Content injection
- MCP tool deduplication
- i18n (Chinese / English / Japanese / Korean)

## Version Management

Version numbers are maintained by a single file, injected during build, and not depended on at runtime.

### Version Source

[`scripts/version.txt`](file:///Users/Neeke/data/www/SeasAGI/SeasAGI/SeasAGI-Client/scripts/version.txt) is the single version definition file:

```
0.2.0
```

- Update the version by modifying this single file
- Can be overridden temporarily via the `SEASAGI_VERSION` environment variable (e.g., `SEASAGI_VERSION=1.0.0 bash build-all.sh`)

### Build-time Injection Flow

1. Each platform build script (`build-macos.sh` / `build-linux.sh` / `build-windows.sh`) reads `scripts/version.txt` on startup
2. Injects the version via `export VITE_APP_VERSION="$VERSION"` into the frontend build environment
3. During Vite build, `import.meta.env.VITE_APP_VERSION` is compiled as a static value into the frontend output
4. Wails bundles the frontend output into the desktop client binary

### Frontend Display

The version badge is shown to the right of the logo in the upper-left corner of the client UI:

```
[icon] SeasAGI  v0.2.0
```

Implementation location: [`Layout.tsx`](file:///Users/Neeke/data/www/SeasAGI/SeasAGI/SeasAGI-Client/src-app/frontend/src/components/Layout.tsx) renders it using `import.meta.env.VITE_APP_VERSION`.

### Build Independence

The version number is compiled as a static value embedded in the frontend output at build time. `scripts/version.txt` is not packaged into the client. The client runs completely independently from this file.

## Common Commands

```bash
# Start client development mode
bash SeasAGI-Client/scripts/dev.sh

# Build client for current platform
bash SeasAGI-Client/scripts/build.sh

# Build for specific platform
bash SeasAGI-Client/scripts/build-macos.sh   # macOS
bash SeasAGI-Client/scripts/build-linux.sh    # Linux (Ubuntu/CentOS)
bash SeasAGI-Client/scripts/build-windows.sh  # Windows
```

## Build Artifacts

- `SeasAGI-Client/src-app/build/bin/SeasAGI.app`
- `SeasAGI-Client/src-app/build/bin/SeasAGI.exe`
- `SeasAGI-Client/src-app/build/bin/SeasAGI`
- `SeasAGI-Client/build/SeasAGI-<version>.dmg` (macOS only)

## Linux Dependencies

Ubuntu/Debian:
```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev build-essential
```

CentOS/RHEL:
```bash
sudo yum install gtk3-devel webkit2gtk3-devel
```