# infra-lab MCP 안정화 로드맵 설계 문서 v0.3

## 0. 문서 목적

이 문서는 `infra-lab`을 사람이 직접 CLI로 사용하는 도구에서 확장하여, AI agent가 MCP를 통해 안전하게 조회·진단·계획·승인형 실행까지 수행할 수 있도록 만드는 단계별 설계와 일정을 정의한다.

핵심 목표는 다음과 같다.

```text
agent가 infra-lab을 사용할 수 있게 한다.
하지만 agent에게 raw shell, raw kubectl, raw ssh, raw tofu 권한을 직접 주지 않는다.
대신 MCP 서버가 안정적인 typed tool만 제공한다.
```

최종 목표 구조는 다음과 같다.

```text
agent / MCP client
  ↓
infra-lab-mcp local stdio server
  ↓
contract / schema / policy / plan store / operation store / lock / audit / approval
  ↓
ilab --json
  ↓
k8s-tool.sh / tofu / kubectl / ssh / VM runtime
```

초기에는 별도 원격 서버를 만들지 않는다.
`infra-lab-mcp`는 로컬 STDIO 방식으로 실행되는 작은 프로세스로 시작한다.

---

# 1. 현재 infra-lab 상태 요약

현재 `infra-lab`에는 다음 구성요소가 있다.

```text
infra-lab/
  ilab/                 read-only 중심 Go CLI + 일부 profile/env 명령
  scripts/k8s-tool.sh   up/down/status/clean/addons-* 실행 진입점
  addons/               addon install/verify/uninstall 스크립트
  envs/                 profile 예제
  state/                runtime state
  remote/               SSH 기반 remote helper
```

현재 `ilab`에는 다음 계열의 명령이 있다.

```text
ilab doctor
ilab env list
ilab env status [env]
ilab env use <profile>
ilab env plan <profile>
ilab env up <profile>
ilab env down <profile>
ilab env rebuild <profile> --approve
ilab profile list
ilab profile show <name>
ilab profile validate <name>
ilab profile new <name>
ilab profile clone <src> <dst>
ilab k8s status [env]
ilab vm list
ilab vm list --all
ilab vm version <vm-name>
ilab vm ssh <vm-name>
```

현재 `k8s-tool.sh`에는 다음 계열의 명령이 있다.

```text
host-setup
host-cleanup
up
down
status
clean
addons-install
addons-uninstall
addons-verify
profile-cilium-collect
profile-cilium-verify
profile-gateway-verify
```

이 중 MCP에 바로 노출해도 되는 것은 read-only 계열이다.
`up`, `down`, `clean`, `rebuild`, `addons-install`, `addons-uninstall`, `vm ssh`는 위험도가 있으므로 단계적으로만 노출한다.

---

# 2. 핵심 설계 원칙

## 2.1 MCP는 shell proxy가 아니다

금지한다.

```text
run_shell
run_script
run_kubectl
run_ssh
run_tofu
raw_exec
debug_exec
```

허용하는 것은 typed tool이다.

```text
env_status
profile_validate
collect_snapshot
up_plan
env_up_prepare
env_up_commit
```

agent는 명령 문자열을 만들지 않는다.
agent는 MCP tool에 구조화된 인자를 넘긴다.
명령 선택, 검증, 실행 정책은 MCP 서버가 소유한다.

---

## 2.2 단계적으로 연다

한 번에 승인형 실행까지 열지 않는다.

단계는 다음 순서로 진행한다.

```text
Stage 0: Contract Foundation
Stage 1: Read-only MCP
Stage 2: Evidence / Snapshot
Stage 3: Plan-only MCP
Stage 4: Profile Write
Stage 5: Approved Safe Mutation
Stage 6: Approved Env Up
Stage 7: Approved Destructive Execution
```

각 단계마다 사용자가 직접 테스트할 수 있는 안정화 기간을 둔다.

---

## 2.3 Stage 0이 가장 중요하다

MCP 서버보다 먼저 안정화해야 하는 것은 `ilab --json` contract다.

MCP tool schema는 `ilab --json` contract 위에 만들어진다.
따라서 `ilab --json` 출력이 흔들리면 MCP도 흔들린다.

Stage 0의 핵심 산출물은 다음이다.

```text
- JSON envelope/output package
- command별 data schema
- golden JSON fixture
- make test-contract
- exit code 정책
- version/capabilities
```

---

# 3. JSON Contract

## 3.1 공통 Envelope

`ilab` 명령은 사람용 텍스트 출력과 별개로 `--json` 출력을 제공한다.

공통 envelope는 다음 형태를 사용한다.

```json
{
  "ok": true,
  "command": "env.status",
  "contractVersion": "infra-lab.contract/v1",
  "data": {},
  "warnings": [],
  "errors": []
}
```

실패도 항상 valid JSON이어야 한다.

```json
{
  "ok": false,
  "command": "profile.validate",
  "contractVersion": "infra-lab.contract/v1",
  "data": null,
  "warnings": [],
  "errors": [
    {
      "code": "PROFILE_INVALID",
      "message": "libvirt.sshPublicKey is required",
      "field": "libvirt.sshPublicKey"
    }
  ]
}
```

중요 규칙은 다음과 같다.

```text
- stdout에는 JSON만 출력한다.
- warning/log는 envelope.warnings에 넣는다.
- 디버그 로그는 stderr 또는 operation log로 보낸다.
- 에러가 나도 stdout은 valid JSON이어야 한다.
- contractVersion을 항상 포함한다.
- command 이름을 항상 포함한다.
- warnings와 errors는 비어 있어도 항상 배열로 존재한다.
```

---

## 3.2 Envelope warnings와 data 내부 상태 이름 분리

`envelope.warnings`는 전역 contract/runtime 수준 warning이다.

예:

```text
- deprecated field 사용
- 일부 capability 없음
- snapshot 일부 수집 실패
- fallback path 사용
```

`data` 내부에서는 `warnings`라는 이름을 쓰지 않는다.
이름 충돌을 피하기 위해 다음 이름을 사용한다.

```text
data.conditions:
  대상 리소스의 상태 조건

data.findings:
  snapshot/diagnostic 결과

data.health:
  요약 상태
```

금지 예시:

```json
{
  "data": {
    "status": "ok",
    "warnings": []
  },
  "warnings": []
}
```

권장 예시:

```json
{
  "data": {
    "status": "ok",
    "conditions": [
      {
        "type": "ClusterReachable",
        "status": "True",
        "reason": "KubeconfigValid",
        "message": "Cluster is reachable"
      }
    ]
  },
  "warnings": []
}
```

---

## 3.3 command별 data schema 문서화

Envelope만 정하면 부족하다.
MCP schema를 안정적으로 만들려면 command별 `data` 타입을 문서화해야 한다.

Stage 0 산출물에 다음 문서를 추가한다.

```text
docs/contracts/ILAB_JSON_CONTRACT.ko.md
docs/contracts/ILAB_COMMAND_DATA_SCHEMA.ko.md
docs/contracts/examples/*.json
```

우선 다음 command들의 data schema를 명시한다.

```text
version
capabilities
doctor
env.list
env.status
k8s.status
vm.list
vm.version
profile.list
profile.show
profile.validate
```

---

## 3.4 golden JSON fixture 기반 contract test

단순히 `jq .`로 파싱되는지만 보면 부족하다.

반드시 golden fixture 기반 테스트를 둔다.

예시:

