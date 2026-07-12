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
3. Claude Code에 MCP 등록
4. Claude 설정 JSON 보기
5. 종료
```

권장 순서:

```text
1번 상태 점검
  → ready=true 확인
2번 Codex에 MCP 등록 (또는 3번 Claude Code에 MCP 등록)
  → Codex/Claude Code 재시작 또는 새 세션 열기
```

Claude Code CLI는 `claude mcp add`로 codex와 동일하게 자동 등록된다 (3번, scope: user로 전역 등록).

Claude Desktop(GUI 앱)은 설정 파일 위치와 병합 방식이 환경마다 다를 수 있다.
따라서 Claude Desktop은 4번으로 설정 JSON을 확인한 뒤 Claude Desktop 설정에 직접 반영한다.

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

## 5. Setup check 출력 해설

상태 점검 예시:

```text
infra-lab MCP setup check
ready: true
root: /opt/go/src/github.com/HeaInSeo/infra-lab
server: /opt/go/src/github.com/HeaInSeo/infra-lab/bin/infra-lab-mcp --transport stdio
version: v0.7.0
contract: infra-lab.contract/v1
capabilities: 11
```

필드 의미:

```text
ready:
  MCP 서버를 사용할 준비가 되었는지 나타낸다.
  true면 기본 바이너리와 contract bootstrap이 정상이다.
  false면 findings/errors를 먼저 확인해야 한다.

root:
  MCP 서버가 infra-lab 저장소 루트로 인식한 경로다.
  이 경로를 기준으로 bin/ilab, state/, envs/ 등을 찾는다.

server:
  MCP client가 실행할 MCP 서버 바이너리와 transport 정보다.
  --transport stdio는 Codex/Claude가 stdin/stdout으로 통신한다는 뜻이다.

version:
  bin/ilab version --json에서 확인한 infra-lab 버전이다.

contract:
  ilab --json과 MCP가 합의한 JSON contract 버전이다.
  현재는 infra-lab.contract/v1을 사용한다.

capabilities:
  ilab이 현재 제공한다고 선언한 기능 수다.
  MCP 서버는 이 capability 목록을 보고 tool을 등록한다.

tools:
  MCP 서버가 현재 실제로 등록한 tool 목록을 카테고리별로 요약한다.
  capabilities가 ilab 기능 선언이라면, tools는 agent가 호출할 수 있는 MCP tool 목록이다.
```

`capabilities`는 MCP tool 자체가 아니라, `ilab --json`이 안정적으로 제공하는 기능 이름이다.
예를 들어 `env.status.v1` capability가 있어야 MCP 서버가 `env_status` tool을 안전하게 등록한다.

대표 capability:

```text
version.v1:
  ilab version --json 사용 가능

capabilities.v1:
  ilab capabilities --json 사용 가능

doctor.v1:
  ilab doctor --json 사용 가능

env.list.v1:
  ilab env list --json 사용 가능

env.status.v1:
  ilab env status --json 사용 가능

k8s.status.v1:
  ilab k8s status --json 사용 가능

vm.list.v1:
  ilab vm list --json 사용 가능

vm.version.v1:
  ilab vm version --json 사용 가능

profile.list.v1:
  ilab profile list --json 사용 가능

profile.show.v1:
  ilab profile show --json 사용 가능

profile.validate.v1:
  ilab profile validate --json 사용 가능
```

Tool catalog:

```text
tools.summary.total:
  현재 MCP 서버에 등록된 전체 tool 수다.

tools.summary.readOnly:
  version, doctor, env_status, vm_list 같은 조회 도구 수다.

tools.summary.evidence:
  collect_snapshot, summarize_health 같은 증거 수집 도구 수다.

tools.summary.planOnly:
  up_plan, down_plan, rebuild_plan 같은 실행 없는 계획 도구 수다.

tools.summary.profileWrite:
  profile_save_as, profile_clone 같은 profile 파일 write 도구 수다.

