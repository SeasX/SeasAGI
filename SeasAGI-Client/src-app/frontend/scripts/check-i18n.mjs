#!/usr/bin/env node
// i18n 文案一致性检查（纯 Node，无第三方依赖）。
//
// 校验项：
//   1. zh-CN / en / ja / ko 四份 locale 的 key 集合完全一致（缺失或多余都报错）；
//   2. 任何 key 的值不得为空字符串；
//   3. 同一 key 在四份语言中的 {{name}} 占位符集合必须一致；
//   4. 值中不得出现单花括号占位符（如 {name}），避免插值失效。
// 失败时打印明细并以 exit 1 退出。

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const localesDir = resolve(rootDir, "src", "i18n", "locales");
const LOCALES = ["zh-CN", "en", "ja", "ko"];
const BASE_LOCALE = "zh-CN";
const PLACEHOLDER_RE = /\{\{([a-zA-Z0-9_]+)\}\}/g;
const SINGLE_BRACE_RE = /(?<!\{)\{[a-zA-Z0-9_]+\}(?!\})/;

const errors = [];

function loadLocale(locale) {
  const path = resolve(localesDir, `${locale}.json`);
  return JSON.parse(readFileSync(path, "utf8"));
}

function placeholders(value) {
  const found = new Set();
  for (const match of value.matchAll(PLACEHOLDER_RE)) {
    found.add(match[1]);
  }
  return found;
}

function sorted(values) {
  return [...values].sort();
}

function main() {
  const dicts = Object.fromEntries(
    LOCALES.map((locale) => [locale, loadLocale(locale)])
  );

  const baseKeys = new Set(Object.keys(dicts[BASE_LOCALE]));

  for (const locale of LOCALES) {
    const keys = new Set(Object.keys(dicts[locale]));

    const missing = sorted([...baseKeys].filter((key) => !keys.has(key)));
    const extra = sorted([...keys].filter((key) => !baseKeys.has(key)));
    if (missing.length > 0) {
      errors.push(`[key] ${locale} 缺少 ${missing.length} 个 key:`);
      for (const key of missing) errors.push(`  - ${key}`);
    }
    if (extra.length > 0) {
      errors.push(`[key] ${locale} 多出 ${extra.length} 个 key:`);
      for (const key of extra) errors.push(`  + ${key}`);
    }

    for (const [key, value] of Object.entries(dicts[locale])) {
      if (typeof value !== "string") {
        errors.push(`[type] ${locale}:${key} 的值不是字符串`);
        continue;
      }
      if (value.trim() === "") {
        errors.push(`[empty] ${locale}:${key} 的值为空字符串`);
        continue;
      }
      if (SINGLE_BRACE_RE.test(value)) {
        errors.push(`[brace] ${locale}:${key} 含单花括号占位符: ${value}`);
      }
    }
  }

  // 占位符集合一致性（仅比较四份语言都存在的 key）。
  for (const key of baseKeys) {
    const perLocale = new Map();
    let complete = true;
    for (const locale of LOCALES) {
      const value = dicts[locale][key];
      if (typeof value !== "string") {
        complete = false;
        break;
      }
      perLocale.set(locale, sorted(placeholders(value)));
    }
    if (!complete) continue;

    const reference = perLocale.get(BASE_LOCALE).join(",");
    for (const locale of LOCALES) {
      if (perLocale.get(locale).join(",") !== reference) {
        errors.push(
          `[placeholder] ${key} 占位符不一致: ${BASE_LOCALE}=[${reference}] ${locale}=[${perLocale
            .get(locale)
            .join(",")}]`
        );
      }
    }
  }

  if (errors.length > 0) {
    console.error(`[check-i18n] 发现 ${errors.length} 个问题:`);
    for (const line of errors) console.error(line);
    process.exit(1);
  }

  console.log(
    `[check-i18n] OK: ${LOCALES.length} 份语言、${baseKeys.size} 个 key，key/空值/占位符全部一致。`
  );
}

main();
