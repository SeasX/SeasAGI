> [English](README.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · **简体中文**

# SeasAGI Server Community

[![Build status](https://ci.appveyor.com/api/projects/status/github/SeasX/SeasAGI?svg=true)](https://ci.appveyor.com/project/SeasX/SeasAGI)
[![macOS](https://img.shields.io/badge/platform-macOS-blue)](https://github.com/SeasX/SeasAGI)
[![Linux](https://img.shields.io/badge/platform-Linux-blue)](https://github.com/SeasX/SeasAGI)
[![Windows](https://img.shields.io/badge/platform-Windows-blue)](https://github.com/SeasX/SeasAGI)
[![License](https://img.shields.io/badge/license-AGPL%20v3-green)](LICENSE)

社区版云端子项目，包含开源平台控制面、平台中继网关以及内嵌的轻量管理后台。不包含企业治理与计费能力。

## 目录

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
│   └── src/              # 管理后台前端（6 个页面）
├── platform-api/
│   └── cmd/admin_dist/   # 管理后台 SPA 内嵌进 platform-api 二进制
└── relay-gateway/
```

## 职责

- `platform-api`：社区版云端控制面，负责基础认证、通道管理、Provider 资源池、模型目录、基础 Combo CRUD、基础用量统计、SQLite 备份/恢复、租户管理员接口
- `relay-gateway`：社区版中继数据面，负责 relay、多模态转发、模型目录、健康检查、trace、限流、Prometheus 指标、运维接口
- `src-admin`：社区版管理后台前端（6 个页面），内嵌进 platform-api 二进制，通过 `/admin` 提供访问
- `deploy`：社区版部署、安装、systemd、nginx、运维脚本
- `scripts/build.sh`：社区版统一构建入口

## 社区版范围

### 包含能力

- **基础认证**：登录 / 注册 / Token 刷新（bcrypt + JWT HS256 双密钥）
- **通道管理**：平台通道 CRUD，API Key 使用 AES-256-GCM 加密存储
- **Provider 资源池**：通道级 API Key / 区域 / 环境资源，按健康 + 权重 + 优先级自动择优（`ResolveResource`）
- **模型目录**：`data/model-catalog.yaml` 解析入库，通过 `/models` 与 `/models/:name` 查询
- **基础 Combo**：用户级 Combo CRUD + 官方模板拉取
- **基础用量**：用户用量、按模型/通道分组、时间线、错误分布、近期错误
- **基础租户管理**：成员、邀请链接、自定义通道同步、策略、模板、配置快照
- **基础 Admin API**：用户 / 通道 / Combo / Relay Gateway 基础管理
- **SQLite 在线备份**：`VACUUM INTO` 备份、SHA-256 校验、恢复、30 天保留
- **免费通道种子**：`GET /free-channels` 返回 23 家免费 Provider（OpenCode、DuckDuckGo、DeepSeek、腾讯元宝、豆包、讯飞、Coze、AI Horde 等）
- **内嵌管理后台**：`/admin` SPA，含 6 个页面 — 仪表盘 / 用户 / 用量 / 中继网关 / 通道 / Combo
- **i18n**：zh-CN / en / ja / ko 错误消息（`?lang=` 或 `Accept-Language` 协商）
- **安全**：AES-256-GCM API Key 加密、启动密钥校验（`secpolicy`：开发模式自动补全临时密钥，生产模式缺失/弱密钥直接退出）。请求信封仅提供传输机密性（TLS 已覆盖同等目标），不是鉴权边界；设备绑定只是展示/审计线索，`device_id` 由客户端生成，不能用于席位或试用限制；授权、计价、额度、模型范围与结算周期一律由服务端裁决，客户端字段视为不可信输入。
- **Relay 基础转发**：请求透传、健康检查、限流、trace、多模态转发

### 不包含能力

以下内容已移至企业版项目 `SeasAGI-Server-Enterprise/`：

- 套餐驱动的商业功能门控
- 多租户账单 / 订单 / 支付 / 发票
- Combo 治理（可见性策略、部署审批）
- Combo 指标观测与 Provider 健康指标
- 企业 BYOK 双层回退策略
- 企业 SSO / SCIM / 合规 / 审计增强 / SLA / License
- 企业版管理后台页面（审计、风控、告警、Webhooks、套餐、充值、Token 交易/结算）

## 常用命令

```bash
# 编译社区版云端项目
bash SeasAGI-Server/scripts/build.sh

# 安装到 Linux 服务器
sudo bash SeasAGI-Server/deploy/install.sh
```

## 部署步骤（Linux + systemd）

**1. 生成密钥**

```bash
openssl rand -hex 32   # JWT_SECRET
openssl rand -hex 32   # JWT_REFRESH_SECRET
openssl rand -hex 32   # ADMIN_SECRET
openssl rand -hex 32   # SEASAGI_DATA_KEY（或 32 字节随机串）
```

`JWT_SECRET` 与 `JWT_REFRESH_SECRET` 在两个 env 文件中必须完全一致。`ADMIN_SECRET` 必须显式设置，否则 `/api/v1/admin/*` 不可用。

**2. 写 `/etc/seasagi/*.env`**

```bash
sudo vi /etc/seasagi/platform-api.env
# JWT_SECRET、JWT_REFRESH_SECRET、ADMIN_SECRET、SEASAGI_DATA_KEY
# CORS_ALLOWED_ORIGINS=https://your-domain.example
# MAX_REQUEST_BODY_BYTES=8388608
# TRACE_RETENTION_DAYS=30
# TRACE_PURGE_INTERVAL_HOURS=6

sudo vi /etc/seasagi/relay-gateway.env
# 相同的 JWT_SECRET / JWT_REFRESH_SECRET / ADMIN_SECRET
# RELAY_MAX_REQUEST_BODY_BYTES=67108864
```

**3. 启动两个 service**

```bash
sudo systemctl enable --now seasagi-platform-api
sudo systemctl enable --now seasagi-relay-gateway
```

**4. 自检**

```bash
curl -fsS http://127.0.0.1:9318/healthz
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9318/api/v1/admin/users   # 401
curl -s -D - -o /dev/null -X OPTIONS \
  -H 'Origin: https://evil.example' \
  -H 'Access-Control-Request-Method: POST' \
  http://127.0.0.1:9318/api/v1/auth/login | grep -i '^access-control-allow-origin'  # 空
```

**5. 升级与备份**

```bash
# 覆盖二进制前先备份 SQLite
sudo systemctl stop seasagi-platform-api seasagi-relay-gateway
sudo mkdir -p /var/backups/seasagi
sudo sqlite3 /var/lib/seasagi/platform-api.db ".backup '/var/backups/seasagi/platform-api-$(date +%F).db'"
sudo sqlite3 /var/lib/seasagi/platform-api.db 'PRAGMA wal_checkpoint(TRUNCATE);'
# 覆盖二进制后重新启动服务
sudo systemctl start seasagi-platform-api seasagi-relay-gateway
```

数据库 schema 迁移在启动时自动执行且幂等。手工拷贝数据库时需连同 `-wal` / `-shm` 边车文件一起拷贝；在线备份接口（`VACUUM INTO`）本身产生的就是一致性快照。

## 构建产物

- `SeasAGI-Server/build/platform-api`（内嵌管理后台 SPA）
- `SeasAGI-Server/build/relay-gateway`

## 配置说明

所有配置通过环境变量加载，参考 `deploy/.env.example` 创建 `.env` 文件并导出：

```bash
export $(grep -v '^#' deploy/.env.example | xargs)
```

生产模式（`GIN_MODE=release`）下，两个服务在 `JWT_SECRET`、`JWT_REFRESH_SECRET`、`ADMIN_SECRET`、`SEASAGI_DATA_KEY` 缺失或仍为 `change-me*` 占位值时拒绝启动。开发模式（`GIN_MODE=debug` 或显式 `SEASAGI_ALLOW_DEV_SECRETS=1`）则自动生成临时密钥。所有 `/api/v1/admin/*` 接口（含内嵌 `/admin` SPA）均需 `Authorization: Bearer ${ADMIN_SECRET}`；缺失或错误凭证返回 401。

### 平台 API 配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `API_PORT` | 平台 API 监听端口 | `9318` |
| `DB_PATH` | SQLite 数据库路径 | `~/.seasagi/platform-api.db` |
| `JWT_SECRET` | JWT 签名密钥（生产环境必改） | — |
| `JWT_REFRESH_SECRET` | JWT 刷新令牌密钥（生产环境必改） | — |
| `ADMIN_SECRET` | Admin API 密钥 | — |
| `CORS_ALLOWED_ORIGINS` | 跨域允许来源（逗号分隔、精确匹配；生产环境拒绝 `*`） | 仅本机（`localhost` / `127.0.0.1` / `[::1]`） |
| `MAX_REQUEST_BODY_BYTES` | 单请求正文大小上限（字节） | `8388608`（8MB） |
| `SEASAGI_DATA_KEY` | API Key 的 AES-256-GCM 加密密钥 | — |
| `GIN_MODE` | Gin 运行模式（`release` / `debug`） | — |
| `SEASAGI_LOCALES_DIR` | i18n 语言文件目录 | `locales` |
| `BACKUP_DIR` | SQLite 备份目录 | `<db 目录>/backups` |
| `SEASAGI_VERSION` | 部署版本标记 | `1.0.0` |
| `API_BASE_URL` | 服务端基础地址（OAuth 跳转回退地址） | `http://localhost:9318` |
| `PUBLIC_BASE_URL` | 浏览器可达的服务端公网地址（OAuth 授权与回调） | 回退到 `API_BASE_URL` |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | Google OAuth 凭证 | 空（不显示 Google 登录） |
| `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET` | GitHub OAuth 凭证 | 空（不显示 GitHub 登录） |
| `WEBHOOK_DISPATCHER_INTERVAL_SEC` | Webhook 派发轮询间隔（秒） | `5` |
| `WEBHOOK_MAX_RETRIES` | Webhook 最大重试次数 | `3` |

### 中继网关配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `RELAY_PORT` | 中继网关端口 | `8318` |
| `RATE_LIMIT_RPM` | 全局限流速率（次/分） | `60` |
| `HEALTH_CHECK_INTERVAL_SEC` | 通道健康检查间隔 | `60` |
| `POLICY_CACHE_SEC` | 策略缓存刷新间隔 | `60` |
| `PLATFORM_API_URL` | 平台 API 地址（通道快照同步） | `http://127.0.0.1:9318` |
| `METRICS_TOKEN` | `/metrics` Bearer 令牌（回退到 `ADMIN_SECRET`） | — |
| `CORS_ALLOWED_ORIGINS` | 跨域允许来源（逗号分隔、精确匹配；生产环境拒绝 `*`） | 仅本机（`localhost` / `127.0.0.1` / `[::1]`） |
| `RELAY_MAX_REQUEST_BODY_BYTES` | 单请求正文大小上限（字节，中继网关） | `67108864`（64MB） |
| `TRACE_RETENTION_DAYS` | Trace 保留天数（`<=0` 表示禁用清理；platform-api 与 relay-gateway 共用） | `30` |
| `TRACE_PURGE_INTERVAL_HOURS` | Trace 清理间隔小时数（`<1` 按 `1` 处理） | `6` |

## API 端点

### 平台 API（`/api/v1`）

- `auth`：`POST /auth/login` / `POST /auth/register` / `POST /auth/refresh`
- `user`：`GET /user/profile` · `GET/PUT /user/optimization`（路由/健康检查/冷却/sticky/preset 开关）
- `channel`：`GET /channels` · `GET /channels/:id` · `GET /free-channels`（23 家免费 Provider）——用户侧只读，响应永不返回凭证
- `providerresource`：`GET/POST/PUT/DELETE /admin/channels/:id/resources[/:resource_id]` · `GET .../resources/resolve`（仅管理端）
- `modelcatalog`：`GET /models` · `GET /models/:name`
- `device`：`POST /devices/bind` · `GET /devices` · `DELETE /devices/:id`
- `combo`：`GET/POST /combos` · `GET/PUT/DELETE /combos/:id` · `GET /combo-templates`
- `relay`：`GET /relay-gateways`
- `usage`：`GET /usage` · `/usage/models` · `/usage/error-distribution` · `/usage/timeline`
- `version`：`GET /version/check` · `GET /version/migrations`
- `admin`：`GET/DELETE /admin/users` · `GET/POST/PUT/DELETE /admin/channels` · `/admin/channels/:id/resources` · `GET /admin/usage/stats` · `GET /admin/usage/records` · `GET /admin/relay-gateways` · `GET/POST/DELETE /admin/sqlite/backups[/:id]` · `POST /admin/sqlite/backups/:id/restore` · `POST /admin/sqlite/backups/:id/verify` · `POST /admin/channels/seed` · `POST /admin/models/sync`
- 系统：`GET /healthz` · `/admin/*`（内嵌 SPA）

### 中继网关

- 公开：`GET /healthz` · `GET /metrics`（Prometheus，需令牌） · `GET /ops/channels`
- 运维（`/ops`）：`GET /ops/overview` · `GET /ops/channels/health` · `GET /ops/traces[/:trace_id]` · `POST /ops/channels/:id/drain` · `POST /ops/channels/:id/restore` · `PUT /ops/channels/:id/read-only` · `PUT /ops/channels/:id/weight` · `PUT /ops/channels/:id/gray` · `GET /ops/alerts`
- 中继（`/relay`，JWT 保护 + 限流）：`POST /relay/chat/completions` · `POST /relay/embeddings` · `POST /relay/images/generations` · `POST /relay/audio/speech` · `POST /relay/audio/transcriptions` · `GET /relay/models`

## 许可

社区版使用 `AGPL 3.0`。完整企业功能请见 `SeasAGI-Server-Enterprise/`。
