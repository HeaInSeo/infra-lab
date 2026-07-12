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
setup_check
what_can_i_do
version
capabilities
doctor
profile_list
env_list
```

목표:

```text
- MCP tool capability 확인
- 실제 등록된 MCP tool catalog 확인
- lab root와 prerequisite 확인
- 사용 가능한 profile/env 확인
```

`what_can_i_do` 결과를 기준으로 사용자가 "무엇을 할 수 있나?"를 물었을 때는 다음 범주를 구분해서 답한다.

```text
- 조회/진단
- 증거 수집
- 계획 전용
- profile 파일 작성
- 승인형 실행
- 파괴적 승인형 실행
- operation 관리
```

특히 `operation_approve`만 보고 "승인만 가능하다"고 답하지 않는다.
prepare/commit 계열 tool이 catalog에 있으면 다음 흐름이 가능하다고 설명한다.

```text
plan-only → prepare → operation_approve → commit → operation_status/logs
```

## 2. 상태 진단

특정 env를 진단할 때는 snapshot을 우선 사용한다.

```text
collect_snapshot
summarize_health
```

agent는 snapshot의 `data.health`, `data.findings`, `warnings`, `errors` 근거만 사용해 진단한다.

## 3. 변경 제안

실행 전에는 plan-only tool을 먼저 호출한다.

```text
up_plan
down_plan
rebuild_plan
addon_install_plan
addon_uninstall_plan
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
profile_save_as
profile_clone
profile_validate_and_save
```

저장 후에는 다음을 다시 수행한다.

```text
profile_validate
up_plan
```

## 5. 승인형 실행

실행은 항상 prepare로 시작한다.

예:

```text
addon_install_prepare
env_up_prepare
env_down_prepare
env_clean_prepare
env_rebuild_prepare
addon_uninstall_prepare
libvirt_vm_resume_prepare
container_image_build_push_prepare
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
libvirt_vm_resume_commit
container_image_build_push_commit
```

`operation_approve` 이후에는 `operationId`만으로 commit할 수 있다.
초기 token mode가 필요한 client는 `approvalToken`을 함께 넘겨도 된다.

원격 VM 작업 조건:

```text
- env_up_commit, env_down_commit, env_clean_commit, env_rebuild_commit은 로컬 개발 장비에서 실행하지 않는다.
- libvirt_vm_resume_commit도 원격 lab 장비에서만 실행한다.
- 원격 lab 장비의 MCP server 또는 원격 checkout 기준으로 실행한다.
- commit 직전 operation_status로 target.env, target.profile, risk, destructive를 다시 확인한다.
```

libvirt paused VM 복구:

```text
- doctor/collect_snapshot에서 LIBVIRT_VM_PAUSED, LIBVIRT_IO_ERROR, HOST_NOSPACE 근거를 확인한다.
- storage pressure나 block I/O 원인을 먼저 해소한다.
- libvirt_vm_resume_prepare → operation_approve → libvirt_vm_resume_commit 순서로 실행한다.
```

컨테이너 이미지 build/push:

```text
- raw docker/podman 명령을 agent가 직접 실행하지 않는다.
- container_image_build_push_prepare에서 contextDir, dockerfile, image, sourceDigest를 확인한다.
- operation_approve 후 container_image_build_push_commit으로 build/push를 수행한다.
- 실패 시 operation_logs로 builder stdout/stderr를 확인한다.
```

## 6. 실패 처리

실패하면 다음 순서로 확인한다.

```text
operation_status
operation_logs
collect_snapshot
```

lock 문제가 있으면 먼저 lock 목록을 조회한다.

```text
operation_locks
```

lock이 stale인 경우에만 해제한다.

```text
operation_unlock_stale
```

active lock은 해제하지 않는다.

## 7. 취소

아직 실행하지 않은 operation만 취소할 수 있다.

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

## 8. 권장 프롬프트

상태 조회:

```text
infra-lab MCP로 what_can_i_do를 실행하고 가능한 작업을 카테고리별로 보여줘.
```

상태 진단:

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
