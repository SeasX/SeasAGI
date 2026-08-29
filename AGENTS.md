# SeasAGI Agent Guide

## Repository overview

SeasAGI is a layered AI gateway ecosystem with a Wails desktop client, a community server, and an enterprise server. This repository contains the desktop client and the community server.

- `SeasAGI-Client/`: Wails v2 desktop application (Go + React). Local AI gateway with 8 Executors, 6 Translators, Combo step-chain routing, Keychain, and OAuth.
  - `src-app/`: Go backend (`cmd/`, `internal/`) + React frontend (`frontend/`)
  - Module path: `github.com/SeasAGI/SeasAGI-Client`
- `SeasAGI-Server/`: Community edition server (Go + Gin). Platform API + Relay Gateway.
  - `platform-api/`: Control plane (auth, channels, combos, usage, model catalog, provider resources, SQLite backups). Module: `github.com/SeasAGI/SeasAGI-Server/platform-api`
  - `relay-gateway/`: Data plane (request forwarding, health checks, rate limiting, tracing). Module: `github.com/SeasAGI/SeasAGI-Server/relay-gateway`
  - `data/model-catalog.yaml`: Tracked model catalog source with pricing, capabilities, and context window metadata.
  - `deploy/`: systemd services, nginx config, backup scripts, `.env.example`.
- `SeasAGI.v5.md`, `MITM代理.5.29.md`: Architecture and design documents.

## Development commands

### Client (from `SeasAGI-Client/src-app/`)

```bash
go build ./...
go test ./...
go vet ./...
```

Frontend (from `SeasAGI-Client/src-app/frontend/`):

```bash
npm ci
npm run build
```

### Server (from `SeasAGI-Server/platform-api/` or `SeasAGI-Server/relay-gateway/`)

```bash
gofmt -w <changed-go-files>
go build ./...
go test ./...
go vet ./...
```

## Change guidelines

- Keep changes focused and preserve unrelated work in the checkout.
- Add or update tests for backend behavior changes. Prefer in-process fakes over external network dependencies.
- Preserve OpenAI-compatible `/v1` and `/relay` API contracts unless the task explicitly changes them.
- Treat authentication, JWT secrets, API keys, encrypted channel credentials, and audit payloads as security-sensitive.
- Never commit real credentials, local `.env` files, SQLite databases, generated backups, or runtime logs.
- Keep environment variable additions synchronized across `deploy/.env.example` and deployment documentation.
- Keep `data/model-catalog.yaml` tracked; other files under runtime data directories are intentionally ignored.
- Database migrations are versioned SQL in `internal/database/sqlite.go`. Each migration has a `version:` field. Never modify an applied migration — always add a new one.
- Module paths use `github.com/SeasAGI/SeasAGI-Server/*` (community) and `github.com/SeasAGI/SeasAGI-Server-Enterprise/*` (enterprise). Keep imports consistent.

## Pull request guidelines

- Use an English Conventional Commits-style PR title in the format `<type>[optional scope][!]: <short summary>`, limited to 72 characters. Common types: `feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `style`, `revert`. Use a lowercase imperative summary without a trailing period.
- Before creating a pull request, read and complete `.github/pull_request_template.md`.
- Preserve every template section, replace all placeholders, and explain any skipped or non-applicable checks.
- Do not use `gh pr create --fill` or an ad hoc body that bypasses the template.
- Create a ready-for-review pull request by default. Use a draft only when explicitly requested.

## Validation expectations

- Run the narrowest relevant test while iterating, then run the full applicable check set before handing off.
- Run `git diff --check` before committing.
- Report any check that could not run and distinguish new failures from failures already present on the base branch.
- For deployment changes, validate systemd unit files and nginx config syntax when applicable.
