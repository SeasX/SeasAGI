# SeasAGI

**本地大模型 API 智能网关** — 开源客户端 + Server Open Core 模式，统一管理多个 LLM Provider 通道，提供智能路由、会话粘滞、成本优化、全链路可观测与团队协同治理能力。

---

## 概述

SeasAGI 是一个分层 AI 网关平台，包含三个子项目：

| 子项目 | 许可 | 说明 |
|--------|------|------|
| `SeasAGI-Client/` | **GPL 3.0** 全量开源 | 桌面原生客户端（Wails + Go + React），本地统一网关，API Key 永不上传 |
| `SeasAGI-Server/` | **AGPL 3.0** 开源可自托管 | 社区版云端控制面，基础认证 / Combo CRUD / 基础用量 / 基础中继转发 |
| `SeasAGI-Server-Enterprise/` | 闭源 | 企业版云端，含 Stripe 计费 / 多租户治理 / Combo 治理审批 / 管理后台。详见[企业版 README](SeasAGI-Server-Enterprise/README.md) |

客户端可**独立运行**，无需 Server 即可使用所有本地网关功能。云端为可选增值服务，提供远程加速通道、用量同步、团队协作与企业治理。

---

## 核心能力

### 客户端（SeasAGI-Client）

| 能力 | 说明 |
|------|------|
| **统一 API 网关** | 本地 `localhost:4318`，兼容 OpenAI 协议，无缝替换任何 API Client 的 Base URL |
| **自适应智能路由** | Fallback / Round-Robin / Sticky 三种策略，Combo 步骤链编排（主→备→保底） |
| **会话粘滞** | 基于 SHA1 哈希的 session→step 映射，30min TTL，防止多轮对话中途切换模型 |
| **动态 429 惩罚降级** | PenaltyManager，429 +3（上限 10），成功 -1，2min 自然衰减 |
| **成本优化引擎** | 按模型/通道分组分析 + 任务类型差异化建议 + 四种快捷策略一键落地 |
| **8 执行器 + 6 翻译** | OpenAI / Azure / Anthropic / Gemini / Ollama / DeepSeek / Grok / 中继 + 协议自动适配 |
| **全链路可观测** | 错误分类分布 / 请求时间线 / 链路诊断 / Recharts 可视化 / 成本估算 |
| **自动探活与容错** | Key 健康检查（5min 探活 + 自动禁用）+ per-key Cooldown + 熔断器 + 指数退避 |
| **本地优先 · 隐私安全** | API Key 仅存于 Keychain，常量时间比较防时序攻击，请求直连 Provider |
| **组合优化工作台** | 我的方案 / 优化建议 / 模板中心 / 执行分析 四标签页，Combo 拖拽排序 |
| **Playground 即时测试** | 内置聊天测试界面，Combo 选择器 + 执行链可视化 + 步骤回退状态 |
| **i18n** | 中文 / 英文 / 日语 / 韩语 |

### 云端社区版（SeasAGI-Server）

| 能力 | 说明 |
|------|------|
| 基础认证 | 登录 / 注册 / Token 刷新 |
| 基础通道管理 | 平台通道 CRUD |
| 基础 Combo | 用户级 Combo CRUD + 官方模板拉取 |
| 基础用量统计 | 用户用量、按模型/通道分组、时间线、错误分布 |
| 基础租户管理 | 成员、邀请链接、自定义通道同步、配置快照 |
| 基础 Admin API | 用户 / 套餐 / 通道 / Combo / Relay Gateway 管理 |
| Relay 基础转发 | 请求透传、健康检查、限流、trace |

---

## 架构概览

