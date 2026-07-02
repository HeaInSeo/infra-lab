# infra-lab MCP 사용자 설명서

이 문서는 `infra-lab` v0.7.0 기준으로 MCP를 사용해 lab 상태를 조회하고, 진단하고, 승인 기반 작업을 실행하는 방법을 설명한다.

대상 독자:

```text
- infra-lab을 CLI로 사용해 본 사용자
- MCP client에 infra-lab을 연결하려는 사용자
- AI agent에게 raw shell 권한 없이 lab 조회/계획/승인형 실행을 맡기려는 사용자
```

핵심 원칙:

```text
agent에게 raw shell, raw kubectl, raw ssh, raw tofu 권한을 주지 않는다.
agent는 infra-lab MCP의 typed tool만 사용한다.
실행 작업은 prepare → approve → commit 순서를 따른다.
VM 생성/삭제/재빌드 commit은 원격 lab 장비에서만 수행한다.
```

## 1. v0.7.0에서 가능한 것

가능:

```text
- ilab JSON contract 기반 read-only 조회
- MCP stdio server 실행
- version/capabilities/doctor/env/vm/k8s/profile 조회
- snapshot/health summary
- plan-only 변경 계획 생성
- profile 생성/복제/저장
- operation approve/cancel/status/logs/locks
- addon install 승인형 실행
- env up/down/clean 승인형 실행
- destructive prepare/commit 도구 등록
```

주의:

```text
- raw shell/kubectl/ssh/tofu tool은 제공하지 않는다.
- operation_approve는 명시 승인 UX이며, approvalToken은 target 동일성 검증 장치다.
- env_up/down/clean/rebuild commit은 실제 VM과 state를 변경한다.
- 로컬 개발 장비에서 VM lifecycle commit을 실행하지 않는다.
- 원격 lab 장비에서 테스트할 때도 operation_status로 target/risk를 다시 확인한다.
```

## 2. 빌드

저장소 루트에서 실행한다.

```bash
make build
make build-mcp
```

생성되는 바이너리:

```text
bin/ilab
bin/infra-lab-mcp
```

버전 확인:

```bash
bin/ilab version --json
```

예상 형태:

```json
{
  "ok": true,
  "command": "version",
  "contractVersion": "infra-lab.contract/v1",
  "data": {
    "infraLabVersion": "v0.7.0",
    "gitCommit": "...",
    "buildDate": "..."
  },
  "warnings": [],
  "errors": []
}
```

## 3. MCP 설정 메뉴

사람이 MCP JSON이나 긴 등록 명령을 직접 입력하지 않도록 setup 메뉴를 제공한다.

repo 루트에서 실행한다.

```bash
make mcp-setup
```

또는 빌드된 바이너리를 직접 실행해도 된다.

```bash
bin/infra-lab-mcp
```

메뉴:

```text
1. 상태 점검
2. Codex에 MCP 등록
3. Claude 설정 JSON 보기
4. 종료
```

권장 순서:

```text
1번 상태 점검
  → ready=true 확인
2번 Codex에 MCP 등록
  → Codex 재시작 또는 새 세션 열기
```

Claude Desktop은 설정 파일 위치와 병합 방식이 환경마다 다를 수 있다.
따라서 현재 단계에서는 3번으로 설정 JSON을 확인한 뒤 Claude 설정에 반영한다.

## 4. MCP 서버 실행 방식

일반 사용자는 MCP 서버를 직접 실행하지 않는다.
Codex나 Claude 같은 MCP client가 서버를 실행한다.

직접 실행이 필요한 경우는 setup/debug뿐이다.

상태 점검:

```bash
bin/infra-lab-mcp --doctor
```

MCP client가 실행하는 실제 stdio 모드:

직접 실행:

```bash
INFRA_LAB_ROOT=/path/to/infra-lab \
  /path/to/infra-lab/bin/infra-lab-mcp --transport stdio
```

MCP client 설정 예시:

```json
{
  "mcpServers": {
    "infra-lab": {
      "command": "/path/to/infra-lab/bin/infra-lab-mcp",
      "args": ["--transport", "stdio"],
      "env": {
        "INFRA_LAB_ROOT": "/path/to/infra-lab"
      }
    }
  }
}
```

