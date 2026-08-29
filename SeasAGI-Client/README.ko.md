[🇬🇧 English](README.md) | [🇨🇳 中文](README.zh-CN.md) | [🇯🇵 日本語](README.ja.md) | [🇰🇷 한국어](README.ko.md)

---

# SeasAGI Client

[![Build status](https://ci.appveyor.com/api/projects/status/github/SeasX/SeasAGI?svg=true)](https://ci.appveyor.com/project/SeasX/SeasAGI)
[![macOS](https://img.shields.io/badge/platform-macOS-blue)](https://github.com/SeasX/SeasAGI)
[![Linux](https://img.shields.io/badge/platform-Linux-blue)](https://github.com/SeasX/SeasAGI)
[![Windows](https://img.shields.io/badge/platform-Windows-blue)](https://github.com/SeasX/SeasAGI)
[![License](https://img.shields.io/badge/license-GPL%20v3-green)](LICENSE)

데스크톱 클라이언트 하위 프로젝트로, 데스크톱 메인라인 소스 코드와 클라이언트 빌드 스크립트를 보관합니다.

## 디렉토리 구조

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

## 역할

- `src-app`: Wails v2 데스크톱 클라이언트 메인라인 소스 코드
- `src-app/frontend`: React + Vite + TypeScript 프론트엔드 (@dnd-kit 드래그 앤 드롭 + Zod 검증 포함)
- `src-app/internal`: 로컬 게이트웨이, 라우팅, OAuth, 로컬 액세스 토큰, 로깅, 최적화, 사용량 등 Go 모듈
- `scripts/build.sh`: 클라이언트 빌드 진입점, 현재 OS에 따라 자동 분기
- `scripts/build-macos.sh`: macOS 빌드 + DMG 패키징
- `scripts/build-linux.sh`: Linux (Ubuntu/CentOS) 빌드
- `scripts/build-windows.sh`: Windows 빌드
- `scripts/dev.sh`: Wails 로컬 개발 진입점
- `homebrew/seasagi.rb`: macOS Homebrew 배포 formula

## v5 핵심 기능

### 콤보 최적화 워크벤치 (통합 진입점)

좌측 내비게이션이 "콤보 최적화" 단일 진입점으로 통합되었으며, 워크벤치 내부는 4개의 탭으로 구성됩니다:

- **내 솔루션**: 로컬/공식/팀 3계층 Combo 뷰 + 출처 태그 + 클라우드 동기화 (Fetch/Push/Update/Delete)
- **최적화 제안**: 스마트 최적화 다단계 폴백 제안 + 구조 차이 미리보기 + Combo로一键 적용
- **템플릿 센터**: 공식/팀 템플릿 검색, 새로고침, 적용
- **실행 분석**: Combo 히트율/폴백율/평균 시도 횟수 분석 패널

### 모델 레벨 Combo

- Combo는 본질적으로 "장애 폴백 오케스트레이션"입니다: 주 모델 → 예비 모델 → 최후의 모델 (primary/backup/last_resort 3단계 역할 의미)
- 단계 체인 편집 + 드래그 앤 드롭 정렬 (@dnd-kit) + 단계별 channel_id + model 바인딩
- Fallback / Round-Robin 전략 + Sticky Uses 설정
- 클라우드 동기화 완전 지원: `FetchCloudCombos` / `PushCloudCombo` / `UpdateCloudCombo` / `DeleteCloudCombo`
- 로컬 마이그레이터 `migrateModelCombos()`가 자동으로 기존 데이터를 통합 Schema (combo_id/logical_name/display_name/status/source/version)로 보완
- Playground 즉시 테스트: Combo 선택기 + 실행 체인 시각화 + 단계 폴백 상태 표시 (step_role badge)

### 스마트 최적화와 Combo 협업

- 최적화 도구가 다단계 폴백 제안 지원 (primary/backup/last_resort)
- `PreviewComboOptimization` 구조 차이 미리보기 구현 (현재 단계 vs 제안 단계)
- `ApplyComboOptimization` 새 Combo 생성/덮어쓰기 업데이트 지원
- `ApplyRecommendation` 한 번의 클릭으로 Combo 생성 지원
- 추천 유형을 명시적으로 구분: 모델 교체 제안 / 폴백 체인 강화 제안 / 실행 매개변수 제안

### 라우팅 및 전략

- Fallback / Round-Robin / Sticky 세 가지 기본 전략
- Combo 단계 체인 (명시적 channel + model 조합)
- 세션 스티키 라우팅: 첫 번째 user message SHA1 해시 기반 session→step 매핑, 30분 TTL
- 동적 429 페널티 강등: PenaltyManager, 429 발생 시 +3 (최대 10), 성공 시 -1, 2분 자연 감소
- Fallback 정렬 프리셋: intelligence / speed / budget一键 재정렬
- Combo 단계 드래그 앤 드롭 정렬 (@dnd-kit)

### 단계 내 Provider/Channel 후보 풀 (OpenRouter 참조)

- 각 Step이 더 이상 단일 고정 channel만 지원하지 않고, 다중 후보承载 풀을 지원
- 단계 내 정렬 규칙: 안정성/비용/지연 시간/처리량에 따라 자동으로 후보 재정렬
- 요청 레벨 동적 제약: `max_price`, `max_latency_ms`, `data_policy`, `allow_cross_provider_fallback`
- 도구 호출 (tool-calling) 요청은 독립적인 라우팅 전략 사용 (tool success rate가 높은 provider 우선)
- 빠른 전략 별칭: 안정성 우선 / 비용 우선 / 속도 우선 / 도구 우선
- 작업 유형 인식 (task_profile): chat / tools / json / long_context / batch_low_cost 차등 실행

### Provider 및 내결함성

- 9가지 Provider 실행기: OpenAI / Azure / Anthropic / Gemini / Ollama / DeepSeek / Grok / Vertex / 플랫폼 릴레이
- 6가지 형식 변환: OpenAI Chat / OpenAI Responses / Anthropic / Gemini / Vertex AI / Passthrough
- 다중 Key 순환 + 지수 백오프 + 차단기 + 7가지 오류 규칙
- Key 상태 확인: HealthChecker, 5분 간격 정기 탐지, 연속 3회 실패 시 자동으로 unhealthy 표시
- Key 레벨 Cooldown: CooldownManager, per-key 120s TTL, 429 발생 시 쿨다운 설정
- Provider 레벨 상태 점수가 런타임 정렬 결정에 참여
- 엔터프라이즈 BYOK / 플랫폼 공유 용량 이중 레이어 폴백 전략
- 멀티모달 콘텐츠 평탄화: text-only array content 자동으로 string 병합

### Combo 런타임 계측 및 지표

- `ComboRouteMetrics`: 요청 횟수 / 폴백 횟수 / Step1 히트율 / 마지막 단계 히트율
- `step_role`이 RouteStep 로그에 전파됨 (primary/backup/last_resort)
- `GetComboRouteMetrics`가 프론트엔드에 노출됨
- `ExecAnalysisPage` 히트율 개요 막대 그래프 + 평균 시도 횟수 + Combo 상세 패널 표시

### 보안

- Keychain 통합 안전 저장
- 상수 시간 Key 비교 (crypto/subtle.ConstantTimeCompare)
- Zod 프론트엔드 폼 검증
- 터널 원격 액세스 (Cloudflare Tunnel + Tailscale Funnel)
- OAuth 2.0 PKCE (4개 Provider + Token 자동 갱신)

### MITM 프록시 — 원클릭 API 트래픽 하이재킹

- **원클릭 토글**: 설정 페이지 → MITM 탭에서 원클릭으로 시작/중지, 수동 인증서 설정 불필요
- **자동 CA 신뢰**: 루트 CA를 자동 생성하여 macOS/Linux/Windows 시스템 신뢰 체인에 설치, 도메인별 동적 인증서 발행 (23h TTL 캐시)
- **시스템 프록시 통합**: OS 수준 HTTP/HTTPS 프록시를 자동 설정하여 일치하는 모든 트래픽을 투명하게 하이재킹
- **도메인 화이트리스트**: 하이재킹 대상 API 도메인 구성 가능 (기본값: OpenAI, Anthropic, Gemini, DeepSeek, Grok, OpenRouter), 런타임에 동적 추가/제거
- **연결성 확인**: 도메인별「테스트」버튼으로 하이재킹 상태, 도달성, 지연 시간을 원클릭 검증
- **로컬 게이트웨이 라우팅**: 하이재킹된 HTTPS 트래픽이 로컬 SeasAGI 게이트웨이로 투명 전달되어 통합 라우팅, 최적화, 관측성 실현
- **실시간 하이재킹 로그**: 최근 20개 하이재킹 요청의 실시간 테이블 (메서드, 도메인, 경로, 상태 코드, 지연), 3초마다 자동 새로고침
- **CLI 호환 힌트**: 사용자 Shell(bash/zsh/fish/PowerShell/cmd) 자동 감지, 복사 가능한 `export` / `unset` 명령 제공, 시스템 프록시를 읽지 않는 터미널 도구도 지원
- **크래시 복구**: 재시작 시 잔류 시스템 프록시 자동 정리, 헬스 프로브 goroutine이 프록시 이상 감지 시 자동 중지
- **패스스루 안전성**: 비일치 도메인은 투명 터널 전달, 60초 타임아웃 보호로 일반 인터넷 사용에 영향 없음

### 사용자 경험

- Playground 즉시 테스트 페이지
- RTK Token 압축 (9가지 출력 유형 자동 감지)
- Caveman 간결 출력 (4가지 스타일)
- Reasoning Content 주입
- MCP 도구 중복 제거
- i18n 다국어 지원 (중국어/영어/일본어/한국어)

## 버전 관리

버전 번호는 단일 파일로 통합 관리되며, 빌드 시 프론트엔드에 주입되고 런타임 시 해당 파일에 의존하지 않습니다.

### 버전 출처

[`scripts/version.txt`](file:///Users/Neeke/data/www/SeasAGI/SeasAGI/SeasAGI-Client/scripts/version.txt) 가 유일한 버전 번호 정의 파일입니다:

```
0.1.0
```

- 버전 번호 업데이트는 이 파일만 수정하면 됩니다.
- 환경 변수 `SEASAGI_VERSION`으로 임시 덮어쓰기 가능 (예: `SEASAGI_VERSION=1.0.0 bash build-all.sh`)

### 빌드 시 주입流程

1. 각 플랫폼 빌드 스크립트 (`build-macos.sh` / `build-linux.sh` / `build-windows.sh`) 시작 시 `scripts/version.txt`를 읽음
2. `export VITE_APP_VERSION="$VERSION"`를 통해 프론트엔드 빌드 환경에 버전 번호 주입
3. Vite 빌드 시, `import.meta.env.VITE_APP_VERSION`이 정적 값으로 컴파일되어 프론트엔드 산출물에 기록됨
4. Wails가 프론트엔드 산출물을 데스크톱 클라이언트 바이너리에 패키징

### 프론트엔드 표시

클라이언트 인터페이스左上단 Logo 오른쪽에 버전 번호 배지가 표시됩니다:

```
[icon] SeasAGI  v0.1.0
```

구현 위치: [`Layout.tsx`](file:///Users/Neeke/data/www/SeasAGI/SeasAGI/SeasAGI-Client/src-app/frontend/src/components/Layout.tsx) 에서 `import.meta.env.VITE_APP_VERSION`을 사용하여 렌더링.

### 빌드 후 독립성

버전 번호는 빌드 시 이미 정적 값으로 컴파일되어 프론트엔드 산출물에 포함되며, `scripts/version.txt`는 클라이언트에 패키징되지 않습니다. 클라이언트 실행은 해당 파일과 완전히 독립적입니다.

## 자주 사용하는 명령어

```bash
# 클라이언트 개발 모드 시작
bash SeasAGI-Client/scripts/dev.sh

# 현재 플랫폼 클라이언트 컴파일
bash SeasAGI-Client/scripts/build.sh

# 특정 플랫폼 컴파일
bash SeasAGI-Client/scripts/build-macos.sh   # macOS
bash SeasAGI-Client/scripts/build-linux.sh    # Linux (Ubuntu/CentOS)
bash SeasAGI-Client/scripts/build-windows.sh  # Windows
```

## 빌드 산출물

- `SeasAGI-Client/src-app/build/bin/SeasAGI.app`
- `SeasAGI-Client/src-app/build/bin/SeasAGI.exe`
- `SeasAGI-Client/src-app/build/bin/SeasAGI`
- `SeasAGI-Client/build/SeasAGI-<version>.dmg` (macOS 전용)

## Linux 의존성

Ubuntu/Debian:
```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev build-essential
```

CentOS/RHEL:
```bash
sudo yum install gtk3-devel webkit2gtk3-devel
```