tools.summary.approvedMutation:
  addon_install, env_up처럼 승인 후 실행 가능한 변경 도구 수다.

tools.summary.destructive:
  env_down, env_clean, env_rebuild, addon_uninstall처럼 파괴적 변경 도구 수다.

tools.summary.operation:
  operation_approve, operation_status, operation_logs 같은 operation 관리 도구 수다.
```

agent가 "무엇을 할 수 있어?"라고 물었을 때는 `what_can_i_do`를 먼저 호출하는 것이 좋다.
이 tool은 현재 등록된 MCP tool을 다음처럼 구분해서 반환한다.

```text
조회/진단
증거 수집
계획 전용
profile 파일 작성
승인형 실행
파괴적 승인형 실행
operation 관리
```

중요한 차이:

```text
capabilities:
  ilab --json이 제공하는 기능 선언

tools:
  MCP client/agent가 실제 호출할 수 있는 tool 목록
```

바이너리 상태:

```text
binaries:
  ilab:
    MCP 서버가 내부적으로 호출하는 CLI다.
    exists=true, executable=true여야 한다.

  infra-lab-mcp:
    MCP 서버 자신이다.
    exists=true, executable=true여야 한다.
```

다음 단계:

```text
next steps:
  Ask the agent to run setup_check first after MCP connection.
    MCP client에 연결한 뒤 agent에게 setup_check를 먼저 호출하게 하라는 뜻이다.

  Use doctor for host prerequisite diagnostics.
    host 도구, VM runtime, 기본 환경 문제는 doctor로 확인하라는 뜻이다.

  Use collect_snapshot before diagnosing an existing lab.
    이미 생성된 lab 문제를 진단할 때는 snapshot을 먼저 수집하라는 뜻이다.
```

판단 기준:

```text
ready=true:
  MCP 연결 준비 완료.
  다음으로 doctor 또는 collect_snapshot을 호출한다.

ready=false:
  MCP 서버가 정상 사용 준비 상태가 아니다.
  findings/errors의 code와 message를 보고 build, path, permission 문제를 먼저 고친다.
```

## 6. 첫 연결 후 확인할 것

처음 연결되면 setup check와 read-only tool부터 호출한다.

```text
setup_check
what_can_i_do
version
capabilities
doctor
profile_list
env_list
```

확인할 내용:

```text
- infra-lab 버전
- 사용 가능한 MCP tool 목록
- 필수 도구 설치 여부
- 현재 state/env 목록
- 사용 가능한 profile 목록
```

`claude -p` 같은 헤드리스 실행에서 `--allowedTools`로 미리 권한을 열어줄 때는
MCP 서버 등록 이름(`infra-lab`)과 위 tool 이름을 그대로 이어붙인
`mcp__infra-lab__<tool 이름>` 형태를 사용한다 (예: `mcp__infra-lab__doctor`,
`mcp__infra-lab__env_status`). 헤드리스 세션은 대화형 승인 프롬프트를 띄울 수
없으므로 이름이 하나라도 틀리면 그 tool 호출은 승인 없이 그대로 차단된다.

## 7. 상태 조회와 진단

특정 env의 단순 상태 조회:

```text
env_status
k8s_status
vm_list
vm_version
```

진단은 snapshot을 우선 사용한다.

```text
collect_snapshot
summarize_health
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

## 8. 변경 전에는 plan-only를 먼저 사용한다

실행 없이 계획만 만든다.

```text
up_plan
down_plan
rebuild_plan
addon_install_plan
addon_uninstall_plan
profile_diff
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

## 9. Profile 생성과 저장

인프라를 변경하지 않고 profile 파일만 만든다.

```text
profile_save_as
profile_clone
profile_validate_and_save
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
profile_validate
up_plan
```

## 10. 승인형 실행 기본 흐름

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
operation_approve
```

승인 후 상태 확인:

```text
operation_status
```

