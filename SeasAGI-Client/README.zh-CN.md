[🇬🇧 English](README.md) | [🇨🇳 中文](README.zh-CN.md) | [🇯🇵 日本語](README.ja.md) | [🇰🇷 한국어](README.ko.md)

---

# SeasAGI Client

[![Build status](https://ci.appveyor.com/api/projects/status/github/SeasX/SeasAGI?svg=true)](https://ci.appveyor.com/project/SeasX/SeasAGI)
[![macOS](https://img.shields.io/badge/platform-macOS-blue)](https://github.com/SeasX/SeasAGI)
[![Linux](https://img.shields.io/badge/platform-Linux-blue)](https://github.com/SeasX/SeasAGI)
[![Windows](https://img.shields.io/badge/platform-Windows-blue)](https://github.com/SeasX/SeasAGI)
[![License](https://img.shields.io/badge/license-GPL%20v3-green)](LICENSE)

客户端子项目，保留桌面端主线源码与客户端构建脚本。

## 目录

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

## 职责

- `src-app`：Wails v2 桌面客户端主线源码
- `src-app/frontend`：React + Vite + TypeScript 前端（含 @dnd-kit 拖拽 + Zod 校验）
- `src-app/internal`：本地网关、路由、OAuth、本地访问令牌、日志、优化、用量等 Go 模块
- `scripts/build.sh`：客户端构建入口，按当前 OS 自动分发
- `scripts/build-macos.sh`：macOS 构建 + DMG 打包
- `scripts/build-linux.sh`：Linux（Ubuntu/CentOS）构建
- `scripts/build-windows.sh`：Windows 构建
- `scripts/dev.sh`：Wails 本地开发入口
- `homebrew/seasagi.rb`：macOS Homebrew 分发 formula

## v5 核心能力

### 组合优化工作台（统一入口）

左侧导航已融合为"组合优化"单入口，工作台内分 4 个标签页：

- **我的方案**：本地/官方/团队三层 Combo 视图 + 来源标签 + 云端同步（Fetch/Push/Update/Delete）
- **优化建议**：智能优化多步回退建议 + 结构差异预览 + 一键应用为 Combo
- **模板中心**：官方/团队模板搜索、刷新、套用
- **执行分析**：Combo 命中率/回退率/平均尝试次数分析面板

### 模型级 Combo

- Combo 本质是"故障回退编排"：主模型 → 备用模型 → 保底模型（primary/backup/last_resort 三步角色语义）
- 步骤链编辑 + 拖拽排序（@dnd-kit）+ 步骤级 channel_id + model 绑定
- Fallback / Round-Robin 策略 + Sticky Uses 配置
- 云端同步完整：`FetchCloudCombos` / `PushCloudCombo` / `UpdateCloudCombo` / `DeleteCloudCombo`
- 本地迁移器 `migrateModelCombos()` 自动补齐旧数据到统一 Schema（combo_id/logical_name/display_name/status/source/version）
- Playground 即时测试：Combo 选择器 + 执行链可视化 + 步骤回退状态显示（step_role badge）

### 智能优化与 Combo 协同

- 优化器支持多步回退建议（primary/backup/last_resort）
- `PreviewComboOptimization` 实现结构差异预览（当前步骤 vs 建议步骤）
- `ApplyComboOptimization` 支持新建/覆盖更新 Combo
- `ApplyRecommendation` 支持一键生成 Combo
- 推荐显式区分：模型替换建议 / 回退链增强建议 / 运行参数建议

### 路由与策略

- Fallback / Round-Robin / Sticky 三种基础策略
- Combo 步骤链（显式 channel + model 组合）
- 会话粘滞路由：基于首条 user message SHA1 哈希的 session→step 映射，30min TTL
- 动态 429 惩罚降级：PenaltyManager，429 +3（上限 10），成功 -1，2min 自然衰减
- Fallback 排序预设：intelligence / speed / budget 一键重排
- Combo 步骤拖拽排序（@dnd-kit）

### 步骤内 Provider/Channel 候选池（OpenRouter 借鉴成果）

- 每个 Step 不再只支持单一固定 channel，支持多候选承载池
- 步骤内排序规则：按稳定性/成本/延迟/吞吐自动重排候选
- 请求级动态约束：`max_price`、`max_latency_ms`、`data_policy`、`allow_cross_provider_fallback`
- 工具调用（tool-calling）请求独立路由策略（优先 tool success rate 高的 provider）
- 快捷策略别名：稳定优先 / 成本优先 / 速度优先 / 工具优先
- 任务类型感知（task_profile）：chat / tools / json / long_context / batch_low_cost 差异化执行

### Provider 与容错

- 9 种 Provider 执行器：OpenAI / Azure / Anthropic / Gemini / Ollama / DeepSeek / Grok / Vertex / 平台中继
- 6 种格式翻译：OpenAI Chat / OpenAI Responses / Anthropic / Gemini / Vertex AI / Passthrough
- 多 Key 轮转 + 指数退避 + 熔断器 + 7 条错误规则
- Key 健康检查：HealthChecker，5min 定时探活，连续 3 次失败自动标记 unhealthy
- Key 级 Cooldown：CooldownManager，per-key 120s TTL，429 时设置冷却
- Provider 级健康评分参与运行时排序决策
- 企业 BYOK / 平台共享容量双层回退策略
- 多模态内容扁平化：text-only array content 自动合并为 string

### Combo 运行时埋点与指标

- `ComboRouteMetrics`：请求次数 / 回退次数 / Step1 命中率 / 末步命中率
- `step_role` 已传播到 RouteStep 日志（primary/backup/last_resort）
- `GetComboRouteMetrics` 已暴露到前端
- `ExecAnalysisPage` 展示命中率概览条形图 + 平均尝试次数 + Combo 详情面板

### 安全

- Keychain 集成安全存储
- 常量时间 Key 比较（crypto/subtle.ConstantTimeCompare）
- Zod 前端表单校验
- 隧道远程访问（Cloudflare Tunnel + Tailscale Funnel）
- OAuth 2.0 PKCE（4 Provider + Token 自动刷新）

### MITM 代理 — 一键接管 AI API 流量

- **一键开关**：设置页 → MITM 标签页，一键启停，无需手动配置证书
- **自动 CA 信任**：自动生成根 CA 并安装到 macOS/Linux/Windows 系统信任链，按域名动态签发证书（23h TTL 缓存）
- **系统代理集成**：自动设置操作系统级 HTTP/HTTPS 代理，透明拦截所有匹配流量
- **域名白名单**：可配置需要接管的 API 域名（默认：OpenAI、Anthropic、Gemini、DeepSeek、Grok、OpenRouter），运行时动态增删
- **连通性检查**：内置域名级「测试」按钮，一键验证接管状态、可达性和延迟
- **本地网关路由**：被接管的 HTTPS 流量透明转发到本地 SeasAGI 网关，统一路由、优化和可观测
- **实时拦截日志**：最近 20 条拦截请求的实时表格（方法、域名、路径、状态码、延迟），每 3 秒自动刷新
- **CLI 兼容提示**：自动检测用户 Shell（bash/zsh/fish/PowerShell/cmd），提供可复制的 `export` / `unset` 命令，终端工具也能轻松接管
- **崩溃恢复**：重启时自动清理残留系统代理，健康探针 goroutine 检测代理异常后自动停止
- **透传安全**：未匹配域名透明隧道转发，60 秒超时保护，不影响正常上网

### 体验

- Playground 即时测试页面
- RTK Token 压缩（9 种输出类型自动检测）
- Caveman 精简输出（4 种风格）
- Reasoning Content 注入
- MCP 工具去重
- i18n（中文/英文/日语/韩语）

## 版本号管理

版本号统一由单一文件维护，构建时注入前端，运行时无需依赖该文件。

### 版本号源

[`scripts/version.txt`](file:///Users/Neeke/data/www/SeasAGI/SeasAGI/SeasAGI-Client/scripts/version.txt) 是唯一的版本号定义文件：

```
0.1.0
```

- 更新版本号只需修改此文件
- 可通过环境变量 `SEASAGI_VERSION` 临时覆盖（如 `SEASAGI_VERSION=1.0.0 bash build-all.sh`）

### 构建时注入流程

1. 各平台构建脚本（`build-macos.sh` / `build-linux.sh` / `build-windows.sh`）启动时读取 `scripts/version.txt`
2. 将版本号通过 `export VITE_APP_VERSION="$VERSION"` 注入前端构建环境
3. Vite 构建时，`import.meta.env.VITE_APP_VERSION` 被编译为静态值写入前端产物
4. Wails 将前端产物打包进桌面客户端二进制中

### 前端展示

客户端界面左上角 Logo 右侧显示版本号徽标：

```
[icon] SeasAGI  v0.1.0
```

实现位置：[`Layout.tsx`](file:///Users/Neeke/data/www/SeasAGI/SeasAGI/SeasAGI-Client/src-app/frontend/src/components/Layout.tsx) 中使用 `import.meta.env.VITE_APP_VERSION` 渲染。

### 构建后的独立性

版本号在构建时已编译为静态值嵌入前端产物，`scripts/version.txt` 不会被打包进客户端。客户端运行完全独立于该文件。

## 常用命令

```bash
# 启动客户端开发模式
bash SeasAGI-Client/scripts/dev.sh

# 编译当前平台客户端
bash SeasAGI-Client/scripts/build.sh

# 编译指定平台
bash SeasAGI-Client/scripts/build-macos.sh   # macOS
bash SeasAGI-Client/scripts/build-linux.sh    # Linux (Ubuntu/CentOS)
bash SeasAGI-Client/scripts/build-windows.sh  # Windows
```

## 构建产物

- `SeasAGI-Client/src-app/build/bin/SeasAGI.app`
- `SeasAGI-Client/src-app/build/bin/SeasAGI.exe`
- `SeasAGI-Client/src-app/build/bin/SeasAGI`
- `SeasAGI-Client/build/SeasAGI-<version>.dmg`（仅 macOS）

## Linux 依赖

Ubuntu/Debian:
```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev build-essential
```

CentOS/RHEL:
```bash
sudo yum install gtk3-devel webkit2gtk3-devel
```