```text
testdata/contracts/version.golden.json
testdata/contracts/capabilities.golden.json
testdata/contracts/env_list.golden.json
testdata/contracts/profile_validate_ok.golden.json
testdata/contracts/profile_validate_error.golden.json
testdata/contracts/doctor_missing_tool.golden.json
```

테스트는 최소한 다음을 검증한다.

```text
- valid JSON 여부
- contractVersion 존재
- command 존재
- ok 존재
- warnings 배열 존재
- errors 배열 존재
- command별 data schema 일치
- ok:false 응답에서도 envelope 유지
- data 내부에 warnings 필드가 없는지 검증
```

Stage 0에 다음 타깃을 추가한다.

```bash
make test-contract
```

---

## 3.5 exit code 정책

`ilab --json`은 stdout에 JSON을 출력하고, exit code로 오류 성격을 표현한다.

권장 정책은 다음과 같다.

```text
exit 0:
  ok:true

exit 1:
  ok:false 도메인 오류
  예: PROFILE_INVALID, ENV_NOT_FOUND, CLUSTER_UNREACHABLE

exit 2:
  usage/contract 오류
  예: invalid flag, unknown command, invalid argument

exit 3:
  runtime/system 오류
  예: filesystem permission error, JSON serialization failure, unexpected internal error

exit 124:
  ilab-managed timeout
```

중요한 규칙:

```text
- ok:false인 도메인 오류도 stdout에는 valid JSON을 출력한다.
- MCP runner는 exit code와 stdout JSON을 함께 해석한다.
- stdout JSON이 없고 non-zero exit이면 runtime failure로 처리한다.
```

예시:

```bash
bin/ilab profile validate bad.yaml --json
```

stdout:

```json
{
  "ok": false,
  "command": "profile.validate",
  "contractVersion": "infra-lab.contract/v1",
  "data": null,
  "warnings": [],
  "errors": [
    {
      "code": "PROFILE_INVALID",
      "message": "profile validation failed"
    }
  ]
}
```

exit code:

```text
1
```

---

## 3.6 timeout 책임 분리

timeout은 두 종류가 있다.

### 3.6.1 ilab-managed timeout

`ilab` 명령 자체가 timeout을 관리하는 경우다.

이 경우 stdout에 `ok:false` JSON을 출력하고 exit code `124`를 반환한다.

stdout:

```json
{
  "ok": false,
  "command": "k8s.status",
  "contractVersion": "infra-lab.contract/v1",
  "data": null,
  "warnings": [],
  "errors": [
    {
      "code": "COMMAND_TIMEOUT",
      "message": "k8s status timed out after 60s"
    }
  ]
}
```

exit code:

```text
124
```

### 3.6.2 MCP runner timeout

MCP runner가 `exec.CommandContext` 또는 동등한 mechanism으로 `ilab` 프로세스를 종료한 경우다.

이 경우 stdout JSON이 없을 수 있다.
따라서 MCP runner가 직접 envelope를 생성해야 한다.

MCP 생성 envelope:

```json
{
  "ok": false,
  "command": "k8s.status",
  "contractVersion": "infra-lab.contract/v1",
  "data": null,
  "warnings": [],
  "errors": [
    {
      "code": "COMMAND_TIMEOUT",
      "message": "MCP runner killed ilab after 60s"
    }
  ]
}
```

정책:

```text
ilab-managed timeout:
  stdout JSON 있음
  exit 124

MCP runner timeout:
  stdout JSON 없을 수 있음
  MCP가 COMMAND_TIMEOUT envelope 생성
```

---

# 4. Capabilities 정책

## 4.1 bootstrap 필수 capability

MCP 서버는 시작 시 다음 명령을 호출한다.

```bash
ilab version --json
ilab capabilities --json
```

다음 capability는 bootstrap 필수다.

```text
version.v1
capabilities.v1
```

MCP 서버 시작 조건:

```text
1. ilab version --json 성공
2. ilab capabilities --json 성공
3. version.v1 존재
4. capabilities.v1 존재
5. contractVersion 호환
```

이 조건이 실패하면 MCP 서버는 시작하지 않는다.

---

## 4.2 capability 기반 MCP tool registration

MCP 서버는 capability가 없는 tool을 등록하지 않는다.

예:

```json
{
  "ok": true,
  "command": "capabilities",
  "contractVersion": "infra-lab.contract/v1",
  "data": {
    "infraLabVersion": "0.4.0",
    "contractVersion": "infra-lab.contract/v1",
    "capabilities": [
      "version.v1",
      "capabilities.v1",
      "doctor.v1",
      "env.list.v1",
      "env.status.v1",
      "profile.validate.v1",
      "k8s.status.v1",
      "vm.list.v1",
      "vm.version.v1"
    ]
  },
  "warnings": [],
  "errors": []
}
```

등록 정책:

```text
- capability가 있으면 tool 등록
- capability가 없으면 tool 미등록
- 필수 capability가 너무 적으면 read-only degraded mode로 시작
- version/capabilities 자체가 실패하면 MCP 서버 시작 실패
```

이 정책은 Stage 1부터 구현한다.
나중에 붙이면 MCP tool schema와 runtime behavior가 어긋날 수 있다.

---

# 5. 경로와 저장소 정책

## 5.1 Config path

기본 config path는 XDG 기반을 따른다.

우선순위:

```text
1. INFRA_LAB_CONFIG_HOME
2. XDG_CONFIG_HOME/infra-lab
3. ~/.config/infra-lab
```

예:

```text
~/.config/infra-lab/
  profiles/
  mcp/
  audit/
  plans/
```

---

## 5.2 secretSalt 저장 위치

approval token 또는 fingerprint 서명에는 local secret이 필요하다.

secret path 우선순위:

```text
1. INFRA_LAB_MCP_SECRET_PATH
2. INFRA_LAB_CONFIG_HOME/mcp/secret
3. XDG_CONFIG_HOME/infra-lab/mcp/secret
4. ~/.config/infra-lab/mcp/secret
```

기본 저장 위치:

```text
~/.config/infra-lab/mcp/secret
```

권한:

```text
0600
```

생성 정책:

```text
- 없으면 MCP 서버가 최초 실행 시 생성
- 생성 시 32 bytes 이상 random secret 사용
- secret은 stdout/log/audit에 절대 출력하지 않음
```

회전 정책:

```text
- secret rotate 시 기존 PREPARED operation은 EXPIRED 처리
- 회전 명령은 future command로 둔다
- 권장 명령: ilab mcp secret rotate
```

초기에는 문서화만 하고, 실제 rotate command는 Stage 5 이후 구현해도 된다.

---

## 5.3 profile 저장 위치와 우선순위

repo 기반 `envs/`와 사용자 config 기반 profile이 공존하면 이름 충돌이 생길 수 있다.

조회 우선순위:

```text
1. explicit path
2. user profile dir: ~/.config/infra-lab/profiles/
3. repo envs/
```

저장 정책:

```text
- MCP profile write는 기본적으로 user profile dir에만 저장
- repo envs/에는 쓰지 않음
- 같은 이름이 이미 있으면 실패
- 덮어쓰기는 초기 범위에서 금지
- replace/overwrite는 별도 future stage에서만 고려
```

profile 응답에는 source를 명시한다.

```json
{
  "name": "libvirt-cilium",
  "source": "user",
  "path": "/home/user/.config/infra-lab/profiles/libvirt-cilium.yaml"
}
```

