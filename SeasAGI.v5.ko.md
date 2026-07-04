[🇬🇧 English](SeasAGI.v5.md) | [🇨🇳 中文](SeasAGI.v5.zh-CN.md) | [🇯🇵 日本語](SeasAGI.v5.ja.md) | [🇰🇷 한국어](SeasAGI.v5.ko.md)

---

# SeasAGI v5 한 페이지 요약

## 1. 한 줄 설명

SeasAGI는 AI 파워 유저와 개발자를 위한 로컬 통합 모델 게이트웨이로, 사용자가 네트워크 출구를 관리하는 것과 같은 방식으로 LLM API 출구를 관리할 수 있게 합니다.

SeasAGI는 단순한 "키 전환 도구"가 아닙니다 — AI 인프라 소비 측에 "로컬 통합 모델 게이트웨이" 진입점을 구축합니다.

그 가치는 다음과 같습니다:

- 다중 모델 통합 복잡성을 "클라이언트별 반복 설정"에서 "일회성 로컬 설정"으로 감소
- Key 보안에 대한 사용자 불안을 체감 가능한 제품 신뢰로 전환
- 런타임 Provider 불안정을 적응형 라우팅 저하로 전환하여 사용자에게 투명하게 처리
- 사용량 블랙박스를 전차원 인사이트(모델/채널/타임라인/비용/오류)로 전환
- 이미 작동하는 로컬 게이트웨이 기반 위에 미래의 더 높은 가치 전략 라우팅, 팀 에디션, 구독 서비스를 구축

## 2. 사용자 페인 포인트

현재 AI 클라이언트와 IDE 플러그인에는 여러 고빈도 페인 포인트가 있습니다:

- 다른 모델 플랫폼마다 `Base URL`, `API Key`, 모델 이름의 반복 설정이 필요
- 플랫폼 채널과 사용자가 구매한 Key가 혼재, 전환 비용이 높음
- 업스트림 서비스가 불안정할 때 통합 Fallback이나 진단이 없음
- 사용자들은 Keys가 플랫폼에 수집되거나 평문으로 저장될 것을普遍적으로 우려
- 대화 중 모델 전환으로 컨텍스트 단절 및 환각 발생
- Provider의 빈번한 429 속도 제한, 적응형 라우팅 우선순위 조정 없음
- 어떤 모델/채널이 가장 많이 사용되고, 비용이 최적인지 직관적인 인사이트 없음

## 3. 우리의 솔루션

SeasAGI는 로컬에 통합 OpenAI 호환 진입점을 제공하여, 모든 서드파티 클라이언트가 로컬 주소를 한 번만 설정하면 여러 모델 출구에 접근할 수 있습니다.

핵심 기능:

