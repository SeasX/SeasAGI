# SeasAGI v5 一页纸总结

## 1. 项目一句话

SeasAGI 是一个面向 AI 重度用户和开发者的本地统一模型网关，让用户像管理网络出口一样管理 LLM API 出口。

SeasAGI 不只是一个"换 Key 的工具"，而是在 AI 基础设施消费侧构建"本地统一模型网关"入口。

它的价值在于：

- 把多模型接入复杂度从"每个客户端重复配置"降低为"一次本地配置"
- 把用户对 Key 安全的担忧转化为可感知的产品信任
- 把运行时 Provider 不稳定转化为自适应路由降级，用户无感知
- 把用量黑盒转化为全维度洞察（模型/通道/时间线/成本/错误）
- 把未来更高价值的策略路由、团队版、订阅服务建立在一个已经可运行的本地网关基础上


## 2. 用户痛点

当前 AI 客户端和 IDE 插件存在几个高频痛点：

- 不同模型平台需要重复配置 `Base URL`、`API Key`、模型名
- 平台通道和用户自购 Key 混杂，切换成本高
- 上游服务不稳定时，缺少统一回退和诊断能力
- 用户普遍担心自带 Key 被平台采集或明文保存
- 多轮对话中途切换模型导致上下文断裂和幻觉
- Provider 频繁 429 限流时，无法自适应调整路由优先级
- 无法直观洞察哪个模型/通道用量最高、成本最优

## 3. 我们的解决方案

SeasAGI 在本地提供一个统一 OpenAI-compatible 入口，让所有第三方客户端只配置一次本地地址，即可接入多个模型出口。

核心能力：

- 本地统一入口：默认 `http://127.0.0.1:4318/v1`，实际随监听端口动态变化
- 双通道架构：平台通道 + 自定义通道
- 用户自带 Key 永不上传，仅保存在本地 Keychain
- 路由策略：`fallback`、`round_robin`、`sticky`
- 模型级 Combo：支持显式 `channel + model` 步骤链 + 拖拽排序 + 主/备/保底角色语义
- 导航融合：左侧导航已收敛为"组合优化"单入口，工作台内分 4 标签（我的方案 / 优化建议 / 模板中心 / 执行分析）
- Playground 调试：Combo 选择器 + 执行链可视化 + 步骤回退状态显示（step_role badge）
- 默认入口支持 Combo：HomePage 默认 Combo 选择器 + Wails 持久化
- Combo 云端同步：三层视图（本地/官方/团队）+ Fetch/Push/Update/Delete 完整同步能力
- Combo 指标：请求次数 / 回退次数 / Step1 命中率 / 末步命中率
- 智能优化与 Combo 协同：多步回退建议、结构差异预览、一键应用为 Combo
- 步骤内 Provider/Channel 候选池：每个 Step 支持多候选承载 + 自动排序
- 请求级执行约束：max_price / max_latency_ms / data_policy
- 工具调用专用路由策略：优先 tool success rate 高的 provider
- 快捷策略别名：稳定优先 / 成本优先 / 速度优先 / 工具优先
- 任务类型感知：chat / tools / json / long_context / batch_low_cost 差异化执行
- 企业 BYOK / 平台共享容量双层回退
- 会话粘滞路由：基于首条 user message SHA1 哈希，30min TTL，防止多轮对话切换模型
- 动态 429 惩罚降级：PenaltyManager，429 +3，成功 -1，2min 自然衰减
- Fallback 排序预设：intelligence / speed / budget 一键重排
- 8 种 Provider 执行器：OpenAI、Azure、Anthropic、Gemini、Ollama、DeepSeek、Grok、平台中继
- 6 种格式翻译：OpenAI Chat/Responses、Anthropic、Gemini、Vertex AI、Passthrough
- Key 健康检查：5min 定时探活，连续 3 次失败自动标记 unhealthy
- Key 级 Cooldown：per-key 120s TTL，429 时精确冷却，不影响其他 key
- 多模态内容扁平化：text-only array content 自动合并为 string
- Playground 即时测试：聊天 UI + 模型选择 + 本地 gateway 通信
- 常量时间 Key 比较：crypto/subtle.ConstantTimeCompare，消除时序侧信道
- Zod 前端校验：8 种 schema + 表单集成
- 日志与诊断：链路可视化、CSV 导出、结构化错误复制
- RTK Token 压缩：9 种输出类型自动检测 + 压缩
- 全模态 API：Embeddings / Image / TTS / STT
- 隧道远程访问：Cloudflare Tunnel + Tailscale Funnel
- 多账户容错：多 Key 轮转 + 指数退避 + 熔断器 + 7 条错误规则
- OAuth 生态：PKCE + 4 Provider 注册表 + 本地 loopback 回调 + Token 自动刷新
- 本地访问令牌：左侧导航独立页，支持查看 / 复制 / 重置
- 云端能力：注册 / 登录、平台通道同步、用量 / 账单查询
- 云端分析：按模型/通道分组 + 成本估算（GPT-4o 定价）+ 近期错误 + 请求时间线 + 错误分类分布
- 管理端可视化：Recharts BarChart / LineChart / PieChart，6 Tab UsagePage
- Caveman 精简输出：4 种风格注入
- Reasoning Content：DeepSeek-R1/Kimi/QwQ 推理链注入
- MCP 工具去重：12 组等价映射
- 国际化：支持中文、英文、日语、韩语，跟随系统语言