source 값:

```text
explicit
user
repo
```

---

## 5.4 audit log 위치

audit log는 write/mutation 이력을 남기기 위한 핵심 장치다.

audit path 우선순위:

```text
1. INFRA_LAB_AUDIT_PATH
2. INFRA_LAB_ROOT/state/.audit/operations.jsonl
3. INFRA_LAB_CONFIG_HOME/audit/operations.jsonl
4. XDG_CONFIG_HOME/infra-lab/audit/operations.jsonl
5. ~/.config/infra-lab/audit/operations.jsonl
```

audit 실패 정책:

```text
read-only:
  audit 필요 없음.
  audit 시도 실패가 있더라도 warning 처리.

profile write:
  audit 실패 시 기본은 실패.
  단, explicit best-effort mode가 있으면 warning 처리 가능.

mutation:
  audit 실패 시 무조건 실패.
```

mutation은 audit 없이 실행하면 안 된다.

---

## 5.5 plan store 위치

Stage 3부터 plan store를 도입한다.

plan store path 우선순위:

```text
1. INFRA_LAB_PLAN_STORE
2. INFRA_LAB_ROOT/state/.plans/
3. INFRA_LAB_CONFIG_HOME/plans/
4. XDG_CONFIG_HOME/infra-lab/plans/
5. ~/.config/infra-lab/plans/
```

기본 repo-local 위치:

```text
state/.plans/<planId>.json
```

기본 user config 위치:

```text
~/.config/infra-lab/plans/<planId>.json
```

정책:

```text
- Stage 3부터 plan은 기본적으로 저장한다.
- plan에는 expiresAt을 둔다.
- prepare는 planId를 입력으로 받을 수 있다.
- prepare 시 planFingerprint를 재계산한다.
- plan이 만료되었으면 prepare 실패.
- plan store에 쓸 수 없으면 plan-only command는 ok:false로 실패한다.
```

---

# 6. Plan Model

## 6.1 planId, planFingerprint, targetFingerprint

Stage 3의 plan-only 결과는 Stage 5 이후 prepare/commit과 연결되어야 한다.

Plan 결과에는 다음을 포함한다.

```text
planId
planFingerprint
targetFingerprint
risk
destructive
requiresApproval
expiresAt
```

예:

```json
{
  "planId": "plan_20260628_170000_env_up",
  "planFingerprint": "sha256:...",
  "targetFingerprint": "sha256:...",
  "action": "env_up",
  "env": "libvirt-cilium",
  "profile": "libvirt-cilium",
  "risk": "HIGH",
  "destructive": false,
  "requiresApproval": true,
  "createdAt": "2026-06-28T17:00:00+09:00",
  "expiresAt": "2026-06-28T19:00:00+09:00"
}
```

---

## 6.2 plan store JSON

```json
{
  "planId": "plan_20260628_170000_env_up",
  "contractVersion": "infra-lab.contract/v1",
  "planFingerprint": "sha256:...",
  "targetFingerprint": "sha256:...",
  "action": "env_up",
  "env": "libvirt-cilium",
  "profile": {
    "name": "libvirt-cilium",
    "path": "/home/user/.config/infra-lab/profiles/libvirt-cilium.yaml",
    "source": "user",
    "digest": "sha256:..."
  },
  "risk": "HIGH",
  "destructive": false,
  "requiresApproval": true,
  "createdAt": "2026-06-28T17:00:00+09:00",
  "expiresAt": "2026-06-28T19:00:00+09:00",
  "steps": [
    "validate profile",
    "collect pre-snapshot",
    "run env up",
    "collect post-snapshot"
  ]
}
```

---

## 6.3 prepare와 plan 연결

Stage 5 이후 prepare는 가능하면 기존 planId를 입력으로 받는다.

```json
{
  "planId": "plan_20260628_170000_env_up"
}
```

prepare는 다음을 검증한다.

```text
- planId 존재
- plan 미만료
- planFingerprint 재계산 일치
- targetFingerprint 재계산 일치
- capability 충족
- 현재 state가 plan 시점과 충돌하지 않음
```

---

# 7. Approval 설계

## 7.1 approval token의 역할

approval token은 사용자 승인 증명이 아니다.

approval token의 주요 목적은 다음이다.

```text
prepare 때 계산한 대상과
commit 때 실행하려는 대상이 같은지 보장한다.
```

즉 approval token은 **대상 동일성 보장 장치**다.

사용자 승인 UX는 별도로 정의한다.

---

## 7.2 사용자 승인 UX

초기 정책:

```text
v0.6:
  chat-level human confirmation을 사용할 수 있다.
  단, 문서상으로는 이것을 약한 승인 UX로 명시한다.
```

중기 정책:

```bash
ilab operation approve <operationId>
```

장기 정책:

```text
local approval file
signed approval
UI approval
```

권장 최종 흐름:

```text
prepare
  → operationId 생성
  → planFingerprint 생성
  → targetFingerprint 생성
  → approval.required:true 반환

사용자 승인
  → ilab operation approve <operationId>
  또는 infra-lab-mcp approve <operationId>

commit
  → operation 승인 상태 확인
  → fingerprint 재검증
  → lock 획득
  → 실행
```

---

# 8. Operation Model

## 8.1 Operation 상태

operation 상태는 다음으로 단순화한다.

```text
PREPARED
APPROVED
RUNNING
SUCCEEDED
FAILED
CANCELLED
EXPIRED
BLOCKED
```

`APPROVAL_REQUIRED`는 operation status로 사용하지 않는다.
승인 필요 여부는 `approval` field로 표현한다.

---

## 8.2 Approval field

```json
{
  "approval": {
    "required": true,
    "status": "required",
    "mode": "chat-confirmation-v1",
    "approvedAt": null,
    "approvedBy": null
  }
}
```

approval status 값:

```text
not_required
required
approved
rejected
expired
```

---

## 8.3 상태 전이

```text
prepare
  → PREPARED + approval.status=required

approve
  → APPROVED + approval.status=approved

commit
  → RUNNING

complete
  → SUCCEEDED / FAILED

cancel
  → CANCELLED

expire
  → EXPIRED

policy block
  → BLOCKED
```

승인이 필요 없는 operation은 다음 흐름을 따른다.

```text
prepare
  → PREPARED + approval.status=not_required

commit
  → RUNNING
  → SUCCEEDED / FAILED
```

---

## 8.4 Operation 저장 위치

Stage 5부터 operation store를 도입한다.

```text
state/.operations/<operationId>/
  operation.json
  stdout.log
  stderr.log
  pre-snapshot.json
  post-snapshot.json
```

user config fallback:

```text
~/.config/infra-lab/operations/<operationId>/
```

---

## 8.5 operation.json 예시

```json
{
  "operationId": "op_20260628_170000_env_up",
  "tool": "env_up",
  "status": "PREPARED",
  "risk": "HIGH",
  "destructive": false,
  "plan": {
    "planId": "plan_20260628_165900_env_up",
    "planFingerprint": "sha256:..."
  },
  "target": {
    "env": "libvirt-cilium",
    "profile": "libvirt-cilium",
    "targetFingerprint": "sha256:..."
  },
  "createdAt": "2026-06-28T17:00:00+09:00",
  "startedAt": null,
  "finishedAt": null,
  "approval": {
    "required": true,
    "status": "required",
    "mode": "chat-confirmation-v1",
    "approvedAt": null,
    "approvedBy": null
  }
}
```

