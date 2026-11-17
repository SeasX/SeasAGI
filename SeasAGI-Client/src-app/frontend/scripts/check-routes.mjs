#!/usr/bin/env node
// 路由可达性检查（纯 Node，无第三方依赖）。
//
// 背景：P1-1「企业门户是死代码」——侧栏有入口，路由却重定向回首页，
// 类型检查与 Go 测试都发现不了。本脚本把「每个侧栏入口都必须落到真实页面」
// 变成可 CI 执行的门禁：
//   1. 解析 Layout.tsx 中 navEntries 的所有 to 路径；
//   2. 解析 App.tsx 的 <Route path=... element=...>；
//   3. 侧栏路径必须存在，且 element 不能是 <Navigate ...>（重定向视为不可达）。
// 失败时打印明细并以 exit 1 退出。

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const layoutPath = resolve(rootDir, "src", "components", "Layout.tsx");
const appPath = resolve(rootDir, "src", "App.tsx");

const errors = [];

const layout = readFileSync(layoutPath, "utf8");
const app = readFileSync(appPath, "utf8");

// 侧栏入口：{ to: "/path", ... }
const navPaths = new Set();
for (const match of layout.matchAll(/\bto:\s*"([^"]+)"/g)) {
  const path = match[1];
  if (path.startsWith("/")) navPaths.add(path);
}

// 路由表：<Route path="/path" element={<Component ... />} />
const routes = new Map();
for (const match of app.matchAll(/<Route\s+path="([^"]+)"\s+element=\{<([A-Za-z0-9_]+)/g)) {
  routes.set(match[1], match[2]);
}

if (navPaths.size === 0) {
  errors.push("未从 Layout.tsx 解析到任何侧栏路径，检查脚本正则或文件结构是否变化");
}
if (routes.size === 0) {
  errors.push("未从 App.tsx 解析到任何路由，检查脚本正则或文件结构是否变化");
}

for (const path of [...navPaths].sort()) {
  // "/" 也允许以 index 或精确路径注册。
  const component = routes.get(path) ?? (path === "/" ? routes.get("/") : undefined);
  if (!component) {
    errors.push(`侧栏入口 ${path} 没有对应路由`);
    continue;
  }
  if (component === "Navigate") {
    errors.push(`侧栏入口 ${path} 被重定向，页面不可达`);
  }
}

// token-market 子页不直接出现在侧栏，但也应可达：抽查入口页。
for (const required of ["/token-market", "/token-market/create"]) {
  if (!routes.has(required)) {
    errors.push(`缺少路由 ${required}`);
  }
}

if (errors.length > 0) {
  console.error("Route reachability check failed:");
  for (const message of errors) console.error(`  - ${message}`);
  process.exit(1);
}

console.log(`route check: ${navPaths.size} nav entries reachable (${routes.size} routes)`);
