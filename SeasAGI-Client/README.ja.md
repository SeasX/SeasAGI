[🇬🇧 English](README.md) | [🇨🇳 中文](README.zh-CN.md) | [🇯🇵 日本語](README.ja.md) | [🇰🇷 한국어](README.ko.md)

---

# SeasAGI Client

[![Build status](https://ci.appveyor.com/api/projects/status/github/SeasX/SeasAGI?svg=true)](https://ci.appveyor.com/project/SeasX/SeasAGI)
[![macOS](https://img.shields.io/badge/platform-macOS-blue)](https://github.com/SeasX/SeasAGI)
[![Linux](https://img.shields.io/badge/platform-Linux-blue)](https://github.com/SeasX/SeasAGI)
[![Windows](https://img.shields.io/badge/platform-Windows-blue)](https://github.com/SeasX/SeasAGI)
[![License](https://img.shields.io/badge/license-GPL%20v3-green)](LICENSE)

クライアントサブプロジェクト。デスクトップ版のメインソースコードとクライアントビルドスクリプトを保持します。

## 目次

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

## 責務

- `src-app`：Wails v2 デスクトップクライアントのメインソースコード
- `src-app/frontend`：React + Vite + TypeScript フロントエンド（@dnd-kit ドラッグ＆ドロップ + Zod バリデーション）
- `src-app/internal`：ローカルゲートウェイ、ルーティング、OAuth、ローカルアクセストークン、ログ、最適化、使用量などの Go モジュール
- `scripts/build.sh`：クライアントビルドエントリポイント、実行中の OS に応じて自動振り分け
- `scripts/build-macos.sh`：macOS ビルド + DMG パッケージング
- `scripts/build-linux.sh`：Linux（Ubuntu/CentOS）ビルド
- `scripts/build-windows.sh`：Windows ビルド
- `scripts/dev.sh`：Wails ローカル開発エントリポイント
- `homebrew/seasagi.rb`：macOS Homebrew 配布用 Formula

## v5 コア機能

### コンボ最適化ワークベンチ（統一エントリ）

左側のナビゲーションは「コンボ最適化」の単一エントリに統合されました。ワークベンチ内は 4 つのタブに分かれています：

- **マイコンボ**：ローカル/公式/チームの 3 階層コンボビュー + ソースタグ + クラウド同期（Fetch/Push/Update/Delete）
- **最適化提案**：スマート最適化によるマルチステップフォールバック提案 + 構造差分プレビュー + ワンクリックでコンボとして適用
- **テンプレートセンター**：公式/チームテンプレートの検索、リフレッシュ、適用
- **実行分析**：コンボヒット率/フォールバック率/平均試行回数の分析パネル

### モデルレベルコンボ

- コンボの本質は「障害フォールバックオーケストレーション」：メインモデル → バックアップモデル → ラストリゾートモデル（primary/backup/last_resort の 3 ステップロールセマンティック）
- ステップチェーン編集 + ドラッグ＆ドロップ並べ替え（@dnd-kit）+ ステップ単位の channel_id + model バインディング
- Fallback / Round-Robin 戦略 + Sticky Uses 設定
- クラウド同期完全対応：`FetchCloudCombos` / `PushCloudCombo` / `UpdateCloudCombo` / `DeleteCloudCombo`
- ローカルマイグレーター `migrateModelCombos()` が旧データを自動的に統一スキーマ（combo_id/logical_name/display_name/status/source/version）に補完
- Playground 即時テスト：コンボセレクター + 実行チェーン可視化 + ステップフォールバック状態表示（step_role バッジ）

### スマート最適化とコンボ連携

- オプティマイザーがマルチステップフォールバック提案（primary/backup/last_resort）をサポート
- `PreviewComboOptimization` で構造差分プレビュー（現在のステップ vs 提案ステップ）を実現
- `ApplyComboOptimization` でコンボの新規作成/上書き更新をサポート
- `ApplyRecommendation` でワンクリックコンボ生成をサポート
- 推奨を明示的に区別：モデル置換提案 / フォールバックチェーン拡張提案 / 実行パラメータ提案

### ルーティングと戦略

- Fallback / Round-Robin / Sticky の 3 つの基本戦略
- コンボステップチェーン（明示的な channel + model の組み合わせ）
- セッションスティッキールーティング：最初の user message の SHA1 ハッシュに基づく session→step マッピング、30分 TTL
- 動的 429 ペナルティダウングレード：PenaltyManager、429 で +3（上限 10）、成功で -1、2分で自然減衰
- Fallback ソートプリセット：intelligence / speed / budget のワンクリック並べ替え
- コンボステップのドラッグ＆ドロップ並べ替え（@dnd-kit）

### ステップ内 Provider/Channel 候補プール（OpenRouter からの成果）

- 各 Step が単一固定チャンネルだけでなく、複数の候補を保持するプールをサポート
- ステップ内ソートルール：安定性/コスト/レイテンシ/スループットに基づき候補を自動並べ替え
- リクエストレベルの動的制約：`max_price`、`max_latency_ms`、`data_policy`、`allow_cross_provider_fallback`
- ツール呼び出し（tool-calling）リクエストの独立したルーティング戦略（tool success rate が高い provider を優先）
- クイック戦略エイリアス：安定優先 / コスト優先 / 速度優先 / ツール優先
- タスクタイプ認識（task_profile）：chat / tools / json / long_context / batch_low_cost の差異化実行

### Provider とフォールトトレランス

- 9 種類の Provider 実行エンジン：OpenAI / Azure / Anthropic / Gemini / Ollama / DeepSeek / Grok / Vertex / プラットフォームリレー
- 6 種類のフォーマット変換：OpenAI Chat / OpenAI Responses / Anthropic / Gemini / Vertex AI / Passthrough
- マルチキーローテーション + 指数バックオフ + サーキットブレーカー + 7 つのエラールール
- Key ヘルスチェック：HealthChecker、5分間隔での定期生存確認、連続 3 回失敗で自動的に unhealthy にマーク
- Key レベルクールダウン：CooldownManager、per-key 120秒 TTL、429 時にクールダウン設定
- Provider レベルのヘルススコアが実行時ソート判定に参加
- エンタープライズ BYOK / プラットフォーム共有容量の 2 層フォールバック戦略
- マルチモーダルコンテンツのフラット化：text-only の array content を自動的に string に統合

### コンボ実行時計測と指標

- `ComboRouteMetrics`：リクエスト回数 / フォールバック回数 / Step1 ヒット率 / 最終ステップヒット率
- `step_role` が RouteStep ログに伝播（primary/backup/last_resort）
- `GetComboRouteMetrics` がフロントエンドに公開済み
- `ExecAnalysisPage` でヒット率概要棒グラフ + 平均試行回数 + コンボ詳細パネルを表示

### セキュリティ

- Keychain 統合による安全なストレージ
- 定数時間 Key 比較（crypto/subtle.ConstantTimeCompare）
- Zod フロントエンドフォームバリデーション
- トンネルリモートアクセス（Cloudflare Tunnel + Tailscale Funnel）
- OAuth 2.0 PKCE（4 Provider + Token 自動リフレッシュ）

### MITM プロキシ — ワンクリック API ハイジャック

- **ワンクリック切替**：設定ページ → MITM タブでワンクリック起動/停止、手動証明書設定不要
- **自動 CA 信頼**：ルート CA を自動生成し macOS/Linux/Windows のシステム信頼チェーンにインストール、ドメインごとに動的に証明書を発行（23h TTL キャッシュ）
- **システムプロキシ統合**：OS レベルの HTTP/HTTPS プロキシを自動設定し、マッチする全トラフィックを透過的にハイジャック
- **ドメインホワイトリスト**：ハイジャック対象の API ドメインを設定可能（デフォルト：OpenAI、Anthropic、Gemini、DeepSeek、Grok、OpenRouter）、実行時に動的に追加/削除
- **接続性チェック**：ドメイン単位の「テスト」ボタンでハイジャック状態、到達性、レイテンシをワンクリック検証
- **ローカルゲートウェイルーティング**：ハイジャックされた HTTPS トラフィックはローカル SeasAGI ゲートウェイに透過転送され、統合ルーティング、最適化、可観測性を実現
- **リアルタイムハイジャックログ**：直近 20 件のハイジャックリクエストをリアルタイムテーブルで表示（メソッド、ドメイン、パス、ステータスコード、レイテンシ）、3 秒ごとに自動更新
- **CLI 互換ヒント**：ユーザーの Shell（bash/zsh/fish/PowerShell/cmd）を自動検出し、コピー＆ペースト可能な `export` / `unset` コマンドを提供、システムプロキシを読まないターミナルツールも対応
- **クラッシュリカバリ**：再起動時に残留システムプロキシを自動クリーンアップ、ヘルスプローブ goroutine がプロキシ異常を検出すると自動停止
- **パススルー安全性**：非マッチングドメインは透過的トンネル転送、60 秒タイムアウト保護で通常のブラウジングに影響なし

#### 既知の制限（ハイジャック対象外）

ワンクリックハイジャックはシステムプロキシ（TCP 上の HTTP/HTTPS）とローカル CA に基づいています。以下のトラフィックは客観的な盲区であり、監視・治理の対象になりません：

- **HTTP/3（QUIC / UDP 443）**：システムプロキシは TCP のみを扱うため、UDP ベースのトラフィックはハイジャックされません
- **h2 / gRPC 長時間接続**：標準 HTTP/HTTPS リクエストのみ解析・治理し、gRPC などのバイナリ長時間接続は書き換えられません
- **証明書ピンニング（Certificate Pinning）クライアント**：内部で証明書検証を行うアプリはローカル CA を拒否し、MITM を回避します
- **システムプロキシを読まないプロセス**：独自ネットワークスタックや明示的プロキシを持つ CLI/ツールは、環境変数の手動設定が必要です（「CLI 互換ヒント」参照）
- **ハイジャックドメインリスト外のトラフィック**：リスト内のドメインのみローカルゲートウェイへ転送されます

> WebSocket（ws/wss）トラフィックは同じゲートウェイメインパスを再利用し、正常にハイジャック・課金されます。

### エクスペリエンス

- Playground 即時テストページ
- RTK Token 圧縮（9 種類の出力タイプを自動検出）
- Caveman 簡略出力（4 スタイル）
- Reasoning Content インジェクション
- MCP ツール重複排除
- i18n（中国語/英語/日本語/韓国語）

## バージョン番号管理

バージョン番号は単一のファイルで一元管理され、ビルド時にフロントエンドに注入されます。実行時にこのファイルへの依存はありません。

### バージョン番号のソース

[`scripts/version.txt`](file:///Users/Neeke/data/www/SeasAGI/SeasAGI/SeasAGI-Client/scripts/version.txt) が唯一のバージョン番号定義ファイルです：

```
0.1.5
```

- バージョン番号の更新はこのファイルを変更するだけで完了します
- 環境変数 `SEASAGI_VERSION` で一時的に上書き可能です（例：`SEASAGI_VERSION=1.0.0 bash build-all.sh`）

### ビルド時注入フロー

1. 各プラットフォームのビルドスクリプト（`build-macos.sh` / `build-linux.sh` / `build-windows.sh`）起動時に `scripts/version.txt` を読み込み
2. バージョン番号を `export VITE_APP_VERSION="$VERSION"` でフロントエンドのビルド環境に注入
3. Vite ビルド時に `import.meta.env.VITE_APP_VERSION` が静的値としてコンパイルされ、フロントエンド成果物に書き込まれる
4. Wails がフロントエンド成果物をデスクトップクライアントのバイナリにパッケージング

### フロントエンド表示

クライアントインターフェース左上の Logo 右側にバージョン番号バッジを表示：

```
[icon] SeasAGI  v0.1.5
```

実装箇所：[`Layout.tsx`](file:///Users/Neeke/data/www/SeasAGI/SeasAGI/SeasAGI-Client/src-app/frontend/src/components/Layout.tsx) 内で `import.meta.env.VITE_APP_VERSION` を使用してレンダリング。

### ビルド後の独立性

バージョン番号はビルド時に静的値としてフロントエンド成果物にコンパイルされて埋め込まれるため、`scripts/version.txt` はクライアントにバンドルされません。クライアントの実行はこのファイルから完全に独立しています。

## よく使うコマンド

```bash
# クライアント開発モードを起動
bash SeasAGI-Client/scripts/dev.sh

# 現在のプラットフォーム向けにクライアントをコンパイル
bash SeasAGI-Client/scripts/build.sh

# 指定プラットフォーム向けにコンパイル
bash SeasAGI-Client/scripts/build-macos.sh   # macOS
bash SeasAGI-Client/scripts/build-linux.sh    # Linux (Ubuntu/CentOS)
bash SeasAGI-Client/scripts/build-windows.sh  # Windows
```

## ビルド成果物

- `SeasAGI-Client/src-app/build/bin/SeasAGI.app`
- `SeasAGI-Client/src-app/build/bin/SeasAGI.exe`
- `SeasAGI-Client/src-app/build/bin/SeasAGI`
- `SeasAGI-Client/build/SeasAGI-<version>.dmg`（macOS のみ）

## Linux 依存パッケージ

Ubuntu/Debian:
```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev build-essential
```

CentOS/RHEL:
```bash
sudo yum install gtk3-devel webkit2gtk3-devel
```