- 로컬 통합 진입점: 기본 `http://127.0.0.1:4318/v1`, 리슨 포트에 따라 동적 변경
- 듀얼 채널 아키텍처: 플랫폼 채널 + 커스텀 채널
- 사용자 제공 Keys는 절대 업로드되지 않고, 로컬 Keychain에만 저장
- 라우팅 전략: `fallback`, `round_robin`, `sticky`
- 모델 수준 Combo: 명시적 `channel + model` 단계 체인 + 드래그앤드롭 정렬 + 주/백업/최후수단 역할 의미론
- 내비게이션 융합: 좌측 내비를 "Combo 최적화" 단일 진입으로 통합, 4개 탭 워크벤치 (내 계획 / 최적화 제안 / 템플릿 센터 / 실행 분석)
- Playground 디버깅: Combo 선택기 + 실행 체인 시각화 + 단계 Fallback 상태 표시 (step_role 배지)
- 기본 진입점 Combo 지원: HomePage 기본 Combo 선택기 + Wails 지속성
- Combo 클라우드 동기화: 3계층 뷰 (로컬/공식/팀) + Fetch/Push/Update/Delete 전체 동기화 기능
- Combo 메트릭: 요청 수 / Fallback 수 / Step1 적중률 / 최종 단계 적중률
- 스마트 최적화 & Combo 협업: 다단계 Fallback 제안, 구조적 diff 미리보기, 원클릭 Combo 적용
- 단계별 Provider/Channel 후보 풀: 각 Step이 여러 후보 지원 + 자동 순위 지정
- 요청 수준 실행 제약: max_price / max_latency_ms / data_policy
- Tool-call 전용 라우팅 전략: 도구 성공률이 높은 Provider 우선
- 빠른 전략 별칭: stability-first / cost-first / speed-first / tool-first
- 작업 유형 인식: chat / tools / json / long_context / batch_low_cost 차별화 실행
- 엔터프라이즈 BYOK / 플랫폼 공유 용량 듀얼 레이어 Fallback
- 세션 고착 라우팅: 첫 사용자 메시지 기반 SHA1 해시, 30분 TTL, 대화 중 모델 전환 방지
- 동적 429 페널티 저하: PenaltyManager, 429 +3, 성공 -1, 2분 자연 감쇠
- Fallback 정렬 프리셋: intelligence / speed / budget 원클릭 재정렬
- 8개 Provider 실행자: OpenAI, Azure, Anthropic, Gemini, Ollama, DeepSeek, Grok, 플랫폼 릴레이
- 6개 포맷 변환기: OpenAI Chat/Responses, Anthropic, Gemini, Vertex AI, Passthrough
- Key 상태 확인: 5분 주기 프로브, 3회 연속 실패 시 자동 불건강 마킹
- Key 수준 Cooldown: Key별 120초 TTL, 429에 정확한 냉각, 다른 Key에 영향 없음
- 멀티모달 콘텐츠 평탄화: 텍스트 전용 배열 콘텐츠 자동 문자열 병합
- Playground 즉시 테스트: 채팅 UI + 모델 선택 + 로컬 게이트웨이 통신
- 상수 시간 Key 비교: crypto/subtle.ConstantTimeCompare, 타이밍 사이드채널 제거
- Zod 프론트엔드 검증: 8개 스키마 + 폼 통합
- 로깅 & 진단: 링크 시각화, CSV 내보내기, 구조화된 오류 복사
- RTK 토큰 압축: 9개 출력 유형 자동 감지 + 압축
- 전체 모달 API: Embeddings / Image / TTS / STT
- 터널 원격 접속: Cloudflare Tunnel + Tailscale Funnel
- 멀티 계정 장애 허용: 멀티 Key 순환 + 지수 백오프 + 서킷 브레이커 + 7개 오류 규칙
- OAuth 생태계: PKCE + 4개 Provider 레지스트리 + 로컬 루프백 콜백 + 토큰 자동 갱신
- 로컬 액세스 토큰: 좌측 내비 독립 페이지, 보기 / 복사 / 재설정
- 클라우드 기능: 회원가입 / 로그인, 플랫폼 채널 동기화, 사용량 / 결제 조회
- 클라우드 분석: 모델별/채널별 그룹화 + 비용 추정 (GPT-4o 가격) + 최근 오류 + 요청 타임라인 + 오류 분류 분포
- 관리자 시각화: Recharts BarChart / LineChart / PieChart, 6탭 UsagePage
- Caveman 간결 출력: 4개 스타일 주입
- 추론 콘텐츠: DeepSeek-R1/Kimi/QwQ 추론 체인 주입
- MCP 도구 중복 제거: 12개 동치 매핑
- 국제화: 중국어, 영어, 일본어, 한국어, 시스템 언어 따름

## 4. 왜 지금인가

- AI 사용자가 여러 클라이언트와 여러 모델을 동시에 사용하는 것이 일상이 됨
- 로컬 우선, 보안 우선, 통합 진입점 요구가 급격히 증가
- "클라우드 집약 플랫폼"은 존재하지만, "로컬 게이트웨이 + 강력한 보안 경계 + 적응형 라우팅 + 전체 체인 관측 가능성"을 갖춘 성숙한 제품은 여전히 희귀
- SeasAGI는 먼저 개별 개발자 시장을 침투한 후, 팀 에디션과 크로스 플랫폼 에디션으로 확장 가능