```text
┌─────────────────────────────────────────────────────────────┐
│                    SeasAGI-Client (GPL 3.0)                  │
│                                                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌─────────────┐ │
│  │ 本地网关  │  │ 路由引擎 │  │ 成本优化  │  │ 全链路可观测 │ │
│  │ :4318/v1  │  │ Combo/   │  │ 优化建议  │  │ 日志/诊断/   │ │
│  │ OpenAI  │  │ Fallback │  │ 快捷策略  │  │ 图表/成本   │ │
│  └──────────┘  └──────────┘  └──────────┘  └─────────────┘ │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌─────────────┐ │
│  │ 8 执行器  │  │ 6 翻译   │  │ Keychain │  │ Playground  │ │
│  │ Provider  │  │ 协议适配 │  │ 安全存储 │  │ 即时测试    │ │
│  └──────────┘  └──────────┘  └──────────┘  └─────────────┘ │
└──────────────────────┬──────────────────────────────────────┘
                       │ API 协议
┌──────────────────────┴──────────────────────────────────────┐
│                SeasAGI-Server  /  -Enterprise               │
│                                                             │
│  ┌───────────────────────┐  ┌─────────────────────────────┐ │
│  │  社区版 (AGPL 3.0)    │  │  企业版 (闭源)              │ │
│  │                       │  │                             │ │
│  │  基础认证 · 通道管理   │  │   计费 · 套餐门控     │ │
│  │  Combo CRUD · 用量    │  │  多租户 · Combo 治理        │ │
│  │  Relay 转发 · 部署    │  │  高级指标 · BYOK 双回退    │ │
│  │                       │  │  管理后台 · SSO/SCIM        │ │
│  └───────────────────────┘  └─────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │  relay-gateway: 中继数据面，多模态转发 + 健康检查 + trace│ │
│  └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

---

## 开源 / 闭源边界

| 组件 | 许可 | 开源范围 |
|------|------|----------|
| `SeasAGI-Client/` | **GPL 3.0** | 全量开源，包括本地网关、Provider 执行器、路由策略、安全模块、前端 UI |
| `SeasAGI-Server/` | **AGPL 3.0** | 社区版开源，基础控制面 + 基础中继，可自托管 |
| `SeasAGI-Server-Enterprise/` | **闭源（BSL / EULA）** | 企业版，承载计费、多租户治理、Combo 治理、管理后台 |

---

## 仓库结构

```text
SeasAGI/
├── SeasAGI-Client/                开源客户端（GPL 3.0）
│   ├── src-app/                    Wails 主线源码（Go + React）
│   ├── scripts/                    构建脚本
│   └── homebrew/                   Homebrew 分发 formula
├── SeasAGI-Server/                开源云端社区版（AGPL 3.0）
│   ├── platform-api/               社区版控制面
│   ├── relay-gateway/              社区版中继数据面
│   ├── deploy/                     部署脚本 + systemd + nginx
│   └── scripts/                    构建脚本
├── web-docs/                      官网与文档
│   ├── src-web/                    官网静态页
│   ├── 系统架构设计文档.v5.md      架构文档
│   └── ...
├── build-all.sh                   一键构建脚本
├── SeasAGI.v5.一页纸总结.md        一页纸总结
```

---

## 快速开始

### 下载客户端

从 [GitHub Releases](https://github.com/SeasX/SeasAGI/releases) 下载对应平台的最新版本。

### 配置通道

1. 安装并启动 SeasAGI Client
2. 在"通道管理"中添加你的 Provider API Key
3. 将任何 API Client 的 Base URL 改为 `http://127.0.0.1:4318/v1`
4. 开始享受智能路由！

### 自托管云端社区版

```bash
# 编译
bash SeasAGI-Server/scripts/build.sh

# 安装到 Linux 服务器（需 root）
sudo bash SeasAGI-Server/deploy/install.sh

# 配置环境变量
vi /etc/seasagi/platform-api.env
# 设置 JWT_SECRET、JWT_REFRESH_SECRET、ADMIN_SECRET

# 启动服务
sudo systemctl enable --now seasagi-platform-api
sudo systemctl enable --now seasagi-relay-gateway
```

---

## 构建

### 一键构建

```bash
# 默认：Client + Server 社区版
bash ./build-all.sh

# 构建企业版 Server
bash ./build-all.sh --server-edition enterprise

# 多平台 Client
bash ./build-all.sh --platform darwin/amd64,darwin/arm64,linux/amd64
```

### 单独构建

```bash
# 客户端
bash SeasAGI-Client/scripts/build.sh

# Server 社区版
bash SeasAGI-Server/scripts/build.sh

# Server 企业版
bash SeasAGI-Server-Enterprise/scripts/build.sh
```

### 构建产物

| 产物 | 路径 |
|------|------|
| 客户端应用 | `SeasAGI-Client/src-app/build/bin/` |
| 客户端 DMG | `SeasAGI-Client/build/` |
| Server 社区版二进制 | `SeasAGI-Server/build/` |

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 前端 | React + TypeScript + Vite + Zustand + @dnd-kit + Zod + Recharts |
| 桌面框架 | Wails v2 (Go + WebView) |
| 客户端后端 | Go（本地网关 / 路由引擎 / 成本优化 / Provider 执行器） |
| 云端框架 | Go + Gin |
| 数据库 | SQLite |
| 支付 | Stripe（企业版） |
| 部署 | systemd + nginx |
| 客户端分发 | Homebrew Cask / DMG / 二进制 |

---

## 文档

- [一页纸总结](SeasAGI.v5.一页纸总结.md)
- [系统架构设计文档](web-docs/系统架构设计文档.v5.md)
- [开源分析](开源分析.md)
- [客户端 README](SeasAGI-Client/README.md)
- [Server 社区版 README](SeasAGI-Server/README.md)
- [Server 企业版 README](SeasAGI-Server-Enterprise/README.md)

---

## 许可

- `SeasAGI-Client/` — **GPL 3.0**，全量开源
- `SeasAGI-Server/` — **AGPL 3.0**，社区版开源可自托管
- `SeasAGI-Server-Enterprise/` — **闭源（BSL / EULA）**，面向商业部署

---

## 社区

- [GitHub Issues](https://github.com/SeasX/SeasAGI/issues) — 报告 Bug / 提交建议
- [GitHub Releases](https://github.com/SeasX/SeasAGI/releases) — 下载最新版本

---
