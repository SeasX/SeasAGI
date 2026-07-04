[🇬🇧 English](README.md) | [🇨🇳 中文](README.zh-CN.md) | [🇯🇵 日本語](README.ja.md) | [🇰🇷 한국어](README.ko.md)

---

# SeasAGI

**로컬 LLM API 지능형 게이트웨이** — 오픈소스 클라이언트 + 서버 오픈코어 모델, 다중 LLM Provider 채널 통합 관리, 지능형 라우팅, 세션 고착성, 비용 최적화, 전체 체인 관측 가능성 및 팀 협업 거버넌스 제공.

---

## 개요

SeasAGI는 세 개의 하위 프로젝트로 구성된 계층형 AI 게이트웨이 플랫폼입니다:

| 하위 프로젝트 | 라이선스 | 설명 |
|-------------|---------|-------------|
| `SeasAGI-Client/` | **GPL 3.0** 완전 오픈소스 | 네이티브 데스크톱 클라이언트 (Wails + Go + React), 로컬 통합 게이트웨이, API Keys 절대 업로드 안 함 |
| `SeasAGI-Server/` | **AGPL 3.0** 오픈소스 자체 호스팅 가능 | 커뮤니티 에디션 클라우드 제어 평면, 기본 인증 / Combo CRUD / 기본 사용량 / 기본 릴레이 포워딩 |
| `SeasAGI-Server-Enterprise/` | 폐쇄소스 | 엔터프라이즈 에디션 클라우드, Stripe 결제 / 멀티 테넌트 거버넌스 / Combo 거버넌스 승인 / 관리자 대시보드 포함. [엔터프라이즈 README](SeasAGI-Server-Enterprise/README.md) 참조 |

클라이언트는 **독립 실행**이 가능합니다 — 서버 없이도 모든 로컬 게이트웨이 기능이 작동합니다. 클라우드는 원격 가속 채널, 사용량 동기화, 팀 협업 및 엔터프라이즈 거버넌스를 제공하는 선택적 부가 서비스입니다.

---

## 핵심 기능

### 클라이언트 (SeasAGI-Client)

| 기능 | 설명 |
|------------|-------------|
| **통합 API 게이트웨이** | 로컬 `localhost:4318`, OpenAI 호환, 모든 API 클라이언트의 Base URL을 원활하게 교체 가능 |
| **적응형 지능 라우팅** | Fallback / Round-Robin / Sticky 전략, Combo 단계 체인 오케스트레이션 (주 → 백업 → 최후 수단) |
| **세션 고착성** | SHA1 해시 기반 세션→단계 매핑, 30분 TTL, 대화 중 모델 전환 방지 |
| **동적 429 페널티 저하** | PenaltyManager, 429 +3 (상한 10), 성공 -1, 2분 자연 감쇠 |
| **비용 최적화 엔진** | 모델별/채널별 그룹 분석 + 작업 유형 차별화 제안 + 4가지 빠른 전략 원클릭 프리셋 |
| **8개 실행자 + 6개 변환기** | OpenAI / Azure / Anthropic / Gemini / Ollama / DeepSeek / Grok / Relay + 자동 프로토콜 적응 |
| **전체 체인 관측 가능성** | 오류 분류 분포 / 요청 타임라인 / 링크 진단 / Recharts 시각화 / 비용 추정 |
| **자동 상태 확인 & 장애 허용** | Key 상태 확인 (5분 프로브 + 자동 비활성화) + Key별 Cooldown + 서킷 브레이커 + 지수 백오프 |
| **로컬 우선 · 프라이버시 보안** | API Keys는 Keychain에만 저장, 상수 시간 비교로 타이밍 공격 방지, 요청은 Provider로 직접 전송 |
| **Combo 최적화 워크벤치** | 내 계획 / 최적화 제안 / 템플릿 센터 / 실행 분석 — 4개 탭, Combo 드래그앤드롱 정렬 |
| **Playground 즉시 테스트** | 내장 채팅 테스트 UI, Combo 선택기 + 실행 체인 시각화 + 단계 Fallback 상태 |
| **i18n** | 중국어 / 영어 / 일본어 / 한국어 |

### 서버 커뮤니티 에디션 (SeasAGI-Server)

| 기능 | 설명 |
|------------|-------------|
| 기본 인증 | 로그인 / 회원가입 / 토큰 갱신 |
| 기본 채널 관리 | 플랫폼 채널 CRUD |
| 기본 Combo | 사용자 수준 Combo CRUD + 공식 템플릿 가져오기 |
| 기본 사용량 통계 | 사용자 사용량, 모델별/채널별 그룹화, 타임라인, 오류 분포 |
| 기본 테넌트 관리 | 멤버, 초대 링크, 커스텀 채널 동기화, 설정 스냅샷 |
| 기본 관리자 API | 사용자 / 계획 / 채널 / Combo / 릴레이 게이트웨이 관리 |
| 릴레이 기본 포워딩 | 요청 통과, 상태 확인, 속도 제한, 추적 |

---

## 아키텍처 개요

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

## 오픈소스 / 폐쇄소스 경계