MCP 서버는 시작 시 다음을 확인한다.

```text
bin/ilab version --json
bin/ilab capabilities --json
```

`version.v1`, `capabilities.v1`, contract version이 맞지 않으면 시작하지 않는다.

## 5. 첫 연결 후 확인할 것

처음 연결되면 setup check와 read-only tool부터 호출한다.

```text
infra_lab.setup_check
infra_lab.version
infra_lab.capabilities
infra_lab.doctor
infra_lab.profile_list
infra_lab.env_list
```

확인할 내용:

```text
- infra-lab 버전
- 사용 가능한 MCP tool 목록
- 필수 도구 설치 여부
- 현재 state/env 목록
- 사용 가능한 profile 목록
```

## 6. 상태 조회와 진단

특정 env의 단순 상태 조회:

```text
infra_lab.env_status
infra_lab.k8s_status
infra_lab.vm_list
infra_lab.vm_version
```

진단은 snapshot을 우선 사용한다.

```text
infra_lab.collect_snapshot
infra_lab.summarize_health
```

agent는 다음 필드만 근거로 진단해야 한다.

```text
data.health
data.findings
data.conditions
warnings
errors
```

좋은 프롬프트:

```text
infra-lab snapshot을 수집하고 health, findings, warnings만 근거로 현재 상태를 요약해줘.
```

## 7. 변경 전에는 plan-only를 먼저 사용한다

실행 없이 계획만 만든다.

```text
infra_lab.up_plan
infra_lab.down_plan
infra_lab.rebuild_plan
infra_lab.addon_install_plan
infra_lab.addon_uninstall_plan
infra_lab.profile_diff
```

계획에서 확인할 필드:

```text
risk
destructive
requiresApproval
blocked
reasons
steps
targetFingerprint
```

좋은 프롬프트:

```text
이 profile로 새 env를 만들 계획만 생성하고 risk, destructive, blocked 여부를 알려줘. 실행하지 마.
```

## 8. Profile 생성과 저장

인프라를 변경하지 않고 profile 파일만 만든다.

```text
infra_lab.profile_save_as
infra_lab.profile_clone
infra_lab.profile_validate_and_save
```

정책:

```text
- 기존 profile 덮어쓰기 금지
- 기본 저장 위치는 ~/.config/infra-lab/profiles/
- repo envs/에는 쓰지 않음
- 저장 후 validate 실행
- audit log 기록
```

profile 저장 후 권장 순서:

```text
infra_lab.profile_validate
infra_lab.up_plan
```

## 9. 승인형 실행 기본 흐름

모든 실행 작업은 3단계를 따른다.

```text
prepare
  → operation_approve
  → commit
```

prepare 후 확인할 것:

```text
operationId
risk
destructive
target
targetFingerprint
steps
approval.status
```

승인:

```text
infra_lab.operation_approve
```

승인 후 상태 확인:

```text
infra_lab.operation_status
```

commit:

```text
infra_lab.addon_install_commit
infra_lab.env_up_commit
infra_lab.env_down_commit
infra_lab.env_clean_commit
infra_lab.env_rebuild_commit
infra_lab.addon_uninstall_commit
```

`operation_approve` 이후에는 `operationId`만으로 commit할 수 있다.
초기 token mode client는 `approvalToken`을 함께 넘겨도 된다.

## 10. Addon install 예시

대상 env에 addon install을 준비한다.

```text
infra_lab.addon_install_prepare
```

예시 target:

```text
env: test-wizard-env
addon: metrics-server
```

확인:

```text
operation_status
target.env
target.addon
risk
steps
```

승인:

```text
infra_lab.operation_approve
```

실행:

```text
infra_lab.addon_install_commit
```

완료 후:

```text
infra_lab.operation_status
infra_lab.operation_logs
infra_lab.collect_snapshot
```

참고:

```text
metrics-server는 base addon으로 처리된다.
그 외 addon은 optional addon으로 처리된다.
```

## 11. 새 env 생성 예시

새 env를 만들기 전 profile을 준비한다.

```text
infra_lab.profile_save_as
infra_lab.profile_validate
infra_lab.up_plan
```

prepare:

```text
infra_lab.env_up_prepare
```

반드시 확인:

```text
target.env
target.profile
risk
destructive
steps
targetFingerprint
```