## 4. 为什么现在值得做

- AI 用户同时使用多个客户端和多个模型已成为常态
- 本地优先、安全优先、统一入口的需求正在快速上升
- 市场上已有"云聚合平台"，但"本地网关 + 强安全边界 + 自适应路由 + 全链路可观测"的成熟产品仍然稀缺
- SeasAGI 可以先从个人开发者市场切入，再向团队版、跨平台版扩张

## 5. 产品定位

- 目标用户：
  - AI 重度用户
  - 个人开发者
  - 独立开发者
  - 小型 AI 团队技术负责人
- 当前形态：
  - Wails 桌面客户端（当前主验证平台为 macOS，代码结构已覆盖 Windows / Linux）
  - 本地 OpenAI-compatible API 网关
- 核心场景：
  - 统一接入多个模型平台
  - 平台通道与自购 Key 切换
  - 通道故障回退与自适应降级
  - 请求失败链路诊断
  - 多轮对话会话粘滞
  - 用量洞察与成本优化

## 6. 差异化壁垒

### 6.1 安全边界明确

- 用户自带 Key 不上传平台
- Key 保存在 macOS Keychain
- 本地访问令牌保存在本地 SQLite，不进入云端
- 令牌校验使用常量时间比较，消除时序侧信道攻击
- 平台通道与用户通道边界在 UI 中明确展示

### 6.2 自适应智能路由

- 不是简单的"默认通道切换器"
- 会话粘滞：多轮对话自动路由到同一模型，防止幻觉
- 动态 429 惩罚：Provider 限流时自动下沉优先级，恢复后自动回升
- 排序预设：intelligence / speed / budget 一键重排
- Key 健康检查：自动探活，失效 Key 自动禁用
- per-key Cooldown：精确控制，不影响其他 key
- 多模态内容扁平化：兼容更多客户端格式
- 步骤内 Provider/Channel 候选池 + 自动排序：同一步内按稳定性/成本/延迟动态选优
- 工具调用独立路由策略：依据 tool success rate 排序，而非仅价格优先
- 请求级动态约束：每次调用可附加 max_price / max_latency_ms / data_policy

### 6.3 模型级 Combo 编排与治理

