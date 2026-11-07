> [English](README.md) · [日本語](README.ja.md) · **한국어** · [简体中文](README.zh.md)

# SeasAGI Server Community

[![Build status](https://ci.appveyor.com/api/projects/status/github/SeasX/SeasAGI?svg=true)](https://ci.appveyor.com/project/SeasX/SeasAGI)
[![macOS](https://img.shields.io/badge/platform-macOS-blue)](https://github.com/SeasX/SeasAGI)
[![Linux](https://img.shields.io/badge/platform-Linux-blue)](https://github.com/SeasX/SeasAGI)
[![Windows](https://img.shields.io/badge/platform-Windows-blue)](https://github.com/SeasX/SeasAGI)
[![License](https://img.shields.io/badge/license-AGPL%20v3-green)](LICENSE)

커뮤니티 에디션 클라우드 하위 프로젝트입니다. 오픈소스 플랫폼 컨트롤 플레인, 플랫폼 릴레이 게이트웨이 및 내장형 경량 관리자 대시보드를 포함합니다. 엔터프라이즈 거버넌스와 결제는 포함되지 않습니다.

## 디렉토리 구조

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
│   └── src/              # 관리자 대시보드 프론트엔드 (6개 페이지)
├── platform-api/
│   └── cmd/admin_dist/   # 관리자 대시보드 SPA를 platform-api 바이너리에 내장
└── relay-gateway/
```

## 역할

- `platform-api`: 커뮤니티 클라우드 컨트롤 플레인 — 기본 인증, 채널 관리, Provider 리소스 풀, 모델 카탈로그, 기본 Combo CRUD, 기본 사용량 통계, SQLite 백업/복원, 테넌트 관리 API
- `relay-gateway`: 커뮤니티 릴레이 데이터 플레인 — 릴레이, 멀티모달 전달, 모델 카탈로그, 헬스 체크, 트레이스, 속도 제한, Prometheus 메트릭, 운영 인터페이스
- `src-admin`: 커뮤니티 관리자 대시보드 프론트엔드 (6개 페이지), platform-api 바이너리에 내장되어 `/admin`에서 제공
- `deploy`: 커뮤니티 배포, 설치, systemd, nginx, 운영 스크립트
- `scripts/build.sh`: 커뮤니티 통합 빌드 진입점

## 커뮤니티 에디션 범위

### 포함 기능

- **기본 인증**: 로그인 / 회원가입 / 토큰 갱신 (bcrypt + JWT HS256 이중 키)
- **채널 관리**: 플랫폼 채널 CRUD, API Key는 AES-256-GCM으로 암호화 저장
- **Provider 리소스 풀**: 채널 단위 API Key / 리전 / 환경 리소스, 헬스 + 가중치 + 우선순위 기반 자동 최적 선택 (`ResolveResource`)
- **모델 카탈로그**: `data/model-catalog.yaml`을 파싱하여 `model_catalog` 테이블에 등록, `/models`와 `/models/:name`으로 조회
- **기본 Combo**: 사용자 레벨 Combo CRUD + 공식 템플릿 가져오기
- **기본 사용량**: 사용자 사용량, 모델/채널별 그룹화, 타임라인, 오류 분포, 최근 오류
- **기본 테넌트 관리**: 멤버, 초대 링크, 사용자 정의 채널 동기화, 정책, 템플릿, 설정 스냅샷
- **기본 Admin API**: 사용자 / 채널 / Combo / Relay Gateway 기본 관리
- **SQLite 온라인 백업**: `VACUUM INTO` 백업, SHA-256 검증, 복원, 30일 보관
- **무료 채널 시드**: `GET /free-channels`로 23개 무료 프로바이더 반환 (OpenCode, DuckDuckGo, DeepSeek, Tencent 원바오, Doubao, iFlytek, Coze, AI Horde 등)
- **내장 관리자 대시보드**: `/admin` SPA, 6개 페이지 — 대시보드 / 사용자 / 사용량 / 릴레이 게이트웨이 / 채널 / Combo
- **i18n**: zh-CN / en / ja / ko 오류 메시지 (`?lang=` 또는 `Accept-Language` 협상)
- **보안**: AES-256-GCM API Key 암호화, 시작 시 키 검증 (`secpolicy`: 개발 모드는 임시 키 자동 생성, 프로덕션 모드는 미설정/약한 키면 즉시 종료). 요청 봉투(envelope)는 전송 기밀성만 제공하며(TLS가 동일한 목표를 이미 충족) 인증 경계가 아닙니다. 기기 바인딩은 표시/감사 단서일 뿐이며 `device_id`는 클라이언트가 생성하므로 시트나 체험 제한에 사용할 수 없습니다. 권한 부여, 과금, 할당량, 모델 범위, 정산 주기는 항상 서버가 결정하며 클라이언트 제공 필드는 신뢰할 수 없는 입력으로 취급합니다.
- **릴레이 기본 전달**: 요청 투과 전달, 헬스 체크, 속도 제한, 트레이스, 멀티모달 전달

### 제외 기능

다음 기능은 엔터프라이즈 프로젝트 `SeasAGI-Server-Enterprise/`로 이전되었습니다:

- 플랜 기반 상업 기능 게이팅
- 멀티 테넌트 청구 / 주문 / 결제 / 인보이스
- Combo 거버넌스 (가시성 정책, 배포 승인)
- Combo 메트릭 관측 및 Provider 상태 지표
- 엔터프라이즈 BYOK 이중 계층 폴백 전략
- 엔터프라이즈 SSO / SCIM / 규정 준수 / 감사 강화 / SLA / 라이선스
- 엔터프라이즈 관리자 대시보드 페이지 (감사, 리스크, 알림, Webhooks, 플랜, 충전, Token 거래/결제)

## 자주 사용하는 명령어

```bash
# 커뮤니티 클라우드 프로젝트 빌드
bash SeasAGI-Server/scripts/build.sh

# Linux 서버에 설치
sudo bash SeasAGI-Server/deploy/install.sh
```

## 배포 단계 (Linux + systemd)

**1. 시크릿 생성**

```bash
openssl rand -hex 32   # JWT_SECRET
openssl rand -hex 32   # JWT_REFRESH_SECRET
openssl rand -hex 32   # ADMIN_SECRET
openssl rand -hex 32   # SEASAGI_DATA_KEY (또는 32바이트 임의 문자열)
```

`JWT_SECRET`과 `JWT_REFRESH_SECRET`은 두 env 파일에서 반드시 동일해야 합니다. `ADMIN_SECRET`은 필수이며, 없으면 `/api/v1/admin/*`를 사용할 수 없습니다.

**2. `/etc/seasagi/*.env` 작성**

```bash
sudo vi /etc/seasagi/platform-api.env
# JWT_SECRET, JWT_REFRESH_SECRET, ADMIN_SECRET, SEASAGI_DATA_KEY
# CORS_ALLOWED_ORIGINS=https://your-domain.example
# MAX_REQUEST_BODY_BYTES=8388608
# TRACE_RETENTION_DAYS=30
# TRACE_PURGE_INTERVAL_HOURS=6

sudo vi /etc/seasagi/relay-gateway.env
# 동일한 JWT_SECRET / JWT_REFRESH_SECRET / ADMIN_SECRET
# RELAY_MAX_REQUEST_BODY_BYTES=67108864
```

**3. 두 service 시작**

```bash
sudo systemctl enable --now seasagi-platform-api
sudo systemctl enable --now seasagi-relay-gateway
```

**4. 자체 점검**

```bash
curl -fsS http://127.0.0.1:9318/healthz
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9318/api/v1/admin/users   # 401
curl -s -D - -o /dev/null -X OPTIONS \
  -H 'Origin: https://evil.example' \
  -H 'Access-Control-Request-Method: POST' \
  http://127.0.0.1:9318/api/v1/auth/login | grep -i '^access-control-allow-origin'  # 비어 있음
```

**5. 업그레이드와 백업**

```bash
# 바이너리 교체 전 SQLite 백업
sudo systemctl stop seasagi-platform-api seasagi-relay-gateway
sudo mkdir -p /var/backups/seasagi
sudo sqlite3 /var/lib/seasagi/platform-api.db ".backup '/var/backups/seasagi/platform-api-$(date +%F).db'"
sudo sqlite3 /var/lib/seasagi/platform-api.db 'PRAGMA wal_checkpoint(TRUNCATE);'
# 바이너리 교체 후 서비스 재시작
sudo systemctl start seasagi-platform-api seasagi-relay-gateway
```

스키마 마이그레이션은 시작 시 자동 실행되며 멱등입니다. 데이터베이스를 수동으로 복사할 때는 `-wal` / `-shm` 사이드카 파일도 함께 복사하십시오. 온라인 백업 엔드포인트(`VACUUM INTO`)는 일관성 있는 스냅샷을 생성합니다.

## 빌드 산출물

- `SeasAGI-Server/build/platform-api` (관리자 대시보드 SPA 내장)
- `SeasAGI-Server/build/relay-gateway`

## 설정

모든 설정은 환경 변수를 통해 로드됩니다. `deploy/.env.example`을 참고하여 `.env` 파일을 생성하고 내보내십시오:

```bash
export $(grep -v '^#' deploy/.env.example | xargs)
```

프로덕션 모드(`GIN_MODE=release`)에서는 `JWT_SECRET`, `JWT_REFRESH_SECRET`, `ADMIN_SECRET`, `SEASAGI_DATA_KEY`가 없거나 `change-me*` 플레이스홀더로 남아 있으면 두 서비스 모두 시작을 거부합니다. 개발 모드(`GIN_MODE=debug` 또는 명시적 `SEASAGI_ALLOW_DEV_SECRETS=1`)에서는 임시 시크릿을 자동 생성합니다. 모든 `/api/v1/admin/*` 엔드포인트(내장 `/admin` SPA 포함)는 `Authorization: Bearer ${ADMIN_SECRET}`가 필요하며, 자격 증명이 없거나 틀리면 401을 반환합니다.

### 플랫폼 API 설정

| 변수 | 설명 | 기본값 |
|------|------|--------|
| `API_PORT` | 플랫폼 API 수신 포트 | `9318` |
| `DB_PATH` | SQLite 데이터베이스 경로 | `~/.seasagi/platform-api.db` |
| `JWT_SECRET` | JWT 서명 시크릿 (프로덕션 환경 필수) | — |
| `JWT_REFRESH_SECRET` | JWT 리프레시 토큰 시크릿 (프로덕션 환경 필수) | — |
| `ADMIN_SECRET` | Admin API 시크릿 | — |
| `CORS_ALLOWED_ORIGINS` | CORS 허용 오리진 (쉼표 구분·정확 일치, 프로덕션에서는 `*` 거부) | 루프백만 (`localhost` / `127.0.0.1` / `[::1]`) |
| `MAX_REQUEST_BODY_BYTES` | 요청 본문 최대 크기(바이트) | `8388608`(8MB) |
| `SEASAGI_DATA_KEY` | API Key용 AES-256-GCM 암호화 키 | — |
| `GIN_MODE` | Gin 모드 (`release` / `debug`) | — |
| `SEASAGI_LOCALES_DIR` | i18n 언어 파일 디렉토리 | `locales` |
| `BACKUP_DIR` | SQLite 백업 디렉토리 | `<db 디렉토리>/backups` |
| `SEASAGI_VERSION` | 배포 버전 표시 값 | `1.0.0` |
| `API_BASE_URL` | 서버 기본 URL (OAuth 리디렉션 폴백) | `http://localhost:9318` |
| `PUBLIC_BASE_URL` | 브라우저에서 접근 가능한 서버 URL (OAuth 인가·콜백) | `API_BASE_URL`로 폴백 |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | Google OAuth 자격 증명 | 비어 있음 (Google 로그인 숨김) |
| `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET` | GitHub OAuth 자격 증명 | 비어 있음 (GitHub 로그인 숨김) |
| `WEBHOOK_DISPATCHER_INTERVAL_SEC` | Webhook 디스패처 폴링 간격(초) | `5` |
| `WEBHOOK_MAX_RETRIES` | Webhook 최대 재시도 횟수 | `3` |

### 릴레이 게이트웨이 설정

| 변수 | 설명 | 기본값 |
|------|------|--------|
| `RELAY_PORT` | 릴레이 게이트웨이 포트 | `8318` |
| `RATE_LIMIT_RPM` | 전역 속도 제한 (회/분) | `60` |
| `HEALTH_CHECK_INTERVAL_SEC` | 채널 헬스 체크 간격 | `60` |
| `POLICY_CACHE_SEC` | 정책 캐시 갱신 간격 | `60` |
| `PLATFORM_API_URL` | 플랫폼 API URL (채널 스냅샷 동기화) | `http://127.0.0.1:9318` |
| `METRICS_TOKEN` | `/metrics` Bearer 토큰 (`ADMIN_SECRET`으로 폴백) | — |
| `CORS_ALLOWED_ORIGINS` | CORS 허용 오리진 (쉼표 구분·정확 일치, 프로덕션에서는 `*` 거부) | 루프백만 (`localhost` / `127.0.0.1` / `[::1]`) |
| `RELAY_MAX_REQUEST_BODY_BYTES` | 요청 본문 최대 크기(바이트, 릴레이 게이트웨이) | `67108864`(64MB) |
| `TRACE_RETENTION_DAYS` | Trace 보존 일수 (`<=0`이면 정리 비활성. platform-api와 relay-gateway 공통) | `30` |
| `TRACE_PURGE_INTERVAL_HOURS` | Trace 정리 간격(시간, `<1`은 `1`로 처리) | `6` |

## API 엔드포인트

### 플랫폼 API (`/api/v1`)

- `auth`: `POST /auth/login` / `POST /auth/register` / `POST /auth/refresh`
- `user`: `GET /user/profile` · `GET/PUT /user/optimization` (라우팅/헬스체크/쿨다운/sticky/preset 토글)
- `channel`: `GET /channels` · `GET /channels/:id` · `GET /free-channels` (23개 무료 프로바이더) — 사용자 측 읽기 전용, 자격 증명은 반환되지 않음
- `providerresource`: `GET/POST/PUT/DELETE /admin/channels/:id/resources[/:resource_id]` · `GET .../resources/resolve` (관리자 전용)
- `modelcatalog`: `GET /models` · `GET /models/:name`
- `device`: `POST /devices/bind` · `GET /devices` · `DELETE /devices/:id`
- `combo`: `GET/POST /combos` · `GET/PUT/DELETE /combos/:id` · `GET /combo-templates`
- `relay`: `GET /relay-gateways`
- `usage`: `GET /usage` · `/usage/models` · `/usage/error-distribution` · `/usage/timeline`
- `version`: `GET /version/check` · `GET /version/migrations`
- `admin`: `GET/DELETE /admin/users` · `GET/POST/PUT/DELETE /admin/channels` · `/admin/channels/:id/resources` · `GET /admin/usage/stats` · `GET /admin/usage/records` · `GET /admin/relay-gateways` · `GET/POST/DELETE /admin/sqlite/backups[/:id]` · `POST /admin/sqlite/backups/:id/restore` · `POST /admin/sqlite/backups/:id/verify` · `POST /admin/channels/seed` · `POST /admin/models/sync`
- 시스템: `GET /healthz` · `/admin/*` (내장 SPA)

### 릴레이 게이트웨이

- 공개: `GET /healthz` · `GET /metrics` (Prometheus, 토큰 보호) · `GET /ops/channels`
- 운영 (`/ops`): `GET /ops/overview` · `GET /ops/channels/health` · `GET /ops/traces[/:trace_id]` · `POST /ops/channels/:id/drain` · `POST /ops/channels/:id/restore` · `PUT /ops/channels/:id/read-only` · `PUT /ops/channels/:id/weight` · `PUT /ops/channels/:id/gray` · `GET /ops/alerts`
- 릴레이 (`/relay`, JWT 보호 + 속도 제한): `POST /relay/chat/completions` · `POST /relay/embeddings` · `POST /relay/images/generations` · `POST /relay/audio/speech` · `POST /relay/audio/transcriptions` · `GET /relay/models`

## 라이선스

커뮤니티 에디션은 `AGPL 3.0`으로 라이선스됩니다. 전체 엔터프라이즈 기능은 `SeasAGI-Server-Enterprise/`를 참조하십시오.
