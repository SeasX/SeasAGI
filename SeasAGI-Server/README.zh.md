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
│   └── src/              # 管理后台前端（7 个页面）
├── platform-api/
│   └── cmd/admin_dist/   # 管理后台 SPA 内嵌进 platform-api 二进制
└── relay-gateway/
```

## 职责

- `platform-api`：社区版云端控制面，负责基础认证、通道管理、Provider 资源池、模型目录、基础 Combo CRUD、基础用量统计、SQLite 备份/恢复、租户管理员接口
- `relay-gateway`：社区版中继数据面，负责 relay、多模态转发、模型目录、健康检查、trace、限流、Prometheus 指标、运维接口
- `src-admin`：社区版管理后台前端（7 个页面），内嵌进 platform-api 二进制，通过 `/admin` 提供访问
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
- **基础 Admin API**：用户 / 套餐 / 通道 / Combo / Relay Gateway 基础管理
- **SQLite 在线备份**：`VACUUM INTO` 备份、SHA-256 校验、恢复、30 天保留
- **免费通道种子**：`GET /free-channels` 返回 23 家免费 Provider（OpenCode、DuckDuckGo、DeepSeek、腾讯元宝、豆包、讯飞、Coze、AI Horde 等）
- **内嵌管理后台**：`/admin` SPA，含 7 个页面 — 仪表盘 / 用户 / 用量 / 中继网关 / 通道 / Combo / Token 市场
- **i18n**：zh-CN / en / ja / ko 错误消息（`?lang=` 或 `Accept-Language` 协商）
- **安全**：AES-256-GCM API Key 加密、启动弱密钥检测（`secpolicy` 自动生成随机 JWT 密钥，生产模式弱密钥直接 Fatal）
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

## 构建产物

- `SeasAGI-Server/build/platform-api`（内嵌管理后台 SPA）
- `SeasAGI-Server/build/relay-gateway`

## 配置说明

所有配置通过环境变量加载，参考 `deploy/.env.example` 创建 `.env` 文件并导出：

```bash
export $(grep -v '^#' deploy/.env.example | xargs)
```

### 平台 API 配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `API_PORT` | 平台 API 监听端口 | `9318` |
| `DB_PATH` | SQLite 数据库路径 | `~/.seasagi/platform-api.db` |
| `JWT_SECRET` | JWT 签名密钥（生产环境必改） | — |
| `JWT_REFRESH_SECRET` | JWT 刷新令牌密钥（生产环境必改） | — |
| `ADMIN_SECRET` | Admin API 密钥 | — |
| `CORS_ALLOW_ORIGIN` | 跨域允许来源 | `*` |
| `SEASAGI_DATA_KEY` | API Key 的 AES-256-GCM 加密密钥 | — |
| `GIN_MODE` | Gin 运行模式（`release` / `debug`） | — |
| `SEASAGI_LOCALES_DIR` | i18n 语言文件目录 | `locales` |
| `BACKUP_DIR` | SQLite 备份目录 | `<db 目录>/backups` |

### 中继网关配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `RELAY_PORT` | 中继网关端口 | `8318` |
| `RATE_LIMIT_RPM` | 全局限流速率（次/分） | `60` |
| `HEALTH_CHECK_INTERVAL_SEC` | 通道健康检查间隔 | `60` |
| `POLICY_CACHE_SEC` | 策略缓存刷新间隔 | `60` |
| `PLATFORM_API_URL` | 平台 API 地址（通道快照同步） | `http://127.0.0.1:9318` |
| `METRICS_TOKEN` | `/metrics` Bearer 令牌（回退到 `ADMIN_SECRET`） | — |
| `CORS_ALLOW_ORIGIN` | 跨域允许来源 | `*` |

## API 端点

### 平台 API（`/api/v1`）

- `auth`：`POST /auth/login` / `POST /auth/register` / `POST /auth/refresh`
- `user`：`GET/PUT /user/profile` · `GET/PUT /user/optimization`（路由/健康检查/冷却/sticky/preset 开关）
- `channel`：`GET/POST/PUT/DELETE /channels` · `GET /free-channels`（23 家免费 Provider）
- `providerresource`：`GET/POST/PUT/DELETE /channels/:id/resources[/:resource_id]` · `GET .../resources/resolve`
- `modelcatalog`：`GET /models` · `GET /models/:name`
- `device`：`POST /devices/bind` · `GET /devices` · `DELETE /devices/:id`
- `combo`：`GET/POST /combos` · `GET/PUT/DELETE /combos/:id` · `GET /combo-templates`
- `relay`：`GET /relay-gateways`
- `usage`：`GET /usage` · `/usage/models` · `/usage/error-distribution` · `/usage/timeline`
- `version`：`GET /version/check` · `GET /version/migrations`
- `admin`：`GET/DELETE /admin/users` · `GET /admin/usage/stats` · `GET /admin/usage/records` · `GET /admin/relay-gateways` · `GET/POST/DELETE /admin/sqlite/backups[/:id]` · `POST /admin/sqlite/backups/:id/restore` · `POST /admin/sqlite/backups/:id/verify` · `POST /admin/channels/seed` · `POST /admin/models/sync`
- 系统：`GET /healthz` · `/admin/*`（内嵌 SPA）

### 中继网关

- 公开：`GET /healthz` · `GET /metrics`（Prometheus，需令牌） · `GET /ops/channels`
- 运维（`/ops`）：`GET /ops/overview` · `GET /ops/channels/health` · `GET /ops/traces[/:trace_id]` · `POST /ops/channels/:id/drain` · `POST /ops/channels/:id/restore` · `PUT /ops/channels/:id/read-only` · `PUT /ops/channels/:id/weight` · `PUT /ops/channels/:id/gray` · `GET /ops/alerts`
- 中继（`/relay`，JWT 保护 + 限流）：`POST /relay/chat/completions` · `POST /relay/embeddings` · `POST /relay/images/generations` · `POST /relay/audio/speech` · `POST /relay/audio/transcriptions` · `GET /relay/models`

## 许可

社区版使用 `AGPL 3.0`。完整企业功能请见 `SeasAGI-Server-Enterprise/`。
