[🇬🇧 English](README.md) | [🇨🇳 中文](README.zh-CN.md) | [🇯🇵 日本語](README.ja.md) | [🇰🇷 한국어](README.ko.md)

---

# SeasAGI

**本地 LLM API 智能网关** — 开源客户端 + 服务端开源核心模式，统一管理多个 LLM Provider 渠道，提供智能路由、会话粘性、成本优化、全链路可观测性及团队协作治理。

---

## 概览

SeasAGI 是一个分层的 AI 网关平台，由三个子项目组成：

| 子项目 | 许可证 | 说明 |
|--------|--------|------|
| `SeasAGI-Client/` | **GPL 3.0** 完全开源 | 原生桌面客户端（Wails + Go + React），本地统一网关，API Keys 永不上传 |
| `SeasAGI-Server/` | **AGPL 3.0** 开源自部署 | 社区版云端控制面，基础认证 / Combo CRUD / 基础用量 / 基础中继转发 |
| `SeasAGI-Server-Enterprise/` | 闭源 | 企业版云端，包含 Stripe 计费 / 多租户治理 / Combo 治理审批 / 管理后台。详见 [企业版 README](SeasAGI-Server-Enterprise/README.md) |

客户端可以**独立运行** — 所有本地网关功能无需服务端即可使用。云端是可选增值服务，提供远程加速渠道、用量同步、团队协作和企业治理。

---

## 核心能力

### 客户端（SeasAGI-Client）

| 能力 | 说明 |
|------|------|
| **统一 API 网关** | 本地 `localhost:4318`，OpenAI 兼容，无缝替换任意 API 客户端的 Base URL |
| **自适应智能路由** | Fallback / Round-Robin / Sticky 策略，Combo 步骤链编排（主 → 备 → 兜底） |
| **会话粘性** | 基于 SHA1 哈希的会话→步骤映射，30 分钟 TTL，防止对话中途切换模型 |
| **动态 429 惩罚降级** | PenaltyManager，429 +3（上限 10），成功 -1，2 分钟自然衰减 |
| **成本优化引擎** | 按模型/渠道分组分析 + 任务类型差异化建议 + 四种快速策略一键预设 |
| **8 个执行器 + 6 个转换器** | OpenAI / Azure / Anthropic / Gemini / Ollama / DeepSeek / Grok / Relay + 自动协议适配 |
| **全链路可观测性** | 错误分类分布 / 请求时间线 / 链路诊断 / Recharts 可视化 / 成本估算 |
| **自动健康检查与容错** | Key 健康检查（5 分钟探测 + 自动禁用）+ 逐 Key Cooldown + 熔断器 + 指数退避 |
| **本地优先 · 隐私安全** | API Keys 仅存储于 Keychain，恒定时间比较防止时序攻击，请求直连 Provider |
| **Combo 优化工作台** | 我的方案 / 优化建议 / 模板中心 / 执行分析 — 四个标签页，Combo 拖拽排序 |
| **Playground 即时测试** | 内置聊天测试 UI，Combo 选择器 + 执行链可视化 + 步骤回退状态 |
| **i18n** | 中文 / 英文 / 日文 / 韩文 |

### 服务端社区版（SeasAGI-Server）

| 能力 | 说明 |
|------|------|
| 基础认证 | 登录 / 注册 / Token 刷新 |
| 基础渠道管理 | 平台渠道 CRUD |
| 基础 Combo | 用户级 Combo CRUD + 官方模板获取 |
| 基础用量统计 | 用户用量，按模型/渠道分组，时间线，错误分布 |
| 基础租户管理 | 成员、邀请链接、自定义渠道同步、配置快照 |
| 基础管理 API | 用户 / 方案 / 渠道 / Combo / 中继网关管理 |
| 中继基础转发 | 请求透传、健康检查、限流、链路追踪 |

---

## 架构概览

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

## 开源 / 闭源边界

| 组件 | 许可证 | 开源范围 |
|------|--------|----------|
| `SeasAGI-Client/` | **GPL 3.0** | 完全开源，包括本地网关、Provider 执行器、路由策略、安全模块、前端 UI |
| `SeasAGI-Server/` | **AGPL 3.0** | 社区版开源，基础控制面 + 基础中继，可自部署 |
| `SeasAGI-Server-Enterprise/` | **闭源（BSL / EULA）** | 企业版，承载计费、多租户治理、Combo 治理、管理后台 |