- 故障回退编排：不是静态模型列表，而是"主模型 → 备用模型 → 保底模型"显式回退链
- 导航融合：左侧单入口"组合优化"，工作台内统一完成配置→建议→应用→验证
- 智能优化与 Combo 协同：多步回退建议 + 结构差异预览 + 一键应用
- 云端同步：三层视图（本地/官方/团队）+ 完整同步（Fetch/Push/Update/Delete）
- 执行分析：Combo 命中率/回退率/平均尝试次数面板
- Server 控制面：三级 Combo 管理 + 版本发布/回滚 + 差异化下发
- 企业治理：可见性策略（角色/套餐/用户组/租户四级）+ 部署审批流程
- Combo 运行时埋点：请求次数/回退次数/Step1命中率/末步命中率
- 快捷策略别名：稳定优先 / 成本优先 / 速度优先 / 工具优先
- 任务类型感知：按 chat/tools/json/long_context 差异化执行

### 6.4 全链路可观测

- 日志支持命中链路和逐步失败原因
- 错误分类分布（429/401/403/404/timeout/500/503）
- 请求时间线时序图（24h/7d/30d）
- 按模型/通道分组分析
- 近期错误快速查看
- 成本估算（GPT-4o 定价基准）
- 管理端图表可视化（BarChart / LineChart / PieChart）

### 6.4 本地网关体验

- 所有客户端只需配置一次本地地址
- 用户不需要在 Chatbox、Cherry Studio、Cursor 等多个客户端反复改配置
- Playground 即时测试，配置完通道直接验证

### 6.5 可扩展架构

- 外部统一 OpenAI-compatible
- 内部通过协议归一化 + Provider 执行器扩展更多上游
- 执行器注册表支持动态注册新 Provider
- OAuth PKCE 流程支持任意 OAuth 2.0 Provider
- 后续可平滑扩展团队版、策略下发、配置同步

## 7. 三子项目规整

- `SeasAGI-Client/`：客户端子项目，内部保留 `src-app` 原结构，承载 Wails 桌面客户端主线源码与客户端编译脚本
- `SeasAGI-Server/`：云端子项目，内部保留 `platform-api`、`relay-gateway`、`src-admin`、`deploy` 原结构，承载控制面、数据面、管理后台与部署脚本
- `web-docs/`：官网与文档子项目，内部保留 `src-web`、主线架构/PRD/排期文档，以及 `cc-switch`、`9router` 等历史或参考目录
- 根目录：仅保留总 `README.md`、`SeasAGI.v5.一页纸总结.md` 与一键编译脚本 `build-all.sh`

## 8. 当前进展

当前 `v5` 已完成的关键能力：

- Wails + Go 客户端已跑通
- 本地网关支持 `/v1/models`、`/v1/chat/completions` + 全模态 API
- 平台通道与自定义通道双通道管理
- 路由策略 `fallback` / `round_robin` / `sticky` + Combo 步骤链
- 会话粘滞路由（SHA1 + session→step + 30min TTL）
- 动态 429 惩罚降级（PenaltyManager + 时间衰减 + Circuit Breaker 协同）
- Fallback 排序预设（intelligence / speed / budget）
- Key 健康检查（5min 探活 + 连续 3 次失败自动禁用）
- Key 级 Cooldown（per-key 120s TTL + 自动清理）
- 常量时间 Key 比较（crypto/subtle）
- 多模态内容扁平化
- Playground 即时测试页面
- 8 种 Provider 执行器 + 6 种格式翻译
- 日志筛选、排序、CSV 导出、结构化错误复制
- RTK Token 压缩 + 隧道远程访问
- 多账户容错 + OAuth 2.0 PKCE
- Caveman + Reasoning + MCP 工具去重
- Zod 前端校验 + Combo 拖拽排序
- 云端分析 API（按模型/通道分组 + 成本估算 + 近期错误 + 时间线 + 错误分类分布）
- 管理端可视化（Recharts BarChart / LineChart / PieChart）
- 国际化（中文/英文/日语/韩语）