---

## 8.6 commit 검증 기준

commit 시 다음을 검증한다.

```text
- operationId 존재
- operation status가 APPROVED이거나 승인 불필요 PREPARED 상태
- 초기 token mode인 경우 token 유효
- tool 일치
- env 일치
- planFingerprint 일치
- targetFingerprint 일치
- token 미만료
- lock 획득 가능
```

---

# 9. Lock 설계

## 9.1 Lock 단위

기본 lock 단위는 env이다.

```text
state/.locks/<env>.lock
```

---

## 9.2 Lock 필요 작업

```text
addon_install_commit
env_up_commit
env_down_commit
env_clean_commit
env_rebuild_commit
addon_uninstall_commit
```

---

## 9.3 Lock 불필요 작업

```text
version
capabilities
profile_list
profile_show
profile_validate
env_list
snapshot
plan-only
```

단, snapshot은 실행 중 operation이 있으면 envelope.warnings 또는 data.findings에 표시한다.

---

## 9.4 Lock 파일

```json
{
  "operationId": "op_20260628_170000_env_up",
  "env": "libvirt-flannel",
  "tool": "env_up_commit",
  "startedAt": "2026-06-28T17:00:00+09:00",
  "expiresAt": "2026-06-28T19:00:00+09:00",
  "pid": 12345,
  "hostname": "devbox"
}
```

---

## 9.5 Stale lock 정책

```text
- expiresAt 이전 자동 해제 금지
- expiresAt 이후 stale candidate
- 자동 해제는 기본 비활성
- 수동 복구 명령으로 해제
```

권장 명령:

```bash
ilab operation locks
ilab operation unlock <env> --stale-only
```

MCP tool로 raw unlock은 노출하지 않는다.
필요 시 read-only `operation_locks`만 노출한다.

---

# 10. Audit 설계

모든 write와 mutation은 audit log를 남긴다.

audit path는 다음 우선순위를 따른다.

```text
1. INFRA_LAB_AUDIT_PATH
2. INFRA_LAB_ROOT/state/.audit/operations.jsonl
3. INFRA_LAB_CONFIG_HOME/audit/operations.jsonl
4. XDG_CONFIG_HOME/infra-lab/audit/operations.jsonl
5. ~/.config/infra-lab/audit/operations.jsonl
```

예시:

```json
{
  "time": "2026-06-28T17:00:00+09:00",
  "operationId": "op_20260628_170000_env_up",
  "tool": "env_up_commit",
  "actor": "agent",
  "risk": "HIGH",
  "target": {
    "env": "libvirt-cilium",
    "profile": "libvirt-cilium"
  },
  "result": "failed",
  "errorCode": "KUBERNETES_BOOTSTRAP_FAILED"
}
```

audit 실패 정책:

```text
read-only:
  audit 필요 없음

profile write:
  audit 실패 시 기본은 실패
  explicit best-effort mode가 있으면 warning 처리 가능

mutation:
  audit 실패 시 무조건 실패
```

---

# 11. Tool 노출 정책

## 11.1 Tool 위험도 등급

| 등급 | 설명 | 예시 | MCP 노출 시점 |
| -- | -- | -- | -- |
| R0 | 메타데이터 조회 | version, capabilities | Stage 1 |
| R1 | 상태 조회 | env_list, env_status, k8s_status | Stage 1 |
| R2 | 증거 수집 | collect_snapshot | Stage 2 |
| P1 | 계획 생성 | up_plan, down_plan, rebuild_plan | Stage 3 |
| W1 | 파일 생성 | profile_new, profile_clone | Stage 4 |
| W2 | 낮은 위험 변경 | addon install prepare/commit | Stage 5 |
| W3 | 새 환경 생성 | env_up prepare/commit | Stage 6 |
| D1 | 파괴적 변경 | env_down, rebuild, clean, addon_uninstall | Stage 7 |

---

## 11.2 절대 금지 tool

MCP 서버에는 다음 tool을 만들지 않는다.

```text
run_shell
run_script
run_kubectl
run_ssh
run_tofu
raw_exec
debug_exec
```

`vm ssh`도 agent에게 직접 열지 않는다.
필요하면 `vm_version`, `collect_vm_evidence`처럼 read-only 명령으로 대체한다.

---

# 12. 아키텍처

## 12.1 초기 아키텍처: local stdio MCP

초기 구조는 다음과 같다.

```text
agent / MCP client
  ↓ stdio
bin/infra-lab-mcp
  ↓ exec.Command
bin/ilab --json
  ↓
infra-lab state / kubeconfig / VM runtime / Kubernetes API
```

초기에는 MCP 서버가 `ilab` 바이너리를 호출한다.

예:

```text
env_status
  → bin/ilab env status <env> --json
```

장점:

```text
- 구현이 빠르다.
- Go internal package 제약을 피할 수 있다.
- infra-lab 내부 구현 변경에 덜 취약하다.
- contract만 안정화하면 MCP 서버를 별도로 관리할 수 있다.
```

---

## 12.2 중기 아키텍처: shared package 분리

MCP가 안정화되면 `ilab/internal/lab`의 공용 로직을 `pkg/lab` 또는 `pkg/contract`로 옮긴다.

목표 구조:

```text
infra-lab/
  pkg/
    lab/
    contract/
    operation/
    policy/
  ilab/
    cmd/
  mcp/
    cmd/infra-lab-mcp/
    internal/tools/
```

이 전환은 v0.1~v0.4 완료 후 진행한다.
초기에는 구현 안정성과 contract 검증을 우선한다.

---

## 12.3 장기 아키텍처: control API / workflow 분리

장기적으로는 다음 구조를 고려한다.

```text
agent
  ↓
MCP adapter
  ↓
infra-lab control API
  ↓
operation workflow / policy / audit / lock
  ↓
ilab / k8s-tool.sh / tofu / kubectl / VM runtime
```

이 문서의 1차 범위는 local stdio MCP이다.
HTTP MCP나 control API는 후속 확장으로 둔다.

---

# 13. Stage별 설계와 일정

# Stage 0 — Contract Foundation

## 목적

MCP를 만들기 전에 `ilab`이 machine-readable JSON contract를 안정적으로 제공하도록 한다.

이 단계의 목표는 단순히 `--json`을 추가하는 것이 아니다.
MCP tool schema의 기반이 되는 contract를 고정하는 것이다.

## 기간

```text
개발: 1주
안정화: 1주
총: 2주
```

---

## 구현 범위

추가할 명령:

```text
ilab version --json
ilab capabilities --json
ilab doctor --json
ilab env list --json
ilab env status [env] --json
ilab k8s status [env] --json
ilab vm list --json
ilab vm list --all --json
ilab vm version <vm-name> --json
ilab profile list --json
ilab profile show <name> --json
ilab profile validate <name> --json
```

---

## Stage 0 세부 PR

### PR 1 — JSON envelope/output package

```text
- 공통 Envelope 타입 추가
- Warning/ErrorInfo 타입 추가
- JSON 출력 helper 추가
- --json global flag 추가
- data 내부 warnings 금지 규칙 문서화
```

예상 파일:

```text
ilab/internal/output/envelope.go
ilab/internal/output/json.go
ilab/internal/output/error.go
ilab/cmd/root.go
```

### PR 2 — version/capabilities --json

```text
- ilab version --json
- ilab capabilities --json
- contractVersion 반환
- version.v1, capabilities.v1 반환
```