## 5. 제품 포지셔닝

- 대상 사용자:
  - AI 파워 유저
  - 개별 개발자
  - 인디 개발자
  - 소규모 AI 팀 기술 리드
- 현재 형태:
  - Wails 데스크톱 클라이언트 (주 검증 플랫폼 macOS, 코드 구조는 Windows / Linux 포함)
  - 로컬 OpenAI 호환 API 게이트웨이
- 핵심 시나리오:
  - 다중 모델 플랫폼 통합 접속
  - 플랫폼 채널과 자가 구매 Key 전환
  - 채널 장애 Fallback 및 적응형 저하
  - 요청 실패 링크 진단
  - 다중 턴 대화 세션 고착성
  - 사용량 인사이트 및 비용 최적화

## 6. 차별화 장벽

### 6.1 명확한 보안 경계

- 사용자 제공 Keys는 플랫폼에 절대 업로드되지 않음
- Keys는 macOS Keychain에 저장
- 로컬 액세스 토큰은 로컬 SQLite에 저장, 클라우드에 유입되지 않음
- 토큰 검증은 상수 시간 비교 사용, 타이밍 사이드채널 공격 제거
- 플랫폼 채널과 사용자 채널 경계가 UI에 명확히 표시

### 6.2 적응형 지능 라우팅

- 단순한 "기본 채널 전환기"가 아님
- 세션 고착성: 다중 턴 대화가 자동으로 동일 모델로 라우팅, 환각 방지
- 동적 429 페널티: Provider 속도 제한 시 자동으로 우선순위 하락, 성공 시 자동 복구
- 정렬 프리셋: intelligence / speed / budget 원클릭 재정렬
- Key 상태 확인: 자동 프로브, 실패한 Keys 자동 비활성화
- Key별 Cooldown: 정밀 제어, 다른 Key에 영향 없음
- 멀티모달 콘텐츠 평탄화: 더 많은 클라이언트 포맷과 호환
- 단계별 Provider/Channel 후보 풀 + 자동 순위 지정: 동일 단계 내에서 안정성/비용/지연 시간에 따라 최적 선택
- Tool-call 독립 라우팅 전략: 도구 성공률로 정렬, 단순 가격 우선이 아님
- 요청 수준 동적 제약: 각 호출에 max_price / max_latency_ms / data_policy 첨부 가능

### 6.3 모델 수준 Combo 오케스트레이션 & 거버넌스

- 장애 Fallback 오케스트레이션: 정적 모델 목록이 아닌, "주 → 백업 → 최후 수단" 명시적 Fallback 체인
- 내비게이션 융합: 좌측 내비 단일 진입 "Combo 최적화", 워크벤치 통합 흐름: 설정→제안→적용→검증
- 스마트 최적화 & Combo 협업: 다단계 Fallback 제안 + 구조적 diff 미리보기 + 원클릭 적용
- 클라우드 동기화: 3계층 뷰 (로컬/공식/팀) + 전체 동기화 (Fetch/Push/Update/Delete)
- 실행 분석: Combo 적중률 / Fallback률 / 평균 시도 수 대시보드
- 서버 제어 평면: 3수준 Combo 관리 + 버전 게시/롤백 + 차별화 출시
- 엔터프라이즈 거버넌스: 가시성 정책 (역할/계획/사용자 그룹/테넌트 4수준) + 배포 승인 흐름
- Combo 런타임 계측: 요청 수 / Fallback 수 / Step1 적중률 / 최종 단계 적중률
- 빠른 전략 별칭: stability-first / cost-first / speed-first / tool-first
- 작업 유형 인식: chat/tools/json/long_context별 차별화 실행

### 6.4 전체 체인 관측 가능성

