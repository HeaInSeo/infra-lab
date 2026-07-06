# MCP Tool Discovery 개선 설계와 스프린트 일정

## 1. 목적

현재 infra-lab MCP는 env 생성/삭제, addon 설치/제거, profile write, plan-only, operation 관리 도구를 제공한다.
하지만 agent가 전체 tool 목록을 부분적으로만 요약하면 사용자는 실제 가능한 작업을 낮게 인식할 수 있다.

목표는 다음과 같다.

```text
agent가 infra-lab MCP로 무엇을 할 수 있는지 정확히 설명하게 한다.
setup check에서 실제 등록된 tool 목록을 카테고리별로 보여준다.
사용자가 "뭘 할 수 있어?"라고 물었을 때 조회/계획/파일 write/승인형 실행을 구분해서 답하게 한다.
```

이 작업은 권한을 새로 여는 작업이 아니다.
이미 등록된 typed tool의 발견성과 설명력을 높이는 작업이다.

---

## 2. 현재 문제

MCP 서버에는 다음 계열의 tool이 이미 있다.

```text
read-only:
  version, capabilities, doctor, env_list, env_status, k8s_status, vm_list, vm_version

evidence:
  collect_snapshot, summarize_health

plan-only:
  up_plan, down_plan, rebuild_plan, addon_install_plan, addon_uninstall_plan

profile write:
  profile_clone, profile_save_as, profile_validate_and_save

approved mutation:
  addon_install_prepare/commit
  env_up_prepare/commit
  env_down_prepare/commit
  env_clean_prepare/commit
  env_rebuild_prepare/commit
  addon_uninstall_prepare/commit

operation:
  operation_approve, operation_cancel, operation_status, operation_logs, operation_locks, operation_unlock_stale
```

하지만 agent가 tool 목록을 압축해서 설명하면 다음처럼 오해할 수 있다.

```text
operation_approve는 있는데 prepare tool은 없다.
조회/진단만 가능하다.
실제 VM 생성/삭제는 할 수 없다.
```

이는 구현 기능 부족이 아니라 discovery/description 문제다.

---

## 3. 설계 원칙

## 3.1 raw 권한은 추가하지 않는다

금지 tool은 계속 금지한다.

```text
run_shell
run_kubectl
run_ssh
run_tofu
raw_exec
```

## 3.2 실제 등록된 tool 기준으로 설명한다

정적 문서만 기준으로 하지 않는다.
MCP 서버가 현재 capability 필터를 통과해 실제 등록한 tool 목록을 기준으로 설명한다.

## 3.3 실행 flow를 tool 목록과 함께 보여준다

단순 이름 목록만으로는 부족하다.
변경 작업은 반드시 다음 흐름을 같이 보여준다.

```text
plan-only → prepare → operation_approve → commit → operation_status/logs
```

## 3.4 위험도와 실행 위치를 같이 표시한다

VM lifecycle 작업은 실제 lab state를 변경한다.
따라서 discovery 결과에 다음 경고를 포함한다.

```text
env_up/down/clean/rebuild commit은 원격 lab 장비에서 실행해야 한다.
commit은 승인 후에만 실행한다.
```

---

## 4. Tool Catalog 모델

`setup_check` 응답에 `tools` 필드를 추가한다.

예시:

```json
{
  "tools": {
    "summary": {
      "total": 40,
      "readOnly": 12,
      "planOnly": 5,
      "profileWrite": 3,
      "approvedMutation": 4,
      "destructive": 8,
      "operation": 6
    },
    "categories": [
      {
        "name": "readOnly",
        "title": "조회/진단",
        "description": "state를 변경하지 않는 조회 도구",
        "tools": [
          {
            "name": "env_list",
            "purpose": "환경 목록 조회",
            "mutates": false,
            "requiresApproval": false
          }
        ]
      }
    ],
    "flows": [
      {
        "name": "approvedEnvUp",
        "steps": [
          "up_plan",
          "env_up_prepare",
          "operation_approve",
          "env_up_commit"
        ]
      }
    ]
  }
}
```

---

## 5. 신규 Tool

`what_can_i_do`를 추가한다.

목적:

```text
agent가 자연어 질문 "이 도구로 뭘 할 수 있어?"에 정확히 답하도록 한다.
```

응답은 JSON envelope 문자열로 반환한다.

핵심 필드:

```text
data.tools.summary
data.tools.categories
data.tools.flows
data.safety
data.recommendedPrompts
```

---

## 6. Sprint 일정

## Sprint 1 - Tool catalog MVP

기간:

```text
개발: 0.5일
검증: 0.5일
총: 1일
```

범위:

```text
- setup_check에 data.tools 추가
- what_can_i_do 추가
- tool category summary 추가
- 사용자 설명서 업데이트
- make test-mcp 통과
```

통과 기준:

```text
- setup_check 응답에 prepare/commit tool이 카테고리별로 보인다.
- what_can_i_do가 조회/계획/profile write/승인형 실행/파괴적 실행을 구분한다.
- agent가 "operation_approve만 가능"이라고 오해할 가능성이 줄어든다.
```

## Sprint 2 - Agent prompt hardening

기간:

```text
개발: 0.5일
검증: 0.5일
총: 1일
```

범위:

```text
- docs/MCP_AGENT_WORKFLOW.ko.md 보강
- "뭘 할 수 있어?" 질문에 대한 권장 답변 형식 추가
- 실행 요청 시 plan/prepare/approve/commit 단계 안내 추가
```

## Sprint 3 - Remote execution clarity

기간:

```text
개발: 0.5일
검증: 0.5일
총: 1일
```

범위:

```text
- setup_check에 local/remote 실행 주의 문구 명확화
- 원격 lab에서 Codex를 실행하는 경우와 SSH wrapper를 쓰는 경우 구분
- VM lifecycle commit 주의 문구 강화
```

---

## 7. 시작 순서

우선 Sprint 1을 바로 진행한다.

```text
1. tool catalog 타입 추가
2. 현재 등록된 handlers 기준으로 category 구성
3. setup_check data.tools에 포함
4. what_can_i_do 등록
5. 사용자 설명서 업데이트
6. make test-mcp
```

