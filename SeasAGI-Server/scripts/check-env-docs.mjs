#!/usr/bin/env node
// 校验 deploy/.env.example 与 README.md 环境变量表的一致性。
//
// 规则：
//   1. .env.example 中的每个 KEY 必须出现在 README 环境变量表；
//   2. README 环境变量表中的每个 KEY 必须出现在 .env.example，或在允许清单中
//      （允许清单只包含确实由组件读取、但不需要写进 .env.example 的可选项）。
// 失败时打印明细并以 exit 1 退出。

import { readFileSync, existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const envPath = resolve(rootDir, "deploy", ".env.example");
const readmePath = resolve(rootDir, "README.md");

// 仅存在于文档、不需要写进 .env.example 的可选变量。
const DOC_ONLY_ALLOWLIST = new Set([
  "SEASAGI_LOCALES_DIR",
  "BACKUP_DIR",
  "METRICS_TOKEN",
]);

function parseEnvKeys(text) {
  const keys = new Set();
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) continue;
    const match = line.match(/^([A-Z][A-Z0-9_]*)\s*=/);
    if (match) keys.add(match[1]);
  }
  return keys;
}

function parseDocKeys(text) {
  const keys = new Set();
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line.startsWith("|")) continue;
    const cells = line.split("|");
    if (cells.length < 3) continue;
    const firstCell = cells[1];
    for (const match of firstCell.matchAll(/`([A-Z][A-Z0-9_]*)`/g)) {
      keys.add(match[1]);
    }
  }
  return keys;
}

function difference(a, b) {
  return [...a].filter((item) => !b.has(item)).sort();
}

function main() {
  const checks = [
    { name: ".env.example", path: envPath },
    { name: "README.md", path: readmePath },
  ];
  for (const check of checks) {
    if (!existsSync(check.path)) {
      console.error(`[env-docs] 缺少文件: ${check.path}`);
      process.exit(1);
    }
  }

  const envKeys = parseEnvKeys(readFileSync(envPath, "utf8"));
  const docKeys = parseDocKeys(readFileSync(readmePath, "utf8"));

  const missingInDocs = difference(envKeys, docKeys);
  const undocumented = difference(docKeys, envKeys).filter(
    (key) => !DOC_ONLY_ALLOWLIST.has(key)
  );

  let failed = false;
  if (missingInDocs.length > 0) {
    failed = true;
    console.error("[env-docs] 以下变量存在于 deploy/.env.example，但未写入 README 环境变量表:");
    for (const key of missingInDocs) console.error(`  - ${key}`);
  }
  if (undocumented.length > 0) {
    failed = true;
    console.error("[env-docs] 以下变量出现在 README 环境变量表，但不在 deploy/.env.example 或允许清单中:");
    for (const key of undocumented) console.error(`  - ${key}`);
  }

  if (failed) process.exit(1);

  console.log(
    `[env-docs] OK: .env.example ${envKeys.size} 个变量，README 文档 ${docKeys.size} 个变量，全部一致。`
  );
}

main();
