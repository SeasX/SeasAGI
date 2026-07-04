[🇬🇧 English](README.md) | [🇨🇳 中文](README.zh-CN.md) | [🇯🇵 日本語](README.ja.md) | [🇰🇷 한국어](README.ko.md)

---

# SeasAGI

**ローカル LLM API インテリジェントゲートウェイ** — オープンソースクライアント + サーバーオープンコアモデル、複数の LLM Provider チャネルを統合管理し、インテリジェントルーティング、セッションスティッキネス、コスト最適化、フルチェーンの可観測性、チーム協調ガバナンスを提供します。

---

## 概要

SeasAGI は 3 つのサブプロジェクトから構成される階層型 AI ゲートウェイプラットフォームです：

| サブプロジェクト | ライセンス | 説明 |
|-------------|---------|-------------|
| `SeasAGI-Client/` | **GPL 3.0** 完全オープンソース | ネイティブデスクトップクライアント（Wails + Go + React）、ローカル統合ゲートウェイ、API Keys はアップロードされない |
| `SeasAGI-Server/` | **AGPL 3.0** オープンソース セルフホスト可能 | コミュニティエディションクラウドコントロールプレーン、基本認証 / Combo CRUD / 基本使用量 / 基本リレーフォワーディング |
| `SeasAGI-Server-Enterprise/` | クローズドソース | エンタープライズエディションクラウド、Stripe 課金 / マルチテナントガバナンス / Combo ガバナンス承認 / 管理ダッシュボードを含む。[エンタープライズ README](SeasAGI-Server-Enterprise/README.md) を参照 |

クライアントは**単体で動作**可能 — すべてのローカルゲートウェイ機能は Server なしで動作します。クラウドはリモート高速チャネル、使用量同期、チーム協調、エンタープライズガバナンスを提供するオプションの付加価値サービスです。

---

## コア機能

### クライアント（SeasAGI-Client）

| 機能 | 説明 |
|------------|-------------|
| **統合 API ゲートウェイ** | ローカル `localhost:4318`、OpenAI 互換、任意の API クライアントの Base URL をシームレスに置き換え |
| **適応型インテリジェントルーティング** | フォールバック / ラウンドロビン / スティッキー戦略、Combo ステップチェーンオーケストレーション（プライマリ → バックアップ → 最終手段） |
| **セッションスティッキネス** | SHA1 ハッシュベースのセッション→ステップマッピング、30分 TTL、会話中のモデル切り替えを防止 |
| **動的 429 ペナルティ劣化** | PenaltyManager、429 +3（上限 10）、成功 -1、2分自然減衰 |
| **コスト最適化エンジン** | モデル/チャネル別グループ分析 + タスクタイプ別差分化提案 + 4つのクイック戦略ワンクリックプリセット |
| **8 Executor + 6 Translator** | OpenAI / Azure / Anthropic / Gemini / Ollama / DeepSeek / Grok / Relay + 自動プロトコル適応 |
| **フルチェーン可観測性** | エラー分類分布 / リクエストタイムライン / リンク診断 / Recharts 可視化 / コスト推定 |
| **自動ヘルスチェック & フォールトトレランス** | Key ヘルスチェック（5分プローブ + 自動無効化） + Key 単位 Cooldown + サーキットブレーカー + 指数バックオフ |
| **ローカルファースト · プライバシーセキュア** | API Keys は Keychain のみに保存、定数時間比較でタイミング攻撃を防止、リクエストは Provider に直接送信 |
| **Combo 最適化ワークベンチ** | マイプラン / 最適化提案 / テンプレートセンター / 実行分析 — 4タブ、Combo ドラッグ＆ドロップ並べ替え |
| **Playground 即時テスト** | 内蔵チャットテスト UI、Combo セレクタ + 実行チェーン可視化 + ステップフォールバックステータス |
| **i18n** | 中国語 / 英語 / 日本語 / 韓国語 |

### サーバーコミュニティエディション（SeasAGI-Server）

| 機能 | 説明 |
|------------|-------------|
| 基本認証 | ログイン / 登録 / トークンリフレッシュ |
| 基本チャネル管理 | プラットフォームチャネル CRUD |
| 基本 Combo | ユーザーレベル Combo CRUD + 公式テンプレート取得 |
| 基本使用量統計 | ユーザー使用量、モデル/チャネル別グループ、タイムライン、エラー分布 |
| 基本テナント管理 | メンバー、招待リンク、カスタムチャネル同期、設定スナップショット |
| 基本管理 API | ユーザー / プラン / チャネル / Combo / Relay ゲートウェイ管理 |
| Relay 基本フォワーディング | リクエストパススルー、ヘルスチェック、レート制限、トレース |

---

## アーキテクチャ概要

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

## オープンソース / クローズドソース境界

