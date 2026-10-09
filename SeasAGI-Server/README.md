> **English** · [日本語](README.ja.md) · [한국어](README.ko.md) · [简体中文](README.zh.md)

# SeasAGI Server Community

[![Build status](https://ci.appveyor.com/api/projects/status/github/SeasX/SeasAGI?svg=true)](https://ci.appveyor.com/project/SeasX/SeasAGI)
[![macOS](https://img.shields.io/badge/platform-macOS-blue)](https://github.com/SeasX/SeasAGI)
[![Linux](https://img.shields.io/badge/platform-Linux-blue)](https://github.com/SeasX/SeasAGI)
[![Windows](https://img.shields.io/badge/platform-Windows-blue)](https://github.com/SeasX/SeasAGI)
[![License](https://img.shields.io/badge/license-AGPL%20v3-green)](LICENSE)

Community edition cloud sub-project, containing the open-source platform control plane, platform relay gateway, and an embedded lightweight admin dashboard. Enterprise governance and billing are not included.

## Directory Structure

```text
SeasAGI-Server/
├── README.md
├── scripts/
│   └── build.sh
├── deploy/
│   ├── build.sh
│   ├── install.sh
│   ├── nginx.conf
│   ├── .env.example
│   └── systemd/
├── data/
│   └── model-catalog.yaml
├── locales/
│   ├── en.json
│   ├── ja.json
│   ├── ko.json
│   └── zh-CN.json
├── src-admin/
│   └── src/              # Admin dashboard frontend (6 pages)
├── platform-api/
│   └── cmd/admin_dist/   # Admin SPA embedded into platform-api binary
└── relay-gateway/
```

## Responsibilities

- `platform-api`: Community cloud control plane — basic authentication, channel management, provider resource pools, model catalog, basic Combo CRUD, basic usage statistics, SQLite backup/restore, tenant admin APIs
- `relay-gateway`: Community relay data plane — relay, multimodal forwarding, model catalog, health checks, tracing, rate limiting, Prometheus metrics, O&M interfaces
- `src-admin`: Community admin dashboard frontend (6 pages), embedded into the platform-api binary and served at `/admin`
- `deploy`: Community deployment, installation, systemd, nginx, O&M scripts
- `scripts/build.sh`: Community unified build entry point

## Community Edition Scope

### Included Capabilities

- **Basic authentication**: Login / Register / Token refresh (bcrypt + JWT HS256 dual keys)
- **Channel management**: Platform channel CRUD, API Keys encrypted with AES-256-GCM
- **Provider resource pools**: Per-channel API Key / region / environment resources, health + weight + priority based optimal selection (`ResolveResource`)
- **Model catalog**: `data/model-catalog.yaml` parsed into the `model_catalog` table, queryable via `/models` and `/models/:name`
- **Basic Combo**: User-level Combo CRUD + official template pull
- **Basic usage**: User usage, grouped by model/channel, timeline, error distribution, recent errors
- **Basic tenant management**: Members, invite links, custom channel sync, policies, templates, configuration snapshots
- **Basic Admin API**: User / Channel / Combo / Relay Gateway basic management
- **SQLite online backup**: `VACUUM INTO` backup, SHA-256 verify, restore, 30-day retention
- **Free channels seed**: `GET /free-channels` returns 23 free providers (OpenCode, DuckDuckGo, DeepSeek, Tencent Yuanbao, Doubao, iFlytek, Coze, AI Horde, etc.)
- **Embedded admin dashboard**: `/admin` SPA with 6 pages — Dashboard / Users / Usage / Relay Gateways / Channels / Combos
- **i18n**: zh-CN / en / ja / ko error messages (`?lang=` or `Accept-Language` negotiation)
- **Security**: AES-256-GCM API Key encryption, startup weak-key detection (`secpolicy`, auto-generates random JWT keys, production mode fatals on weak keys)
- **Relay basic forwarding**: Request passthrough, health checks, rate limiting, tracing, multimodal forwarding

### Excluded Capabilities

The following features have been moved to the enterprise project `SeasAGI-Server-Enterprise/`:

- Plan-driven commercial feature gating
- Multi-tenant billing / orders / payments / invoices
- Combo governance (visibility policies, deployment approval)
- Combo metric observability & Provider health indicators
- Enterprise BYOK dual-layer fallback strategy
- Enterprise SSO / SCIM / compliance / audit enhancements / SLA / License
- Enterprise admin dashboard pages (Audit, Risk, Alerts, Webhooks, Plans, Recharge, Token market trading/settlement)

## Common Commands

```bash
# Build community cloud project
bash SeasAGI-Server/scripts/build.sh

# Install to Linux server
sudo bash SeasAGI-Server/deploy/install.sh
```

## Build Artifacts

- `SeasAGI-Server/build/platform-api` (admin SPA embedded)
- `SeasAGI-Server/build/relay-gateway`

## Configuration

All configuration is loaded via environment variables. Refer to `deploy/.env.example` to create a `.env` file and export it:

```bash
export $(grep -v '^#' deploy/.env.example | xargs)
```

### Platform API Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `API_PORT` | Platform API listen port | `9318` |
| `DB_PATH` | SQLite database path | `~/.seasagi/platform-api.db` |
| `JWT_SECRET` | JWT signing secret (required in production) | — |
| `JWT_REFRESH_SECRET` | JWT refresh token secret (required in production) | — |
| `ADMIN_SECRET` | Admin API secret | — |
| `CORS_ALLOW_ORIGIN` | CORS allowed origin | `*` |
| `SEASAGI_DATA_KEY` | AES-256-GCM encryption key for API Keys | — |
| `GIN_MODE` | Gin mode (`release` / `debug`) | — |
| `SEASAGI_LOCALES_DIR` | i18n language file directory | `locales` |
| `BACKUP_DIR` | SQLite backup directory | `<db dir>/backups` |

### Relay Gateway Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `RELAY_PORT` | Relay gateway port | `8318` |
| `RATE_LIMIT_RPM` | Global rate limit (requests/min) | `60` |
| `HEALTH_CHECK_INTERVAL_SEC` | Channel health check interval | `60` |
| `POLICY_CACHE_SEC` | Policy cache refresh interval | `60` |
| `PLATFORM_API_URL` | Platform API base URL for channel snapshot sync | `http://127.0.0.1:9318` |
| `METRICS_TOKEN` | `/metrics` bearer token (falls back to `ADMIN_SECRET`) | — |
| `CORS_ALLOW_ORIGIN` | CORS allowed origin | `*` |

## API Endpoints

### Platform API (`/api/v1`)

- `auth`: `POST /auth/login` / `POST /auth/register` / `POST /auth/refresh`
- `user`: `GET /user/profile` · `GET/PUT /user/optimization` (routing/healthcheck/cooldown/sticky/preset toggles)
- `channel`: `GET/POST/PUT/DELETE /channels` · `GET /free-channels` (23 free providers)
- `providerresource`: `GET/POST/PUT/DELETE /channels/:id/resources[/:resource_id]` · `GET .../resources/resolve`
- `modelcatalog`: `GET /models` · `GET /models/:name`
- `device`: `POST /devices/bind` · `GET /devices` · `DELETE /devices/:id`
- `combo`: `GET/POST /combos` · `GET/PUT/DELETE /combos/:id` · `GET /combo-templates`
- `relay`: `GET /relay-gateways`
- `usage`: `GET /usage` · `/usage/models` · `/usage/error-distribution` · `/usage/timeline`
- `version`: `GET /version/check` · `GET /version/migrations`
- `admin`: `GET/DELETE /admin/users` · `GET /admin/usage/stats` · `GET /admin/usage/records` · `GET /admin/relay-gateways` · `GET/POST/DELETE /admin/sqlite/backups[/:id]` · `POST /admin/sqlite/backups/:id/restore` · `POST /admin/sqlite/backups/:id/verify` · `POST /admin/channels/seed` · `POST /admin/models/sync`
- System: `GET /healthz` · `/admin/*` (embedded SPA)

### Relay Gateway

- Public: `GET /healthz` · `GET /metrics` (Prometheus, token protected) · `GET /ops/channels`
- Ops (`/ops`): `GET /ops/overview` · `GET /ops/channels/health` · `GET /ops/traces[/:trace_id]` · `POST /ops/channels/:id/drain` · `POST /ops/channels/:id/restore` · `PUT /ops/channels/:id/read-only` · `PUT /ops/channels/:id/weight` · `PUT /ops/channels/:id/gray` · `GET /ops/alerts`
- Relay (`/relay`, JWT protected + rate limited): `POST /relay/chat/completions` · `POST /relay/embeddings` · `POST /relay/images/generations` · `POST /relay/audio/speech` · `POST /relay/audio/transcriptions` · `GET /relay/models`

## License

The community edition is licensed under `AGPL 3.0`. For full enterprise features, see `SeasAGI-Server-Enterprise/`.