| 구성 요소 | 라이선스 | 오픈소스 범위 |
|-----------|---------|-------------------|
| `SeasAGI-Client/` | **GPL 3.0** | 완전 오픈소스, 로컬 게이트웨이, Provider 실행자, 라우팅 전략, 보안 모듈, 프론트엔드 UI 포함 |
| `SeasAGI-Server/` | **AGPL 3.0** | 커뮤니티 에디션 오픈소스, 기본 제어 평면 + 기본 릴레이, 자체 호스팅 가능 |
| `SeasAGI-Server-Enterprise/` | **폐쇄소스 (BSL / EULA)** | 엔터프라이즈 에디션, 결제, 멀티 테넌트 거버넌스, Combo 거버넌스, 관리자 대시보드 포함 |

---

## 저장소 구조

```text
SeasAGI/
├── SeasAGI-Client/                오픈소스 클라이언트 (GPL 3.0)
│   ├── src-app/                    Wails 메인 소스 (Go + React)
│   ├── scripts/                    빌드 스크립트
│   └── homebrew/                   Homebrew 배포 포뮬러
├── SeasAGI-Server/                오픈소스 서버 커뮤니티 에디션 (AGPL 3.0)
│   ├── platform-api/               커뮤니티 제어 평면
│   ├── relay-gateway/              커뮤니티 릴레이 데이터 평면
│   ├── deploy/                     배포 스크립트 + systemd + nginx
│   └── scripts/                    빌드 스크립트
├── web-docs/                      공식 웹사이트 및 문서
│   ├── src-web/                    정적 웹사이트
│   └── ...
├── build-all.sh                   원클릭 빌드 스크립트
├── SeasAGI.v5.md                  한 페이지 요약
```

---

## 빠른 시작

### 클라이언트 다운로드

[GitHub Releases](https://github.com/SeasX/SeasAGI/releases)에서 플랫폼에 맞는 최신 릴리스를 다운로드하세요.

### 채널 구성

1. SeasAGI 클라이언트를 설치하고 실행합니다
2. "채널 관리"에서 Provider API Key를 추가합니다
3. 임의의 API 클라이언트의 Base URL을 `http://127.0.0.1:4318/v1`로 변경합니다
4. 지능형 라우팅을 즐기세요!

### 서버 커뮤니티 에디션 자체 호스팅

```bash
# 빌드
bash SeasAGI-Server/scripts/build.sh

# Linux 서버에 설치 (root 권한 필요)
sudo bash SeasAGI-Server/deploy/install.sh

# 환경 변수 구성
vi /etc/seasagi/platform-api.env
# JWT_SECRET, JWT_REFRESH_SECRET, ADMIN_SECRET 설정

# 서비스 시작
sudo systemctl enable --now seasagi-platform-api
sudo systemctl enable --now seasagi-relay-gateway
```

---

## 빌드

### 원클릭 빌드 (모든 에디션)

```bash
# 클라이언트 + 서버 커뮤니티 + 서버 엔터프라이즈 빌드
bash ./build-all.sh

# 멀티 플랫폼 클라이언트
bash ./build-all.sh --platform darwin/amd64,darwin/arm64,linux/amd64
```

### 개별 빌드

```bash
# 클라이언트
bash SeasAGI-Client/scripts/build.sh

# 서버 커뮤니티 에디션
bash SeasAGI-Server/scripts/build.sh

# 서버 엔터프라이즈 에디션
bash SeasAGI-Server-Enterprise/scripts/build.sh
```

### 빌드 산출물

| 산출물 | 경로 |
|----------|------|
| 클라이언트 앱 | `SeasAGI-Client/src-app/build/bin/` |
| 클라이언트 DMG | `SeasAGI-Client/build/` |
| 서버 커뮤니티 바이너리 | `SeasAGI-Server/build/` |
| 서버 엔터프라이즈 바이너리 | `SeasAGI-Server-Enterprise/build/` |

---

## 기술 스택

| 계층 | 기술 |
|-------|------------|
| 프론트엔드 | React + TypeScript + Vite + Zustand + @dnd-kit + Zod + Recharts |
| 데스크톱 프레임워크 | Wails v2 (Go + WebView) |
| 클라이언트 백엔드 | Go (로컬 게이트웨이 / 라우팅 엔진 / 비용 최적화 / Provider 실행자) |
| 서버 프레임워크 | Go + Gin |
| 데이터베이스 | SQLite |
| 결제 | Stripe (엔터프라이즈) |
| 배포 | systemd + nginx |
| 클라이언트 배포 | Homebrew Cask / DMG / 바이너리 |

---

## 문서

- [한 페이지 요약](SeasAGI.v5.md)
- [시스템 아키텍처 설계 문서](web-docs/系统架构设计文档.v5.md)
- [오픈소스 분석](开源分析.md)
- [클라이언트 README](SeasAGI-Client/README.md)
- [서버 커뮤니티 README](SeasAGI-Server/README.md)
- [서버 엔터프라이즈 README](SeasAGI-Server-Enterprise/README.md)

---

## 라이선스

- `SeasAGI-Client/` — **GPL 3.0**, 완전 오픈소스
- `SeasAGI-Server/` — **AGPL 3.0**, 커뮤니티 에디션 오픈소스 자체 호스팅 가능
- `SeasAGI-Server-Enterprise/` — **폐쇄소스 (BSL / EULA)**, 상업 배포용

---

## 커뮤니티

- [GitHub Issues](https://github.com/SeasX/SeasAGI/issues) — 버그 신고 / 제안 제출
- [GitHub Releases](https://github.com/SeasX/SeasAGI/releases) — 최신 버전 다운로드

---
