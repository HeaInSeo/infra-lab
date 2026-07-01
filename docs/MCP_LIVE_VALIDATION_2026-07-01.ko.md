# MCP live 검증 기록 - 2026-07-01

## 범위

원격 lab 장비에서 MCP operation 흐름을 검증했다.
로컬 개발 장비에서는 VM 생성/삭제를 실행하지 않았다.

검증 기준:

```text
remote host:
  hosts/remote-lab.env의 LAB_REMOTE_SSH_TARGET 사용

remote checkout:
  codex/fix-mcp-addon-scope
  commit 8b2492c

MCP server:
  remote checkout의 bin/infra-lab-mcp
```

## Read-only 상태

현재 유효한 Kubernetes env는 `test-wizard-env`로 확인됐다.

```text
env:
  test-wizard-env

cluster:
  reachable: true
  nodesReady: 3
  podsNotReady: 0
  risk: LOW
```

`remote-seoy-libvirt-flannel` state도 존재하지만, 현재 VM build metadata는
`test-wizard-env`를 가리킨다. `remote-seoy-libvirt-flannel`의 kubeconfig는
현재 master IP/CA와 맞지 않아 `CLUSTER_UNREACHABLE`로 확인됐다.

## Operation UX

비파괴 operation 흐름은 원격 MCP에서 정상 동작했다.

```text
addon_install_prepare
operation_approve
operation_status
operation_cancel
operation_locks
```

확인 결과:

```text
- prepare 후 APPROVED 전이 성공
- APPROVED operation status 조회 성공
- cancel 시 CANCELLED 전이 성공
- operation_locks 조회 성공
```

## Addon install 반복 검증

대상:

```text
env: test-wizard-env
addon: metrics-server
```

첫 시도에서 `metrics-server`가 base addon인데 MCP가 optional addon으로 실행하는 문제가 발견됐다.

```text
operationId: op_20260701_125002_addon_install
status: FAILED
errorCode: COMMAND_FAILED
stderr: unknown optional addon: metrics-server
```

수정 후 `metrics-server`는 base addon으로 실행되도록 변경했다.

성공한 반복 검증:

```text
op_20260630_125231_addon_install  SUCCEEDED
op_20260630_125257_addon_install  SUCCEEDED
op_20260630_125259_addon_install  SUCCEEDED
```

각 operation은 다음 단계를 모두 성공 처리했다.

```text
- collect pre-snapshot
- run addon install
- run addon verify
- collect post-snapshot
```

## Env up/down 검증

새 테스트 env:

```text
profile: mcp-live-multipass
env: mcp-live-multipass
backend: multipass
cni: flannel
masters: 1
workers: 1
```

`env_up_prepare`와 `operation_approve`는 성공했다.
`env_up_commit`은 VM 생성 중 실패했다.

```text
operationId: op_20260701_083050_env_up
status: FAILED
failed step: run env up
errorCode: COMMAND_FAILED
```

원인:

```text
cannot find mount entry for snap core22 revision /var/lib/snapd/snap/core22/2411
```

이는 MCP contract 문제가 아니라 원격 장비의 Multipass/snap runtime 문제로 판단한다.
실패 후 operation logs로 원인 추적이 가능했고, lock은 정상 해제됐다.

부분 생성된 VM은 MCP destructive flow로 정리했다.

```text
operationId: op_20260701_092048_env_down
status: SUCCEEDED
steps:
  collect pre-snapshot: succeeded
  run env down: succeeded
  collect post-snapshot: succeeded
```

정리 후 `operation_locks`는 빈 목록이고, Multipass에는 기존 `podbridge5-dev`만 남았다.

이후 같은 원격 장비에서 Multipass 파일쓰기 경로를 재확인했다.

```text
podbridge5-dev 대상 multipass exec stdin 파일쓰기: 성공
```

남은 state dir 때문에 재시도가 막히는 것을 확인했고, `env_clean`이 target env를
명시하도록 MCP 구현을 보강했다.

```text
operationId: op_20260701_094439_env_clean
status: SUCCEEDED
target: mcp-live-multipass
result: state/mcp-live-multipass removed
```

재시도한 env up은 성공했다.

```text
operationId: op_20260701_094505_env_up
status: SUCCEEDED
steps:
  validate profile: succeeded
  collect pre-snapshot: succeeded
  run env up: succeeded
  collect post-snapshot: succeeded
```

생성된 클러스터 상태:

```text
env: mcp-live-multipass
backend: multipass
cni: flannel
nodesReady: 2
podsNotReady: 0
risk: LOW
```

VM build metadata도 확인했다.

```text
lab-master-0:
  envName: mcp-live-multipass
  backend: multipass
  cni: flannel
  role: control-plane

lab-worker-0:
  envName: mcp-live-multipass
  backend: multipass
  cni: flannel
  role: worker
```

성공 검증 후 테스트 VM과 state를 정리했다.

```text
operationId: op_20260701_095410_env_down
status: SUCCEEDED

operationId: op_20260701_095729_env_clean
status: SUCCEEDED
```

최종 상태:

```text
operation_locks: []
state/mcp-live-multipass: removed
multipass VMs: 기존 podbridge5-dev만 남음
```

## 결론

완료:

```text
- 원격 MCP bootstrap/tools/list 검증
- 원격 read-only 상태 조회 검증
- operation approve/cancel/status/logs/locks 검증
- addon install prepare/approve/commit 3회 반복 성공
- 실패 operation의 status/logs 기반 원인 추적 검증
- 실패한 env_up의 부분 VM 정리 env_down 성공
- env_clean target env 지정 보강 및 검증
- env_up prepare/approve/commit 성공 경로 검증
- env_down prepare/approve/commit 성공 경로 검증
- env_clean prepare/approve/commit 성공 경로 검증
```

남은 항목:

```text
- rebuild 성공 경로는 별도 긴 작업으로 남긴다.
```

현재 9점대 개선 과제 기준 진행률:

```text
완료 4 / 전체 4
진행률 100%
```
