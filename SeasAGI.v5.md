[🇬🇧 English](SeasAGI.v5.md) | [🇨🇳 中文](SeasAGI.v5.zh-CN.md) | [🇯🇵 日本語](SeasAGI.v5.ja.md) | [🇰🇷 한국어](SeasAGI.v5.ko.md)

---

# SeasAGI v5 One-Page Summary

## 1. One-Liner

SeasAGI is a local unified model gateway for AI power users and developers, letting users manage LLM API exits the same way they manage network exits.

SeasAGI is not just a "key-switching tool" — it builds a "local unified model gateway" entry point on the AI infrastructure consumption side.

Its value lies in:

- Reducing multi-model integration complexity from "repeated configuration per client" to "one-time local configuration"
- Transforming user anxiety about Key security into perceivable product trust
- Transforming runtime Provider instability into adaptive routing degradation, invisible to users
- Transforming usage black boxes into full-dimension insights (model/channel/timeline/cost/errors)
- Building future higher-value strategic routing, team edition, and subscription services on an already operational local gateway foundation

## 2. User Pain Points

Current AI clients and IDE plugins have several high-frequency pain points:

- Different model platforms require repeated configuration of `Base URL`, `API Key`, model name
- Platform channels and user-purchased Keys are mixed, high switching cost
- No unified fallback or diagnostics when upstream services are unstable
- Users普遍 worry their Keys will be collected or stored in plaintext by platforms
- Mid-conversation model switching causes context breakage and hallucinations
- Provider frequent 429 rate limiting, no adaptive routing priority adjustment
- No intuitive insight into which model/channel has highest usage, optimal cost

## 3. Our Solution

SeasAGI provides a unified OpenAI-compatible entry point locally, so all third-party clients only need to configure the local address once to access multiple model exits.

Core capabilities:

- Local unified entry: default `http://127.0.0.1:4318/v1`, dynamically changes with listen port
- Dual-channel architecture: platform channels + custom channels
- User-provided Keys are never uploaded, stored only in local Keychain
- Routing strategies: `fallback`, `round_robin`, `sticky`
- Model-level Combo: explicit `channel + model` step chain + drag-and-drop sorting + primary/backup/last-resort role semantics
- Navigation fusion: left nav consolidated to "Combo Optimization" single entry, workbench with 4 tabs (My Plans / Optimization Suggestions / Template Center / Execution Analysis)
- Playground debugging: Combo selector + execution chain visualization + step fallback status display (step_role badge)
- Default entry supports Combo: HomePage default Combo selector + Wails persistence
- Combo cloud sync: three-layer view (local/official/team) + Fetch/Push/Update/Delete full sync capability
- Combo metrics: request count / fallback count / Step1 hit rate / last-step hit rate
- Smart optimization & Combo collaboration: multi-step fallback suggestions, structural diff preview, one-click apply as Combo
- Per-step Provider/Channel candidate pool: each Step supports multiple candidates + auto-ranking
- Request-level execution constraints: max_price / max_latency_ms / data_policy
- Tool-call dedicated routing strategy: prioritize providers with high tool success rate
- Quick-strategy aliases: stability-first / cost-first / speed-first / tool-first
- Task-type awareness: chat / tools / json / long_context / batch_low_cost differentiated execution
- Enterprise BYOK / platform shared capacity dual-layer fallback
- Session sticky routing: SHA1 hash based on first user message, 30min TTL, prevents mid-conversation model switching
- Dynamic 429 penalty degradation: PenaltyManager, 429 +3, success -1, 2min natural decay
- Fallback sort presets: intelligence / speed / budget one-click reorder
- 8 Provider executors: OpenAI, Azure, Anthropic, Gemini, Ollama, DeepSeek, Grok, platform relay
- 6 format translators: OpenAI Chat/Responses, Anthropic, Gemini, Vertex AI, Passthrough
- Key health check: 5min periodic probe, 3 consecutive failures auto-mark unhealthy
- Key-level Cooldown: per-key 120s TTL, precise cooling on 429, doesn't affect other keys
- Multi-modal content flattening: text-only array content auto-merged to string
- Playground instant test: chat UI + model selection + local gateway communication
- Constant-time Key comparison: crypto/subtle.ConstantTimeCompare, eliminates timing side-channel
- Zod frontend validation: 8 schemas + form integration
- Logging & diagnostics: link visualization, CSV export, structured error copy
- RTK Token compression: 9 output types auto-detection + compression
- Full-modal API: Embeddings / Image / TTS / STT
- Tunnel remote access: Cloudflare Tunnel + Tailscale Funnel
- Multi-account fault tolerance: multi-Key rotation + exponential backoff + circuit breaker + 7 error rules
- OAuth ecosystem: PKCE + 4 Provider registry + local loopback callback + Token auto-refresh
- Local access token: left nav independent page, view / copy / reset
- Cloud capabilities: register / login, platform channel sync, usage / billing query
- Cloud analytics: per-model/channel grouping + cost estimation (GPT-4o pricing) + recent errors + request timeline + error classification distribution
- Admin visualization: Recharts BarChart / LineChart / PieChart, 6-Tab UsagePage
- Caveman concise output: 4 style injections
- Reasoning Content: DeepSeek-R1/Kimi/QwQ reasoning chain injection
- MCP tool dedup: 12 equivalence mappings
- Internationalization: Chinese, English, Japanese, Korean, follows system language

## 4. Why Now

- AI users simultaneously using multiple clients and multiple models has become the norm
- Local-first, security-first, unified-entry demand is rapidly rising
- "Cloud aggregation platforms" exist, but mature products with "local gateway + strong security boundary + adaptive routing + full-chain observability" remain scarce
- SeasAGI can first penetrate the individual developer market, then expand to team edition and cross-platform edition