### PR 3 — exit code 정책 + timeout 책임 분리

```text
- exit code 정책 구현
- domain error와 usage/system error 구분
- ok:false JSON 출력 후 exit 1 처리
- ilab-managed timeout은 stdout JSON + exit 124
- stdout/stderr 분리
```

### PR 4 — doctor/env/profile read-only --json

```text
- doctor --json
- env list --json
- env status --json
- profile list --json
- profile show --json
- profile validate --json
```

### PR 5 — k8s/vm read-only --json

```text
- k8s status --json
- vm list --json
- vm list --all --json
- vm version --json
```

### PR 6 — command별 data schema 문서 + golden tests

```text
- docs/contracts/ILAB_JSON_CONTRACT.ko.md
- docs/contracts/ILAB_COMMAND_DATA_SCHEMA.ko.md
- docs/contracts/examples/*.json
- testdata/contracts/*.golden.json
- make test-contract
```

---

## command별 data schema 예시

### version data

```json
{
  "infraLabVersion": "0.4.0",
  "gitCommit": "abc123",
  "buildDate": "2026-06-28T00:00:00+09:00"
}
```

### capabilities data

```json
{
  "infraLabVersion": "0.4.0",
  "contractVersion": "infra-lab.contract/v1",
  "capabilities": [
    "version.v1",
    "capabilities.v1",
    "doctor.v1",
    "env.list.v1",
    "env.status.v1",
    "profile.validate.v1"
  ]
}
```

### profile.validate data

```json
{
  "profile": {
    "name": "libvirt-cilium",
    "source": "repo",
    "path": "envs/libvirt-cilium.yaml.example"
  },
  "valid": true,
  "normalized": {
    "backend": "libvirt",
    "cni": "cilium",
    "workers": 2
  },
  "conditions": [
    {
      "type": "SchemaValid",
      "status": "True",
      "reason": "ValidationPassed"
    }
  ]
}
```

### env.list data

```json
{
  "envs": [
    {
      "name": "libvirt-cilium",
      "source": "state",
      "stateDir": "state/libvirt-cilium",
      "status": "present"
    }
  ]
}
```

### env.status data

```json
{
  "env": "libvirt-cilium",
  "stateDir": "state/libvirt-cilium",
  "profile": {
    "name": "libvirt-cilium",
    "path": "state/libvirt-cilium/resolved-profile.yaml"
  },
  "backend": "libvirt",
  "cni": "cilium",
  "status": "ok",
  "conditions": [
    {
      "type": "StateDirPresent",
      "status": "True",
      "reason": "Found"
    }
  ]
}
```

전체 schema는 별도 문서에 정의한다.

---

## Stage 0 테스트

사람이 테스트할 명령:

```bash
make build
bin/ilab version --json | jq .
bin/ilab capabilities --json | jq .
bin/ilab doctor --json | jq .
bin/ilab env list --json | jq .
bin/ilab profile list --json | jq .
bin/ilab profile validate envs/multipass-flannel.yaml.example --json | jq .
make test-contract
```

통과 기준:

```text
- 모든 --json 출력이 jq로 파싱된다.
- 에러가 발생해도 valid JSON이 출력된다.
- stdout에 표 형태 텍스트가 섞이지 않는다.
- stderr에는 디버그/경고만 출력된다.
- contractVersion이 포함된다.
- command가 포함된다.
- warnings/errors 배열이 항상 존재한다.
- data 내부에는 warnings 필드를 사용하지 않는다.
- command별 data schema가 golden fixture와 일치한다.
- exit code 정책이 테스트된다.
- ilab-managed timeout과 MCP runner timeout 정책이 문서화된다.
```

---

# Stage 1 — Read-only MCP

## 목적

agent가 infra-lab 상태를 읽기만 할 수 있게 한다.

```text
agent → infra-lab-mcp → ilab --json
```

## 기간

```text
개발: 1주
안정화: 1주
누적: 4주
```

---

## MCP tools

```text
version
capabilities
doctor
env_list
env_status
k8s_status
vm_list
vm_version
profile_list
profile_show
profile_validate
```

---

## Stage 1 구현 PR

### PR 7 — infra-lab-mcp stdio skeleton

```text
- bin/infra-lab-mcp 생성
- stdio transport
- MCP server skeleton
- ilab runner
- runner timeout 처리
- runner timeout 시 MCP-generated COMMAND_TIMEOUT envelope 생성
```

### PR 8 — capability 기반 MCP tool registration

```text
- MCP 시작 시 ilab version --json 호출
- MCP 시작 시 ilab capabilities --json 호출
- version.v1, capabilities.v1 필수 검증
- capability 없는 tool은 등록하지 않음
- degraded mode 정의
```

### PR 9 — read-only MCP tools 연결

```text
- version/capabilities/doctor
- env_list/env_status
- profile_list/profile_show/profile_validate
- k8s_status
- vm_list/vm_version
```

---

## Runner 정책

```text
- command는 bin/ilab으로 고정한다.
- 인자는 tool별 allowlist로만 구성한다.
- timeout 기본값은 30초로 둔다.
- k8s_status는 60초까지 허용한다.
- raw argument passthrough 금지.
- INFRA_LAB_ROOT를 명시적으로 전달한다.
- non-zero exit code와 JSON envelope를 함께 해석한다.
- stdout JSON이 없고 runner timeout이면 MCP-generated COMMAND_TIMEOUT envelope 생성.
```

---

## 사람이 테스트할 방법

MCP 서버 직접 실행:

```bash
bin/infra-lab-mcp --transport stdio
```

agent 설정 예시:

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

테스트 프롬프트:

```text
현재 infra-lab 환경 목록을 보여줘.
현재 VM 목록을 보여줘.
profile validate 결과를 요약해줘.
현재 Kubernetes 노드와 Pod 상태를 요약해줘.
doctor 결과에서 누락된 도구를 알려줘.
```

---

## Stage 1 통과 기준

```text
- read-only MCP tool 정상 동작
- version.v1, capabilities.v1 bootstrap 검증
- capability 없는 tool은 등록되지 않음
- tool 호출 실패 시 errors[]가 안정적으로 반환
- ilab-managed timeout과 MCP runner timeout 모두 처리
- agent가 shell을 직접 만들지 않음
- agent가 MCP tool만 사용함
- raw shell 계열 tool 없음
- 조회 명령이 state를 변경하지 않음
```

---

# Stage 2 — Evidence / Snapshot

## 목적

agent가 진단에 필요한 증거를 한 번에 수집하게 한다.

단순 조회 tool 여러 개를 agent가 임의 순서로 호출하는 대신, MCP 서버가 표준 snapshot을 제공한다.

## 기간

```text
개발: 1주
안정화: 1주
누적: 6주
```

---

## MCP tools

```text
collect_snapshot
collect_cluster_evidence
collect_vm_evidence
collect_profile_evidence
summarize_health
```

---

## Snapshot 응답 예시

```json
{
  "ok": true,
  "command": "snapshot.collect",
  "contractVersion": "infra-lab.contract/v1",
  "data": {
    "env": "libvirt-flannel",
    "profile": {
      "name": "libvirt-flannel",
      "valid": true
    },
    "cluster": {
      "reachable": true,
      "nodesReady": 3,
      "podsNotReady": []
    },
    "vms": {
      "managed": 3,
      "running": 3,
      "stopped": 0
    },
    "health": {
      "risk": "LOW",
      "summary": "Cluster and VMs look healthy"
    },
    "findings": []
  },
  "warnings": [],
  "errors": []
}
```

