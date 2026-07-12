# infra-lab MCP Libvirt Recovery

이 문서는 libvirt VM이 paused 상태가 된 뒤 agent가 MCP로 복구를 진행할 때의 안전 흐름을 정의한다.

## 범위

`doctor`와 `collect_snapshot`은 다음 문제를 read-only로 진단한다.

```text
LIBVIRT_VM_PAUSED
LIBVIRT_IO_ERROR
HOST_NOSPACE
```

복구 실행은 자동으로 하지 않는다. MCP는 승인형 operation으로만 VM resume을 제공한다.

## Tool

```text
libvirt_vm_resume_prepare
operation_approve
libvirt_vm_resume_commit
operation_status
operation_logs
```

`libvirt_vm_resume_prepare`는 `env`와 `vm`을 받는다. prepare 단계에서 `vm_list` 결과를 확인해 대상 VM이 해당 env의 managed libvirt VM인지 검증한다.

`libvirt_vm_resume_commit`은 승인된 operation만 실행한다. MCP client는 실행 명령이나 libvirt flag를 만들 수 없다. 서버가 고정 명령만 실행한다.

```text
virsh -c qemu:///system resume <vm>
```

## 권장 흐름

먼저 원인을 확인한다.

```text
doctor
collect_snapshot
vm_list
```

resume 전 확인:

```text
- host storage pressure가 해소됐는지 확인
- LIBVIRT_IO_ERROR 원인이 남아 있지 않은지 확인
- target.env와 target.vm이 의도한 값인지 확인
```

승인형 실행:

```text
libvirt_vm_resume_prepare
operation_approve
operation_status
libvirt_vm_resume_commit
operation_status
collect_snapshot
```

## 운영 기준

```text
- 이 tool은 non-destructive지만 risk는 HIGH다.
- 원격 lab 장비의 MCP server 또는 원격 checkout 기준으로 실행한다.
- prepare와 commit 모두 같은 env lock을 사용한다.
- pre/post snapshot과 operation stdout/stderr 로그를 남긴다.
- unmanaged VM이나 다른 backend VM은 거부한다.
```