승인:

```text
infra_lab.operation_approve
```

실행:

```text
infra_lab.env_up_commit
```

생성 후 확인:

```text
infra_lab.operation_status
infra_lab.collect_snapshot
infra_lab.k8s_status
infra_lab.vm_list
```

주의:

```text
env_up_commit은 실제 VM과 Kubernetes cluster를 만든다.
로컬 개발 장비에서 실행하지 않는다.
원격 lab 장비의 MCP server 또는 원격 checkout 기준으로 실행한다.
```

## 12. env down / clean

테스트 env를 내릴 때:

```text
infra_lab.env_down_prepare
infra_lab.operation_approve
infra_lab.env_down_commit
```

state dir까지 제거할 때:

```text
infra_lab.env_clean_prepare
infra_lab.operation_approve
infra_lab.env_clean_commit
```

`env_clean`은 target env를 명시해 해당 env state만 제거한다.

정리 후 확인:

```text
infra_lab.operation_status
infra_lab.operation_locks
infra_lab.env_list
```

## 13. 실패 처리

실패하면 추측하지 말고 operation record와 log를 먼저 본다.

```text
infra_lab.operation_status
infra_lab.operation_logs
infra_lab.collect_snapshot
```

확인할 필드:

```text
status
errorCode
error
steps[].status
stdout
stderr
```

좋은 프롬프트:

```text
operation_status와 operation_logs를 보고 실패 단계를 특정해줘. 추측하지 말고 로그 근거만 사용해줘.
```

## 14. Lock 확인과 stale lock 해제

실행 중 operation은 env lock을 잡는다.

```text
infra_lab.operation_locks
```

active lock은 해제하지 않는다.
`expiresAt` 이후 stale로 확인된 lock만 해제한다.

```text
infra_lab.operation_unlock_stale
```

정책:

```text
- expiresAt 이전 자동 해제 금지
- stale candidate만 수동 해제
- raw unlock tool은 제공하지 않음
```

## 15. Operation 취소

아직 실행하지 않은 operation은 취소할 수 있다.

```text
infra_lab.operation_cancel
```

허용 상태:

```text
PREPARED
APPROVED
```

거부 상태:

```text
RUNNING
SUCCEEDED
FAILED
CANCELLED
EXPIRED
```

## 16. 원격 lab 사용 주의사항

VM lifecycle commit은 원격 lab 장비에서만 수행한다.

해당 작업:

```text
env_up_commit
env_down_commit
env_clean_commit
env_rebuild_commit
```

원격 검증 기준:

```text
- hosts/remote-lab.env 또는 동등한 원격 lab 설정 사용
- 원격 checkout에서 make build, make build-mcp 수행
- 원격 checkout의 bin/infra-lab-mcp를 사용
- commit 직전 operation_status로 target/risk 재확인
- 작업 후 operation_locks가 비었는지 확인
```

## 17. 실제 검증된 v0.7.0 흐름

v0.7.0에서 원격 lab 장비로 검증한 항목:

```text
- MCP bootstrap/tools/list
- read-only env/k8s/vm 조회
- operation approve/cancel/status/logs/locks
- metrics-server addon install prepare/approve/commit 3회
- mcp-live-multipass env_up prepare/approve/commit
- mcp-live-multipass env_down prepare/approve/commit
- mcp-live-multipass env_clean prepare/approve/commit
```

검증 기록:

```text
docs/MCP_LIVE_VALIDATION_2026-07-01.ko.md
```

## 18. 관련 문서

```text
docs/MCP_READONLY.ko.md
docs/MCP_SNAPSHOT.ko.md
docs/MCP_PLAN_ONLY.ko.md
docs/MCP_PROFILE_WRITE.ko.md
docs/MCP_APPROVED_SAFE_MUTATION.ko.md
docs/MCP_APPROVED_ENV_UP.ko.md
docs/MCP_DESTRUCTIVE_EXECUTION.ko.md
docs/MCP_AGENT_WORKFLOW.ko.md
docs/MCP_LIVE_VALIDATION_2026-07-01.ko.md
docs/contracts/ILAB_JSON_CONTRACT.ko.md
docs/contracts/ILAB_COMMAND_DATA_SCHEMA.ko.md
```