| コンポーネント | ライセンス | オープンソース範囲 |
|-----------|---------|-------------------|
| `SeasAGI-Client/` | **GPL 3.0** | 完全オープンソース、ローカルゲートウェイ、Provider Executor、ルーティング戦略、セキュリティモジュール、フロントエンド UI を含む |
| `SeasAGI-Server/` | **AGPL 3.0** | コミュニティエディションオープンソース、基本コントロールプレーン + 基本リレー、セルフホスト可能 |
| `SeasAGI-Server-Enterprise/` | **クローズドソース（BSL / EULA）** | エンタープライズエディション、課金、マルチテナントガバナンス、Combo ガバナンス、管理ダッシュボードを搭載 |

---

## リポジトリ構造

```text
SeasAGI/
├── SeasAGI-Client/                オープンソースクライアント (GPL 3.0)
│   ├── src-app/                    Wails メインソース (Go + React)
│   ├── scripts/                    ビルドスクリプト
│   └── homebrew/                   Homebrew 配布フォーミュラ
├── SeasAGI-Server/                オープンソースサーバーコミュニティエディション (AGPL 3.0)
│   ├── platform-api/               コミュニティコントロールプレーン
│   ├── relay-gateway/              コミュニティリレーデータプレーン
│   ├── deploy/                     デプロイスクリプト + systemd + nginx
│   └── scripts/                    ビルドスクリプト
├── web-docs/                      公式サイト & ドキュメント
│   ├── src-web/                    静的サイト
│   └── ...
├── build-all.sh                   ワンクリックビルドスクリプト
├── SeasAGI.v5.md                  一ページサマリー
```

---

## クイックスタート

### クライアントダウンロード

[GitHub Releases](https://github.com/SeasX/SeasAGI/releases) からお使いのプラットフォーム向けの最新リリースをダウンロードしてください。

### チャネル設定

1. SeasAGI クライアントをインストールして起動
2. 「チャネル管理」で Provider API Key を追加
3. 任意の API クライアントの Base URL を `http://127.0.0.1:4318/v1` に変更
4. インテリジェントルーティングを体験！

### サーバーコミュニティエディションのセルフホスト

```bash
# ビルド
bash SeasAGI-Server/scripts/build.sh

# Linux サーバーにインストール（root 権限必要）
sudo bash SeasAGI-Server/deploy/install.sh

# 環境変数を設定
vi /etc/seasagi/platform-api.env
# JWT_SECRET, JWT_REFRESH_SECRET, ADMIN_SECRET を設定

# サービスを起動
sudo systemctl enable --now seasagi-platform-api
sudo systemctl enable --now seasagi-relay-gateway
```

---

## ビルド

### ワンクリックビルド（全エディション）

```bash
# クライアント + サーバーコミュニティ + サーバーエンタープライズをビルド
bash ./build-all.sh

# マルチプラットフォームクライアント
bash ./build-all.sh --platform darwin/amd64,darwin/arm64,linux/amd64
```

### 個別ビルド

```bash
# クライアント
bash SeasAGI-Client/scripts/build.sh

# サーバーコミュニティエディション
bash SeasAGI-Server/scripts/build.sh

# サーバーエンタープライズエディション
bash SeasAGI-Server-Enterprise/scripts/build.sh
```

### ビルド成果物

| 成果物 | パス |
|----------|------|
| クライアントアプリ | `SeasAGI-Client/src-app/build/bin/` |
| クライアント DMG | `SeasAGI-Client/build/` |
| サーバーコミュニティバイナリ | `SeasAGI-Server/build/` |
| サーバーエンタープライズバイナリ | `SeasAGI-Server-Enterprise/build/` |

---

## 技術スタック

| レイヤー | 技術 |
|-------|------------|
| フロントエンド | React + TypeScript + Vite + Zustand + @dnd-kit + Zod + Recharts |
| デスクトップフレームワーク | Wails v2 (Go + WebView) |
| クライアントバックエンド | Go（ローカルゲートウェイ / ルーティングエンジン / コスト最適化 / Provider Executor） |
| サーバーフレームワーク | Go + Gin |
| データベース | SQLite |
| 決済 | Stripe（エンタープライズ） |
| デプロイ | systemd + nginx |
| クライアント配布 | Homebrew Cask / DMG / Binary |

---

## ドキュメント

- [一ページサマリー](SeasAGI.v5.md)
- [システムアーキテクチャ設計ドキュメント](web-docs/系统架构设计文档.v5.md)
- [オープンソース分析](开源分析.md)
- [クライアント README](SeasAGI-Client/README.md)
- [サーバーコミュニティ README](SeasAGI-Server/README.md)
- [サーバーエンタープライズ README](SeasAGI-Server-Enterprise/README.md)

---

## ライセンス

- `SeasAGI-Client/` — **GPL 3.0**、完全オープンソース
- `SeasAGI-Server/` — **AGPL 3.0**、コミュニティエディションオープンソース セルフホスト可能
- `SeasAGI-Server-Enterprise/` — **クローズドソース（BSL / EULA）**、商用デプロイ向け

---

## コミュニティ

- [GitHub Issues](https://github.com/SeasX/SeasAGI/issues) — バグ報告 / 提案送信
- [GitHub Releases](https://github.com/SeasX/SeasAGI/releases) — 最新バージョンをダウンロード

---