---

## Risk level

```text
LOW       정상
MEDIUM    일부 warning 있음
HIGH      cluster unreachable, VM stopped, profile invalid 등
UNKNOWN   충분한 증거 없음
```

---

## Stage 2 통과 기준

```text
- state가 없는 경우에도 valid JSON
- kubeconfig가 없는 경우에도 valid JSON
- VM runtime이 없는 경우에도 valid JSON
- profile invalid 상태를 risk로 반영
- 실행 중 operation이 있으면 envelope.warnings 또는 data.findings에 표시
- data.warnings를 사용하지 않음
- agent가 snapshot 근거로만 진단
```

---

# Stage 3 — Plan-only MCP

## 목적

agent가 변경을 실행하지 않고, 변경 계획과 위험도를 판단하게 한다.

이 단계부터 `planId`, `planFingerprint`, `targetFingerprint`, `plan store`를 도입한다.

## 기간

```text
개발: 1주
안정화: 1주
누적: 8주
```

---

## MCP tools

```text
profile_diff
up_plan
down_plan
rebuild_plan
addon_install_plan
addon_uninstall_plan
```

---

## 원칙

이 단계의 tool은 절대 실행하지 않는다.

```text
up_plan              = env up 실행 안 함
down_plan            = env down 실행 안 함
rebuild_plan         = down/clean/up 실행 안 함
addon_install_plan   = addon 설치 안 함
addon_uninstall_plan = addon 제거 안 함
```

---

## Plan 응답 예시

```json
{
  "ok": true,
  "command": "plan.rebuild",
  "contractVersion": "infra-lab.contract/v1",
  "data": {
    "planId": "plan_20260628_170000_rebuild",
    "planFingerprint": "sha256:...",
    "targetFingerprint": "sha256:...",
    "action": "rebuild",
    "env": "libvirt-cilium",
    "profile": "libvirt-cilium",
    "destructive": true,
    "requiresApproval": true,
    "risk": "HIGH",
    "createdAt": "2026-06-28T17:00:00+09:00",
    "expiresAt": "2026-06-28T19:00:00+09:00",
    "reasons": [
      {
        "code": "STATE_DIR_WILL_BE_REMOVED",
        "message": "state/libvirt-cilium will be removed"
      },
      {
        "code": "ENV_DOWN_REQUIRED",
        "message": "current environment must be destroyed before rebuild"
      }
    ],
    "steps": [
      "validate profile",
      "collect current snapshot",
      "run env down",
      "remove state directory",
      "run env up",
      "collect post-run snapshot"
    ],
    "blocked": false
  },
  "warnings": [],
  "errors": []
}
```

---

## Stage 3 말에 준비할 타입

Stage 5에서 operation store를 도입하지만, 타입 정의는 Stage 3 말부터 준비한다.

```text
Operation
OperationStatus
Plan
PlanFingerprint
TargetFingerprint
ApprovalPolicy
ApprovalStatus
RiskLevel
```

---

## Stage 3 통과 기준

```text
- destructive 판정 정확
- requiresApproval 판정 정확
- immutable 변경 감지
- scale-in 감지
- planFingerprint 생성
- targetFingerprint 생성
- plan store에 plan 저장
- 만료된 plan 처리 가능
- plan 결과와 실제 ilab/k8s-tool 실행 경로 일치
- agent가 plan을 실행 완료로 오해하지 않음
```

---

# Stage 4 — Profile Write

## 목적

실제 인프라는 건드리지 않고, profile 파일만 생성·복제·저장한다.

이 단계는 write 작업이지만 VM, Kubernetes, tofu state는 변경하지 않는다.

## 기간

```text
개발: 1주
안정화: 1주
누적: 10주
```

---

## MCP tools

```text
profile_new
profile_clone
profile_save_as
profile_validate_and_save
```

---

## 정책

```text
- 기존 profile 덮어쓰기 금지
- save-as만 허용
- 저장 위치는 기본적으로 ~/.config/infra-lab/profiles/
- repo envs/에는 쓰지 않음
- 저장 후 반드시 validate 실행
- 저장 후 profile_diff와 up_plan 가능해야 함
- 모든 write는 audit log 기록
```

---

## Stage 4 통과 기준

```text
- 기존 파일 덮어쓰기 없음
- invalid profile 저장 시 명확한 error
- 저장 후 validate 결과가 JSON으로 반환
- profile source/precedence 정책 준수
- audit log 남음
- audit 실패 정책 준수
- git diff 또는 파일 diff 확인 가능
```

---

# Stage 5 — Approved Safe Mutation

## 목적

상대적으로 위험이 낮은 실제 변경을 승인 기반으로 허용한다.

Stage 5에서는 `addon_install`만 허용한다.
`addon_uninstall`은 safe mutation에서 제외한다.

## 기간

```text
개발: 1주
안정화: 1주
누적: 12주
```

---

## MCP tools

```text
addon_install_prepare
addon_install_commit
operation_status
operation_logs
```

제외:

```text
addon_uninstall_prepare
addon_uninstall_commit
```

---

## Operation store 도입

Stage 5부터 operation store를 도입한다.

```text
state/.operations/<operationId>/
  operation.json
  stdout.log
  stderr.log
  pre-snapshot.json
  post-snapshot.json
```

---

## Prepare 응답 예시

```json
{
  "ok": true,
  "command": "addon.install.prepare",
  "contractVersion": "infra-lab.contract/v1",
  "data": {
    "operationId": "op_20260628_170000_addon_install",
    "planId": "plan_20260628_165900_addon_install",
    "planFingerprint": "sha256:...",
    "targetFingerprint": "sha256:...",
    "approval": {
      "required": true,
      "status": "required",
      "mode": "chat-confirmation-v1"
    },
    "risk": "MEDIUM",
    "target": {
      "env": "libvirt-flannel",
      "addon": "metrics-server"
    },
    "steps": [
      "collect pre-snapshot",
      "run addon install",
      "run addon verify",
      "collect post-snapshot"
    ]
  },
  "warnings": [],
  "errors": []
}
```

---

## Commit 입력 예시

초기 버전에서는 token 기반 commit을 허용할 수 있다.

```json
{
  "operationId": "op_20260628_170000_addon_install",
  "approvalToken": "sha256:..."
}
```

하지만 문서상 권장 최종 형태는 다음이다.

```text
operation approve → commit
```

예:

```bash
ilab operation approve op_20260628_170000_addon_install
```

이후:

```json
{
  "operationId": "op_20260628_170000_addon_install"
}
```

---

## Stage 5 통과 기준

```text
- prepare 없이 commit 불가
- 다른 env로 commit 불가
- planFingerprint 불일치 시 commit 불가
- targetFingerprint 불일치 시 commit 불가
- token 또는 approval 상태 없으면 commit 불가
- lock 중복 실행 방지
- stdout/stderr 저장
- audit 없이는 mutation 실행 불가
- addon verify 실행
- 실패해도 operation_status로 확인 가능
```

---

# Stage 6 — Approved Env Up

## 목적

agent가 승인 기반으로 새 환경을 생성할 수 있게 한다.

단, 이 단계에서는 새 env 생성만 허용한다.

## 허용

```text
- 존재하지 않는 env에 env up
- profile validate 통과한 profile만 up
- up 후 snapshot 수집
```