commit:

```text
addon_install_commit
env_up_commit
env_down_commit
env_clean_commit
env_rebuild_commit
addon_uninstall_commit
```

`operation_approve` 이후에는 `operationId`만으로 commit할 수 있다.
초기 token mode client는 `approvalToken`을 함께 넘겨도 된다.

## 11. Addon install 예시

대상 env에 addon install을 준비한다.

```text
addon_install_prepare
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
operation_approve
```

실행:

```text
addon_install_commit
```

완료 후:

```text
operation_status
operation_logs
collect_snapshot
```

참고:

```text
metrics-server는 base addon으로 처리된다.
그 외 addon은 optional addon으로 처리된다.
```

## 12. 새 env 생성 예시

새 env를 만들기 전 profile을 준비한다.

```text
profile_save_as
profile_validate
up_plan
```

prepare:

```text
env_up_prepare
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
operation_approve
```

실행:

```text
env_up_commit
```

생성 후 확인:

```text
operation_status
collect_snapshot
k8s_status
vm_list
```

주의:

```text
env_up_commit은 실제 VM과 Kubernetes cluster를 만든다.
로컬 개발 장비에서 실행하지 않는다.
원격 lab 장비의 MCP server 또는 원격 checkout 기준으로 실행한다.
```

## 13. env down / clean

테스트 env를 내릴 때:

```text
env_down_prepare
operation_approve
env_down_commit
```

state dir까지 제거할 때:

```text
env_clean_prepare
operation_approve
env_clean_commit
```

`env_clean`은 target env를 명시해 해당 env state만 제거한다.

정리 후 확인:

```text
operation_status
operation_locks
env_list
```

## 14. 실패 처리

실패하면 추측하지 말고 operation record와 log를 먼저 본다.

```text
operation_status
operation_logs
collect_snapshot
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

## 15. Lock 확인과 stale lock 해제

실행 중 operation은 env lock을 잡는다.

```text
operation_locks
```

active lock은 해제하지 않는다.
`expiresAt` 이후 stale로 확인된 lock만 해제한다.

```text
operation_unlock_stale
```

정책:

```text
- expiresAt 이전 자동 해제 금지
- stale candidate만 수동 해제
- raw unlock tool은 제공하지 않음
```

## 16. Operation 취소

아직 실행하지 않은 operation은 취소할 수 있다.

```text
operation_cancel
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

## 17. 원격 lab 사용 주의사항

VM lifecycle commit은 원격 lab 장비에서만 수행한다.

해당 작업:

```text
env_up_commit
env_down_commit
env_clean_commit
env_rebuild_commit
libvirt_vm_resume_commit
```

원격 검증 기준:

```text
- hosts/remote-lab.env 또는 동등한 원격 lab 설정 사용
- 원격 checkout에서 make build, make build-mcp 수행
- 원격 checkout의 bin/infra-lab-mcp를 사용
- commit 직전 operation_status로 target/risk 재확인
- 작업 후 operation_locks가 비었는지 확인
```

## 18. 실제 검증된 v0.7.0 흐름

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

## 19. 관련 문서

```text
docs/MCP_READONLY.ko.md
docs/MCP_SNAPSHOT.ko.md
docs/MCP_PLAN_ONLY.ko.md
docs/MCP_PROFILE_WRITE.ko.md
docs/MCP_APPROVED_SAFE_MUTATION.ko.md
docs/MCP_APPROVED_ENV_UP.ko.md
docs/MCP_DESTRUCTIVE_EXECUTION.ko.md
docs/MCP_LIBVIRT_RECOVERY.ko.md
docs/MCP_AGENT_WORKFLOW.ko.md
docs/MCP_LIVE_VALIDATION_2026-07-01.ko.md
docs/contracts/ILAB_JSON_CONTRACT.ko.md
docs/contracts/ILAB_COMMAND_DATA_SCHEMA.ko.md
```