- 로그에 적중 체인 및 단계별 실패 원인 포함
- 오류 분류 분포 (429/401/403/404/timeout/500/503)
- 요청 타임라인 차트 (24시간/7일/30일)
- 모델별/채널별 그룹 분석
- 최근 오류 빠른 보기
- 비용 추정 (GPT-4o 가격 기준)
- 관리자 차트 시각화 (BarChart / LineChart / PieChart)

### 6.5 로컬 게이트웨이 경험

- 모든 클라이언트가 로컬 주소를 한 번만 설정하면 됨
- Chatbox, Cherry Studio, Cursor 등에서 반복 설정 변경 불필요
- Playground 즉시 테스트, 채널 설정 후 즉시 검증

### 6.6 확장 가능한 아키텍처

- 외부적으로 통합 OpenAI 호환
- 내부적으로 프로토콜 정규화 + Provider 실행자로 더 많은 업스트림 확장
- 실행자 레지스트리가 새 Provider의 동적 등록 지원
- OAuth PKCE 흐름이 임의의 OAuth 2.0 Provider 지원
- 팀 에디션, 정책 출시, 설정 동기화로 원활하게 확장 가능

## 7. 3개 하위 프로젝트 구성

- `SeasAGI-Client/`: 클라이언트 하위 프로젝트, 내부적으로 `src-app` 원래 구조 유지, Wails 데스크톱 클라이언트 메인라인 소스 및 클라이언트 빌드 스크립트 포함
- `SeasAGI-Server/`: 서버 하위 프로젝트, 내부적으로 `platform-api`, `relay-gateway`, `src-admin`, `deploy` 원래 구조 유지, 제어 평면, 데이터 평면, 관리자 대시보드 및 배포 스크립트 포함
- `web-docs/`: 웹사이트 및 문서 하위 프로젝트, 내부적으로 `src-web` 유지, 메인라인 아키텍처/PRD/일정 문서 및 `cc-switch`, `9router` 등 역사 또는 참조 디렉터리 포함
- 루트 디렉터리: 최상위 `README.md`, `SeasAGI.v5.md` 및 원클릭 빌드 스크립트 `build-all.sh`만 유지

## 8. 현재 진행 상황

현재 `v5`에서 완료된 주요 기능:

- Wails + Go 클라이언트 실행
- 로컬 게이트웨이 `/v1/models`, `/v1/chat/completions` + 전체 모달 API 지원
- 플랫폼 채널 및 커스텀 채널 듀얼 채널 관리
- 라우팅 전략 `fallback` / `round_robin` / `sticky` + Combo 단계 체인
- 세션 고착 라우팅 (SHA1 + 세션→단계 + 30분 TTL)
- 동적 429 페널티 저하 (PenaltyManager + 시간 감쇠 + 서킷 브레이커 협업)
- Fallback 정렬 프리셋 (intelligence / speed / budget)
- Key 상태 확인 (5분 프로브 + 3회 연속 실패 자동 비활성화)
- Key 수준 Cooldown (Key별 120초 TTL + 자동 정리)
- 상수 시간 Key 비교 (crypto/subtle)
- 멀티모달 콘텐츠 평탄화
- Playground 즉시 테스트 페이지
- 8개 Provider 실행자 + 6개 포맷 변환기
- 로그 필터링, 정렬, CSV 내보내기, 구조화된 오류 복사
- RTK 토큰 압축 + 터널 원격 접속
- 멀티 계정 장애 허용 + OAuth 2.0 PKCE
- Caveman + 추론 + MCP 도구 중복 제거
- Zod 프론트엔드 검증 + Combo 드래그앤드롭 정렬
- 클라우드 분석 API (모델별/채널별 그룹화 + 비용 추정 + 최근 오류 + 타임라인 + 오류 분류 분포)
- 관리자 시각화 (Recharts BarChart / LineChart / PieChart)
- 국제화 (중국어/영어/일본어/한국어)