## 금지

```text
- 기존 env 덮어쓰기
- env down
- env rebuild
- clean
- scale-in
- raw approve flag injection
```

## 기간

```text
개발: 1주
안정화: 1주
누적: 14주
```

---

## MCP tools

```text
env_up_prepare
env_up_commit
operation_status
operation_logs
```

---

## 실행 흐름

```text
1. env_up_prepare 호출
2. profile load
3. profile validate
4. env 존재 여부 확인
5. up_plan 생성 또는 planId 검증
6. pre-snapshot 저장
7. approval.required:true 반환
8. 사용자 승인
9. env_up_commit 호출
10. lock 획득
11. ilab env up <profile> 실행
12. resolved-profile.yaml 확인
13. post-snapshot 수집
14. audit 기록
```

---

## Stage 6 통과 기준

```text
- 새 env 생성 성공
- 기존 env에는 실행 거부
- 실패 시 원인 추적 가능
- lock 정상 해제
- post-snapshot 자동 수집
- audit 없이는 실행 불가
- agent가 --approve를 임의로 붙일 수 없음
- planFingerprint/targetFingerprint 불일치 시 실행 거부
```

---

# Stage 7 — Approved Destructive Execution

## 목적

마지막 단계에서 `down`, `rebuild`, `clean`, `addon_uninstall` 같은 파괴적 작업을 승인 기반으로 허용한다.

## 기간

```text
개발: 1주
안정화: 2주
누적: 17주
```

---

## MCP tools

```text
env_down_prepare
env_down_commit
env_rebuild_prepare
env_rebuild_commit
env_clean_prepare
env_clean_commit
addon_uninstall_prepare
addon_uninstall_commit
```

---

## 강제 정책

```text
- prepare/commit 2단계 필수
- approval 상태 필수
- operation lock 필수
- target fingerprint 필수
- plan과 commit 대상 일치 검증
- pre-snapshot 필수
- post-snapshot 필수
- audit log 필수
- clean은 down 완료 상태에서만 허용
- rebuild는 down → clean → up을 단계별 operation으로 기록
- addon_uninstall은 destructive로 취급
```

---

## Rebuild 정책

`rebuild`는 내부적으로 하나의 큰 black box로 처리하지 않는다.
다음 단계별 operation으로 기록한다.

```text
rebuild_prepare
  ↓
down_commit
  ↓
clean_commit
  ↓
up_commit
  ↓
post_snapshot
```

또는 하나의 parent operation 아래 child step으로 기록한다.

```json
{
  "operationId": "op_rebuild_001",
  "steps": [
    {
      "name": "down",
      "status": "succeeded"
    },
    {
      "name": "clean",
      "status": "succeeded"
    },
    {
      "name": "up",
      "status": "running"
    }
  ]
}
```

---

## Stage 7 통과 기준

```text
- 승인 없이 destructive 실행 불가
- fingerprint 불일치 시 실행 거부
- lock으로 동시 실행 방지
- 실패 단계 확인 가능
- clean은 down 이후만 가능
- rebuild 실패 후 복구 가이드 제공
- audit log로 전체 이력 추적 가능
- addon_uninstall도 destructive policy 적용
```

---

# 14. Error Code 체계

대표 error code는 다음과 같다.

```text
ROOT_NOT_FOUND
CAPABILITY_UNSUPPORTED
PROFILE_NOT_FOUND
PROFILE_INVALID
PROFILE_NAME_CONFLICT
PROFILE_SOURCE_CONFLICT
ENV_NOT_FOUND
ENV_ALREADY_EXISTS
KUBECONFIG_NOT_FOUND
CLUSTER_UNREACHABLE
VM_RUNTIME_NOT_FOUND
TOFU_NOT_FOUND
KUBECTL_NOT_FOUND
LOCK_HELD
LOCK_STALE
OPERATION_NOT_FOUND
APPROVAL_REQUIRED
APPROVAL_NOT_FOUND
APPROVAL_REJECTED
APPROVAL_TOKEN_INVALID
APPROVAL_TOKEN_EXPIRED
PLAN_NOT_FOUND
PLAN_EXPIRED
PLAN_STORE_UNAVAILABLE
PLAN_FINGERPRINT_MISMATCH
TARGET_FINGERPRINT_MISMATCH
FINGERPRINT_MISMATCH
PLAN_BLOCKED
COMMAND_TIMEOUT
COMMAND_FAILED
SNAPSHOT_FAILED
DESTRUCTIVE_ACTION_BLOCKED
SECRET_NOT_FOUND
SECRET_PERMISSION_INVALID
AUDIT_UNAVAILABLE
AUDIT_WRITE_FAILED
```

---

# 15. 테스트 전략

## 15.1 Contract test

```text
- envelope 필드 존재 검증
- command별 data schema 검증
- golden JSON fixture 비교
- ok:false 응답 검증
- exit code 검증
- data.warnings 금지 검증
- ilab-managed timeout contract 검증
```

명령:

```bash
make test-contract
```

---

## 15.2 Unit test

```text
- JSON envelope 생성
- error mapping
- profile validation
- profile precedence
- immutable conflict detection
- risk evaluator
- planFingerprint 계산
- targetFingerprint 계산
- plan store read/write
- approval token generation/verification
- secret path override
- audit path fallback
- lock acquire/release
- stale lock detection
```

---

## 15.3 Integration test

```text
- ilab --json 명령 실행
- MCP tool → ilab --json 매핑
- capability 없는 tool 미등록
- bootstrap capability 누락 시 MCP 시작 실패
- 없는 env 조회
- invalid profile 검증
- ilab-managed timeout 처리
- MCP runner timeout 처리
- operation log 생성
```

---

## 15.4 실제 lab 테스트

각 단계 안정화 기간에 사용자가 직접 수행한다.

```text
Stage 1:
  조회만 20~30회 반복

Stage 2:
  snapshot 기반 진단 반복

Stage 3:
  plan-only 결과와 실제 기대 실행 비교

Stage 4:
  profile 생성/clone/save-as 검증

Stage 5:
  addon install 승인형 실행 검증

Stage 6:
  새 env up 승인형 실행 검증

Stage 7:
  down/rebuild/clean/addon_uninstall 승인형 실행 검증
```

---

# 16. 전체 일정

## 16.1 권장 일정

| 주차 | 단계 | 개발 내용 | 안정화/사용자 테스트 |
| --: | -- | -- | -- |
| 1주차 | Stage 0 개발 | envelope, version/capabilities, exit code, timeout 정책 | 기본 JSON 출력 검증 |
| 2주차 | Stage 0 안정화 | command data schema, golden test | `make test-contract`, schema 리뷰 |
| 3주차 | Stage 1 개발 | MCP stdio skeleton, tool registration | MCP 연결 테스트 |
| 4주차 | Stage 1 안정화 | read-only tools | agent 조회 20~30회 반복 |
| 5주차 | Stage 2 개발 | snapshot/evidence/health summary | snapshot schema 검증 |
| 6주차 | Stage 2 안정화 | 없는 state/kubeconfig/runtime 테스트 | agent 진단 반복 |
| 7주차 | Stage 3 개발 | plan-only, plan store, risk evaluator | plan schema 검증 |
| 8주차 | Stage 3 안정화 | destructive/immutable/scale-in 판정 | 실제 시나리오별 plan 검증 |
| 9주차 | Stage 4 개발 | profile write, audit log | save-as/clone 테스트 |
| 10주차 | Stage 4 안정화 | profile precedence/conflict 처리 | validate/diff/up_plan 테스트 |
| 11주차 | Stage 5 개발 | operation store, approval token, lock, addon install | prepare/commit 테스트 |
| 12주차 | Stage 5 안정화 | addon install/verify 반복 | token/lock/audit 검증 |
| 13주차 | Stage 6 개발 | env_up prepare/commit | 새 env 생성 테스트 |
| 14주차 | Stage 6 안정화 | operation logs/status, post snapshot | 실패/재시도/중복 실행 테스트 |
| 15주차 | Stage 7 개발 | down/rebuild/clean/addon_uninstall prepare/commit | destructive dry-run 검증 |
| 16~17주차 | Stage 7 안정화 | destructive operation 보강 | 실제 down/rebuild/clean 검증 |