---

## 仓库结构

```text
SeasAGI/
├── SeasAGI-Client/                开源客户端 (GPL 3.0)
│   ├── src-app/                    Wails 主源码 (Go + React)
│   ├── scripts/                    构建脚本
│   └── homebrew/                   Homebrew 分发公式
├── SeasAGI-Server/                开源服务端社区版 (AGPL 3.0)
│   ├── platform-api/               社区控制面
│   ├── relay-gateway/              社区中继数据面
│   ├── deploy/                     部署脚本 + systemd + nginx
│   └── scripts/                    构建脚本
├── web-docs/                      官网与文档
│   ├── src-web/                    静态网站
│   └── ...
├── build-all.sh                   一键构建脚本
├── SeasAGI.v5.md                  一页纸摘要
```

---

## 快速开始

### 下载客户端

从 [GitHub Releases](https://github.com/SeasX/SeasAGI/releases) 下载适用于您平台的最新版本。

### 配置渠道

1. 安装并启动 SeasAGI 客户端
2. 在"渠道管理"中添加您的 Provider API Key
3. 将任意 API 客户端的 Base URL 改为 `http://127.0.0.1:4318/v1`
4. 享受智能路由！

### 自部署服务端社区版

```bash
# 构建
bash SeasAGI-Server/scripts/build.sh

# 安装到 Linux 服务器（需要 root）
sudo bash SeasAGI-Server/deploy/install.sh

# 配置环境变量
vi /etc/seasagi/platform-api.env
# 设置 JWT_SECRET, JWT_REFRESH_SECRET, ADMIN_SECRET

# 启动服务
sudo systemctl enable --now seasagi-platform-api
sudo systemctl enable --now seasagi-relay-gateway
```

---

## 构建

### 一键构建（所有版本）

```bash
# 构建客户端 + 服务端社区版 + 服务端企业版
bash ./build-all.sh

# 多平台客户端
bash ./build-all.sh --platform darwin/amd64,darwin/arm64,linux/amd64
```

### 单独构建

```bash
# 客户端
bash SeasAGI-Client/scripts/build.sh

# 服务端社区版
bash SeasAGI-Server/scripts/build.sh

# 服务端企业版
bash SeasAGI-Server-Enterprise/scripts/build.sh
```

### 构建产物

| 产物 | 路径 |
|------|------|
| 客户端应用 | `SeasAGI-Client/src-app/build/bin/` |
| 客户端 DMG | `SeasAGI-Client/build/` |
| 服务端社区版二进制 | `SeasAGI-Server/build/` |
| 服务端企业版二进制 | `SeasAGI-Server-Enterprise/build/` |

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 前端 | React + TypeScript + Vite + Zustand + @dnd-kit + Zod + Recharts |
| 桌面框架 | Wails v2 (Go + WebView) |
| 客户端后端 | Go（本地网关 / 路由引擎 / 成本优化 / Provider 执行器） |
| 服务端框架 | Go + Gin |
| 数据库 | SQLite |
| 支付 | Stripe（企业版） |
| 部署 | systemd + nginx |
| 客户端分发 | Homebrew Cask / DMG / Binary |

---

## 文档

- [一页纸摘要](SeasAGI.v5.md)
- [系统架构设计文档](web-docs/系统架构设计文档.v5.md)
- [开源分析](开源分析.md)
- [客户端 README](SeasAGI-Client/README.md)
- [服务端社区版 README](SeasAGI-Server/README.md)
- [服务端企业版 README](SeasAGI-Server-Enterprise/README.md)

---

## 许可证

- `SeasAGI-Client/` — **GPL 3.0**，完全开源
- `SeasAGI-Server/` — **AGPL 3.0**，社区版开源自部署
- `SeasAGI-Server-Enterprise/` — **闭源（BSL / EULA）**，用于商业部署

---

## 社区

- [GitHub Issues](https://github.com/SeasX/SeasAGI/issues) — 报告 Bug / 提交建议
- [GitHub Releases](https://github.com/SeasX/SeasAGI/releases) — 下载最新版本

---