## 5. Product Positioning

- Target users:
  - AI power users
  - Individual developers
  - Independent developers
  - Small AI team tech leads
- Current form:
  - Wails desktop client (primary validation platform macOS, code structure covers Windows / Linux)
  - Local OpenAI-compatible API gateway
- Core scenarios:
  - Unified access to multiple model platforms
  - Platform channel and self-purchased Key switching
  - Channel failure fallback and adaptive degradation
  - Request failure link diagnostics
  - Multi-turn conversation session stickiness
  - Usage insight and cost optimization

## 6. Differentiation Barriers

### 6.1 Clear Security Boundary

- User-provided Keys are never uploaded to platform
- Keys stored in macOS Keychain
- Local access tokens stored in local SQLite, never enter cloud
- Token verification uses constant-time comparison, eliminates timing side-channel attacks
- Platform channel vs user channel boundary clearly displayed in UI

### 6.2 Adaptive Intelligent Routing

- Not a simple "default channel switcher"
- Session stickiness: multi-turn conversations auto-route to same model, prevents hallucinations
- Dynamic 429 penalty: Provider rate-limiting auto-sinks priority, auto-recovers on success
- Sort presets: intelligence / speed / budget one-click reorder
- Key health check: auto-probe, failed Keys auto-disabled
- per-key Cooldown: precise control, doesn't affect other keys
- Multi-modal content flattening: compatible with more client formats
- Per-step Provider/Channel candidate pool + auto-ranking: dynamically select best within same step by stability/cost/latency
- Tool-call independent routing strategy: sort by tool success rate, not just price priority
- Request-level dynamic constraints: each call can attach max_price / max_latency_ms / data_policy

### 6.3 Model-level Combo Orchestration & Governance

- Failure fallback orchestration: not a static model list, but "primary → backup → last-resort" explicit fallback chain
- Navigation fusion: left nav single entry "Combo Optimization", workbench unified flow: configure→suggest→apply→verify
- Smart optimization & Combo collaboration: multi-step fallback suggestions + structural diff preview + one-click apply
- Cloud sync: three-layer view (local/official/team) + full sync (Fetch/Push/Update/Delete)
- Execution analysis: Combo hit rate / fallback rate / average attempt count dashboard
- Server control plane: three-level Combo management + version publish/rollback + differentiated rollout
- Enterprise governance: visibility policy (role/plan/user-group/tenant four levels) + deployment approval flow
- Combo runtime instrumentation: request count / fallback count / Step1 hit rate / last-step hit rate
- Quick-strategy aliases: stability-first / cost-first / speed-first / tool-first
- Task-type awareness: differentiated execution by chat/tools/json/long_context

### 6.4 Full-chain Observability

- Logs include hit chain and per-step failure reasons
- Error classification distribution (429/401/403/404/timeout/500/503)
- Request timeline chart (24h/7d/30d)
- Per-model/channel grouped analysis
- Recent errors quick view
- Cost estimation (GPT-4o pricing baseline)
- Admin chart visualization (BarChart / LineChart / PieChart)

### 6.5 Local Gateway Experience

- All clients only need to configure local address once
- Users don't need to repeatedly change configuration in Chatbox, Cherry Studio, Cursor, etc.
- Playground instant test, verify immediately after configuring channels

### 6.6 Extensible Architecture

- Externally unified OpenAI-compatible
- Internally via protocol normalization + Provider executor to extend more upstream
- Executor registry supports dynamic registration of new Providers
- OAuth PKCE flow supports any OAuth 2.0 Provider
- Smoothly extensible to team edition, policy rollout, config sync

## 7. Three Sub-project Organization

- `SeasAGI-Client/`: Client sub-project, retains `src-app` original structure internally, carries Wails desktop client mainline source and client build scripts
- `SeasAGI-Server/`: Server sub-project, retains `platform-api`, `relay-gateway`, `src-admin`, `deploy` original structure internally, carries control plane, data plane, admin dashboard and deployment scripts
- `web-docs/`: Website & docs sub-project, retains `src-web`, mainline architecture/PRD/schedule docs, and `cc-switch`, `9router` etc. historical or reference directories
- Root directory: only retains top `README.md`, `SeasAGI.v5.md` and one-click build script `build-all.sh`

## 8. Current Progress

Current `v5` completed key capabilities:

- Wails + Go client running
- Local gateway supports `/v1/models`, `/v1/chat/completions` + full-modal API
- Platform channel and custom channel dual-channel management
- Routing strategies `fallback` / `round_robin` / `sticky` + Combo step chain
- Session sticky routing (SHA1 + session→step + 30min TTL)
- Dynamic 429 penalty degradation (PenaltyManager + time decay + Circuit Breaker collaboration)
- Fallback sort presets (intelligence / speed / budget)
- Key health check (5min probe + 3 consecutive failures auto-disable)
- Key-level Cooldown (per-key 120s TTL + auto-cleanup)
- Constant-time Key comparison (crypto/subtle)
- Multi-modal content flattening
- Playground instant test page
- 8 Provider executors + 6 format translators
- Log filtering, sorting, CSV export, structured error copy
- RTK Token compression + tunnel remote access
- Multi-account fault tolerance + OAuth 2.0 PKCE
- Caveman + Reasoning + MCP tool dedup
- Zod frontend validation + Combo drag-and-drop sorting
- Cloud analytics API (per-model/channel grouping + cost estimation + recent errors + timeline + error classification distribution)
- Admin visualization (Recharts BarChart / LineChart / PieChart)
- Internationalization (Chinese/English/Japanese/Korean)