---

## 16.2 사용 가능 시점

```text
4주차 말:
  agent가 infra-lab을 안전하게 조회 가능

6주차 말:
  agent가 snapshot 기반으로 상태 진단 가능

8주차 말:
  agent가 변경 계획과 위험도를 제안 가능

10주차 말:
  agent가 profile 생성/복제 가능

12주차 말:
  agent가 승인 기반 addon install 가능

14주차 말:
  agent가 승인 기반 새 env up 가능

17주차 말:
  agent가 승인 기반 down/rebuild/clean/addon_uninstall 가능
```

---

# 17. 버전 계획

## v0.1 — JSON Contract Foundation

```text
범위:
- ilab --json
- version/capabilities
- command별 data schema
- golden tests
- exit code policy
- timeout 책임 분리

상태:
- MCP 없음
- agent 직접 사용 전 준비 단계
```

---

## v0.2 — Read-only MCP

```text
범위:
- local stdio MCP
- bootstrap capability check
- capability-based tool registration
- read-only tools

상태:
- agent 실사용 가능
- 변경 작업 없음
```

---

## v0.3 — Snapshot MCP

```text
범위:
- collect_snapshot
- health summary
- evidence tools
- data.findings / data.conditions 사용

상태:
- agent 진단 가능
```

---

## v0.4 — Plan-only MCP

```text
범위:
- up/down/rebuild/addon plan
- planId/planFingerprint/targetFingerprint
- plan store
- risk evaluator
- destructive 판정

상태:
- agent가 변경 제안 가능
- 실행 없음
```

---

## v0.5 — Profile Write

```text
범위:
- profile_new
- profile_clone
- profile_save_as
- profile precedence
- audit log 시작

상태:
- 파일 write만 가능
- infra 변경 없음
```

---

## v0.6 — Approved Safe Mutation

```text
범위:
- operation store
- approval token
- operation lock
- addon install prepare/commit
- operation status/logs

상태:
- 낮은 위험 변경 가능
- addon uninstall 제외
```

---

## v0.7 — Approved Env Up

```text
범위:
- env_up prepare/commit
- 새 env 생성만 허용

상태:
- agent가 승인 기반 환경 생성 가능
```

---

## v0.8 — Approved Destructive Execution

```text
범위:
- env_down prepare/commit
- env_rebuild prepare/commit
- env_clean prepare/commit
- addon_uninstall prepare/commit

상태:
- 승인 기반 파괴적 작업 가능
```

---

# 18. 구현 우선순위

가장 먼저 해야 할 작업은 다음 순서다.

```text
1. ilab JSON envelope/output package 추가
2. version/capabilities --json 추가
3. exit code 정책 구현
4. timeout 책임 분리 문서화 및 구현
5. doctor/env/profile read-only --json 추가
6. k8s/vm read-only --json 추가
7. command별 data schema 문서 추가
8. golden JSON fixture 추가
9. make test-contract 추가
10. infra-lab-mcp stdio skeleton
11. bootstrap capability check
12. capability 기반 tool registration
13. read-only MCP tools 연결
14. snapshot tool
15. plan-only tool + plan store + planFingerprint
16. profile write + audit
17. operation store + approval + lock
18. approved safe mutation
19. approved env up
20. destructive execution
```

---

# 19. 추천 PR 순서

```text
PR 1:
  ilab JSON envelope/output package 추가

PR 2:
  version/capabilities --json 추가

PR 3:
  exit code 정책 + timeout 책임 분리 + contract error model 추가

PR 4:
  doctor/env/profile read-only --json 추가

PR 5:
  k8s/vm read-only --json 추가

PR 6:
  command별 data schema 문서 + golden tests + make test-contract

PR 7:
  infra-lab-mcp stdio skeleton

PR 8:
  bootstrap capability check + capability 기반 MCP tool registration

PR 9:
  read-only MCP tools 연결

PR 10:
  snapshot/evidence tools

PR 11:
  plan-only tools + plan store

PR 12:
  profile write + audit path 정책

PR 13:
  operation store + approval token + secret path 정책

PR 14:
  lock/stale lock 정책

PR 15:
  addon install prepare/commit

PR 16:
  env_up prepare/commit

PR 17:
  destructive execution prepare/commit
```

---

# 20. 명시적 제외 범위

초기 범위에서 제외한다.

```text
- 원격 HTTP MCP 서버
- multi-user auth
- 웹 UI
- GitOps 연동
- Temporal/Argo workflow 연동
- Kubernetes Operator화
- raw kubectl tool
- raw ssh tool
- raw shell tool
- raw tofu tool
- production cluster 대상 실행
```

이 문서의 범위는 local lab 환경을 agent가 안전하게 사용하는 것이다.

---

# 21. 최종 결론

`infra-lab`은 MCP를 붙이기에 적합하다.
이미 `ilab`이라는 읽기 중심 CLI와 profile 기반 env 명령이 존재하기 때문이다.

하지만 MCP를 붙인다는 것은 agent에게 자유 명령 실행권을 주는 것이 아니다.

올바른 방향은 다음과 같다.

```text
1. ilab JSON contract를 먼저 고정한다.
2. command별 data schema와 golden test를 만든다.
3. data 내부 warning 명칭 충돌을 피하고 conditions/findings를 사용한다.
4. timeout 책임을 ilab-managed와 MCP runner timeout으로 분리한다.
5. MCP는 bootstrap capability를 확인한 뒤 read-only부터 연다.
6. snapshot으로 판단 근거를 안정화한다.
7. Stage 3부터 plan store와 planFingerprint를 도입한다.
8. profile write로 desired state 생성을 검증한다.
9. operation store, approval, lock, audit를 도입한다.
10. safe mutation을 승인 기반으로 연다.
11. 새 env up을 승인 기반으로 연다.
12. 마지막에 down/rebuild/clean/addon_uninstall을 승인 기반으로 연다.
```

권장 일정은 안정화 구간을 포함해 약 17주다.

단, 실제 사용 가능 시점은 더 빠르다.

```text
4주차: read-only MCP 사용 가능
6주차: snapshot 진단 가능
8주차: plan-only 가능
10주차: profile write 가능
12주차: addon install 가능
14주차: 새 env up 가능
17주차: destructive execution 가능
```

구현 착수의 첫 PR은 MCP가 아니다.
첫 PR은 `ilab` JSON envelope/output package여야 한다.

이 순서로 가면 agent가 infra-lab을 직접 사용하더라도, 각 단계마다 사람이 테스트하고 안정화한 뒤 다음 단계로 넘어갈 수 있다.
