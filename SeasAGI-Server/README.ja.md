> [English](README.md) · **日本語** · [한국어](README.ko.md) · [简体中文](README.zh.md)

# SeasAGI Server Community

[![Build status](https://ci.appveyor.com/api/projects/status/github/SeasX/SeasAGI?svg=true)](https://ci.appveyor.com/project/SeasX/SeasAGI)
[![macOS](https://img.shields.io/badge/platform-macOS-blue)](https://github.com/SeasX/SeasAGI)
[![Linux](https://img.shields.io/badge/platform-Linux-blue)](https://github.com/SeasX/SeasAGI)
[![Windows](https://img.shields.io/badge/platform-Windows-blue)](https://github.com/SeasX/SeasAGI)
[![License](https://img.shields.io/badge/license-AGPL%20v3-green)](LICENSE)

コミュニティ版クラウドサブプロジェクトです。オープンソースのプラットフォームコントロールプレーン、プラットフォーム中継ゲートウェイ、および組み込みの軽量管理画面を含みます。エンタープライズガバナンスと課金は含みません。

## ディレクトリ構造

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
│   └── src/              # 管理画面フロントエンド（7 ページ）
├── platform-api/
│   └── cmd/admin_dist/   # 管理画面 SPA を platform-api バイナリに埋め込み
└── relay-gateway/
```

## 責務

- `platform-api`: コミュニティ版クラウドコントロールプレーン — 基本認証、チャンネル管理、Provider リソースプール、モデルカタログ、基本 Combo CRUD、基本使用量統計、SQLite バックアップ/復元、テナント管理 API
- `relay-gateway`: コミュニティ版中継データプレーン — リレー、マルチモーダル転送、モデルカタログ、ヘルスチェック、トレース、レート制限、Prometheus メトリクス、運用インターフェース
- `src-admin`: コミュニティ版管理画面フロントエンド（7 ページ）、platform-api バイナリに埋め込まれ `/admin` で提供
- `deploy`: コミュニティ版デプロイ、インストール、systemd、nginx、運用スクリプト
- `scripts/build.sh`: コミュニティ版統一ビルドエントリポイント

## コミュニティ版の範囲

### 含まれる機能

- **基本認証**: ログイン / 登録 / トークン更新（bcrypt + JWT HS256 二重キー）
- **チャンネル管理**: プラットフォームチャンネルの CRUD、API Key は AES-256-GCM で暗号化保存
- **Provider リソースプール**: チャンネル単位の API Key / リージョン / 環境リソース、ヘルス + 重み + 優先度で自動最適選択（`ResolveResource`）
- **モデルカタログ**: `data/model-catalog.yaml` を解析して `model_catalog` テーブルに登録、`/models` と `/models/:name` で照会
- **基本 Combo**: ユーザーレベル Combo CRUD + 公式テンプレート取得
- **基本使用量**: ユーザー使用量、モデル/チャンネル別グループ、タイムライン、エラー分布、最近のエラー
- **基本テナント管理**: メンバー、招待リンク、カスタムチャンネル同期、ポリシー、テンプレート、設定スナップショット
- **基本 Admin API**: ユーザー / プラン / チャンネル / Combo / Relay Gateway の基本管理
- **SQLite オンラインバックアップ**: `VACUUM INTO` バックアップ、SHA-256 検証、復元、30 日保持
- **無料チャンネルシード**: `GET /free-channels` で 23 の無料プロバイダを返却（OpenCode、DuckDuckGo、DeepSeek、Tencent 元宝、豆包、iFlytek、Coze、AI Horde など）
- **組み込み管理画面**: `/admin` SPA、7 ページ — ダッシュボード / ユーザー / 使用量 / 中継ゲートウェイ / チャンネル / Combo / Token マーケット
- **i18n**: zh-CN / en / ja / ko のエラーメッセージ（`?lang=` または `Accept-Language` で交渉）
- **セキュリティ**: AES-256-GCM API Key 暗号化、起動時弱いキー検出（`secpolicy` がランダム JWT キーを自動生成、本番モードでは弱いキーで Fatal）
- **Relay 基本転送**: リクエスト透過転送、ヘルスチェック、レート制限、トレース、マルチモーダル転送

### 含まれない機能

以下の機能はエンタープライズプロジェクト `SeasAGI-Server-Enterprise/` に移動されました：

- プラン駆動の商用機能ゲーティング
- マルチテナント請求 / 注文 / 支払い / 請求書
- Combo ガバナンス（可視性ポリシー、デプロイ承認）
- Combo メトリクス観測 & Provider 健全性指標
- エンタープライズ BYOK 二層フォールバック戦略
- エンタープライズ SSO / SCIM / コンプライアンス / 監査強化 / SLA / ライセンス
- エンタープライズ管理画面ページ（監査、リスク、アラート、Webhooks、プラン、チャージ、Token 取引/決済）

## よく使うコマンド

```bash
# コミュニティ版クラウドプロジェクトをビルド
bash SeasAGI-Server/scripts/build.sh

# Linux サーバーにインストール
sudo bash SeasAGI-Server/deploy/install.sh
```

## ビルド成果物

- `SeasAGI-Server/build/platform-api`（管理画面 SPA 埋め込み）
- `SeasAGI-Server/build/relay-gateway`

## 設定

すべての設定は環境変数から読み込まれます。`deploy/.env.example` を参考に `.env` ファイルを作成してエクスポートしてください：

```bash
export $(grep -v '^#' deploy/.env.example | xargs)
```

### プラットフォーム API 設定

| 変数 | 説明 | デフォルト値 |
|------|------|-------------|
| `API_PORT` | プラットフォーム API のリッスンポート | `9318` |
| `DB_PATH` | SQLite データベースのパス | `~/.seasagi/platform-api.db` |
| `JWT_SECRET` | JWT 署名シークレット（本番環境では必須） | — |
| `JWT_REFRESH_SECRET` | JWT リフレッシュトークンシークレット（本番環境では必須） | — |
| `ADMIN_SECRET` | Admin API シークレット | — |
| `CORS_ALLOW_ORIGIN` | CORS 許可オリジン | `*` |
| `SEASAGI_DATA_KEY` | API Key 用 AES-256-GCM 暗号化キー | — |
| `GIN_MODE` | Gin モード（`release` / `debug`） | — |
| `SEASAGI_LOCALES_DIR` | i18n 言語ファイルディレクトリ | `locales` |
| `BACKUP_DIR` | SQLite バックアップディレクトリ | `<db ディレクトリ>/backups` |

### 中継ゲートウェイ設定

| 変数 | 説明 | デフォルト値 |
|------|------|-------------|
| `RELAY_PORT` | 中継ゲートウェイのポート | `8318` |
| `RATE_LIMIT_RPM` | 全局限流レート（回/分） | `60` |
| `HEALTH_CHECK_INTERVAL_SEC` | チャンネルヘルスチェック間隔 | `60` |
| `POLICY_CACHE_SEC` | ポリシーキャッシュ更新間隔 | `60` |
| `PLATFORM_API_URL` | プラットフォーム API の URL（チャンネルスナップショット同期） | `http://127.0.0.1:9318` |
| `METRICS_TOKEN` | `/metrics` Bearer トークン（`ADMIN_SECRET` にフォールバック） | — |
| `CORS_ALLOW_ORIGIN` | CORS 許可オリジン | `*` |

## API エンドポイント

### プラットフォーム API（`/api/v1`）

- `auth`: `POST /auth/login` / `POST /auth/register` / `POST /auth/refresh`
- `user`: `GET/PUT /user/profile` · `GET/PUT /user/optimization`（ルーティング/ヘルスチェック/クールダウン/sticky/preset トグル）
- `channel`: `GET/POST/PUT/DELETE /channels` · `GET /free-channels`（23 の無料プロバイダ）
- `providerresource`: `GET/POST/PUT/DELETE /channels/:id/resources[/:resource_id]` · `GET .../resources/resolve`
- `modelcatalog`: `GET /models` · `GET /models/:name`
- `device`: `POST /devices/bind` · `GET /devices` · `DELETE /devices/:id`
- `combo`: `GET/POST /combos` · `GET/PUT/DELETE /combos/:id` · `GET /combo-templates`
- `relay`: `GET /relay-gateways`
- `usage`: `GET /usage` · `/usage/models` · `/usage/error-distribution` · `/usage/timeline`
- `version`: `GET /version/check` · `GET /version/migrations`
- `admin`: `GET/DELETE /admin/users` · `GET /admin/usage/stats` · `GET /admin/usage/records` · `GET /admin/relay-gateways` · `GET/POST/DELETE /admin/sqlite/backups[/:id]` · `POST /admin/sqlite/backups/:id/restore` · `POST /admin/sqlite/backups/:id/verify` · `POST /admin/channels/seed` · `POST /admin/models/sync`
- システム: `GET /healthz` · `/admin/*`（組み込み SPA）

### 中継ゲートウェイ

- 公開: `GET /healthz` · `GET /metrics`（Prometheus、トークン保護） · `GET /ops/channels`
- 運用（`/ops`）: `GET /ops/overview` · `GET /ops/channels/health` · `GET /ops/traces[/:trace_id]` · `POST /ops/channels/:id/drain` · `POST /ops/channels/:id/restore` · `PUT /ops/channels/:id/read-only` · `PUT /ops/channels/:id/weight` · `PUT /ops/channels/:id/gray` · `GET /ops/alerts`
- 中継（`/relay`、JWT 保護 + レート制限）: `POST /relay/chat/completions` · `POST /relay/embeddings` · `POST /relay/images/generations` · `POST /relay/audio/speech` · `POST /relay/audio/transcriptions` · `GET /relay/models`

## ライセンス

コミュニティ版は `AGPL 3.0` でライセンスされます。完全なエンタープライズ機能については `SeasAGI-Server-Enterprise/` を参照してください。
