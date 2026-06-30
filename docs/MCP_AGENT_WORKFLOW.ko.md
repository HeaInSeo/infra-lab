# infra-lab MCP Agent Workflow

이 문서는 agent가 infra-lab MCP를 사용할 때 따를 권장 흐름을 정의한다.

원칙:

```text
- raw shell, raw kubectl, raw ssh, raw tofu tool은 사용하지 않는다.
- 조회 → snapshot → plan → prepare → approve → commit 순서를 지킨다.
- destructive commit 전에는 operation_status와 target을 다시 확인한다.
- VM 생성/삭제/재빌드 commit은 원격 lab 장비에서만 수행한다.
```

## 1. 기본 조회

처음 연결되면 다음 순서로 확인한다.

```text
infra_lab.version
infra_lab.capabilities
infra_lab.doctor
infra_lab.profile_list
infra_lab.env_list
```

목표:

```text
- MCP tool capability 확인
- lab root와 prerequisite 확인
- 사용 가능한 profile/env 확인
```

## 2. 상태 진단

특정 env를 진단할 때는 snapshot을 우선 사용한다.

```text
infra_lab.collect_snapshot
infra_lab.summarize_health
```

agent는 snapshot의 `data.health`, `data.findings`, `warnings`, `errors` 근거만 사용해 진단한다.

## 3. 변경 제안

실행 전에는 plan-only tool을 먼저 호출한다.

```text
infra_lab.up_plan
infra_lab.down_plan
infra_lab.rebuild_plan
infra_lab.addon_install_plan
infra_lab.addon_uninstall_plan
```

응답에서 반드시 확인할 필드:

```text
risk
destructive
requiresApproval
blocked
reasons
steps
targetFingerprint
```

## 4. Profile Write

새 desired state가 필요하면 profile write tool을 사용한다.

```text
infra_lab.profile_save_as
infra_lab.profile_clone
infra_lab.profile_validate_and_save
```

저장 후에는 다음을 다시 수행한다.

```text
infra_lab.profile_validate
infra_lab.up_plan
```

## 5. 승인형 실행

실행은 항상 prepare로 시작한다.

예:

```text
infra_lab.addon_install_prepare
infra_lab.env_up_prepare
infra_lab.env_down_prepare
infra_lab.env_clean_prepare
infra_lab.env_rebuild_prepare
infra_lab.addon_uninstall_prepare
```

prepare 후 확인:

```text
operationId
risk
destructive
target
targetFingerprint
steps
approval.status
```

명시 승인:

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
초기 token mode가 필요한 client는 `approvalToken`을 함께 넘겨도 된다.

원격 VM 작업 조건:

```text
- env_up_commit, env_down_commit, env_clean_commit, env_rebuild_commit은 로컬 개발 장비에서 실행하지 않는다.
- 원격 lab 장비의 MCP server 또는 원격 checkout 기준으로 실행한다.
- commit 직전 operation_status로 target.env, target.profile, risk, destructive를 다시 확인한다.
```

## 6. 실패 처리

실패하면 다음 순서로 확인한다.

```text
infra_lab.operation_status
infra_lab.operation_logs
infra_lab.collect_snapshot
```

lock 문제가 있으면 먼저 lock 목록을 조회한다.

```text
infra_lab.operation_locks
```

lock이 stale인 경우에만 해제한다.

```text
infra_lab.operation_unlock_stale
```

active lock은 해제하지 않는다.

## 7. 취소

아직 실행하지 않은 operation만 취소할 수 있다.

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

## 8. 권장 프롬프트

상태 조회:

```text
infra-lab snapshot을 수집하고 health, findings, warnings만 근거로 요약해줘.
```

변경 계획:

```text
이 profile로 새 env를 만들 계획만 생성하고 risk와 blocked 여부를 알려줘. 실행하지 마.
```

승인형 실행:

```text
addon install prepare를 만들고 operation target과 risk를 요약해줘. 내가 승인하기 전에는 commit하지 마.
```

실패 진단:

```text
operation_status와 operation_logs를 보고 실패 단계를 특정해줘. 추측하지 말고 로그 근거만 사용해줘.
```
