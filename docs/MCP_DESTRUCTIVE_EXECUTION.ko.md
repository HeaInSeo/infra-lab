# infra-lab MCP Destructive Execution

이 문서는 Stage 7 approved destructive execution MCP 도구를 설명한다.

Stage 7에서는 파괴적 작업을 prepare/commit 2단계로만 허용한다.

## Tools

```text
infra_lab.env_down_prepare
infra_lab.env_down_commit
infra_lab.env_clean_prepare
infra_lab.env_clean_commit
infra_lab.env_rebuild_prepare
infra_lab.env_rebuild_commit
infra_lab.addon_uninstall_prepare
infra_lab.addon_uninstall_commit
infra_lab.operation_approve
infra_lab.operation_cancel
infra_lab.operation_locks
infra_lab.operation_unlock_stale
infra_lab.operation_status
infra_lab.operation_logs
```

## 정책

```text
- prepare 없이 commit할 수 없다.
- prepare 후 operation_approve로 명시 승인할 수 있다.
- APPROVED operation은 operationId만으로 commit할 수 있다.
- token mode client는 commit에 approvalToken을 함께 넘겨도 된다.
- 모든 destructive operation은 risk=HIGH, destructive=true다.
- env 단위 lock을 획득한 뒤 실행한다.
- audit 기록에 실패하면 실행하지 않는다.
- pre-snapshot/post-snapshot을 operation store에 저장한다.
- stdout/stderr는 operation store에 저장한다.
- MCP client는 실행 명령 문자열이나 raw flag를 만들 수 없다.
```

## 고정 실행 경로

```text
env_down:
  bin/ilab env down <env>

env_clean:
  FORCE=1 ENV_NAME=<env> scripts/k8s-tool.sh clean

env_rebuild:
  bin/ilab env rebuild <profile> --approve

addon_uninstall:
  ENV_NAME=<env> scripts/k8s-tool.sh addons-uninstall optional <addon>
```

`--approve`는 agent 입력에서 받지 않는다.
MCP 서버가 `env_rebuild_commit`의 고정 실행 경로에만 내부적으로 사용한다.

## prepare 예시

```json
{
  "env": "libvirt-cilium"
}
```

응답:

```json
{
  "ok": true,
  "command": "env.down.prepare",
  "contractVersion": "infra-lab.contract/v1",
  "data": {
    "operationId": "op_20260629_010000_env_down",
    "approvalToken": "sha256:...",
    "risk": "HIGH",
    "target": {
      "env": "libvirt-cilium",
      "targetFingerprint": "sha256:..."
    }
  },
  "warnings": [],
  "errors": []
}
```

## commit 예시

```json
{
  "operationId": "op_20260629_010000_env_down",
  "approvalToken": "sha256:..."
}
```

## 직접 테스트

```bash
make build
make build-mcp
make test-mcp
```

개발 환경에서는 prepare/status/logs를 먼저 검증한다.
commit 계열은 실제 lab 자원을 삭제하거나 재생성하므로 live lab에서만 실행한다.
VM 생성/삭제/재빌드 검증은 로컬 개발 장비가 아니라 원격 lab 장비에서만 수행한다.

명시 승인 흐름:

```text
env_down_prepare
  → operation_approve
  → env_down_commit
```

stale lock 복구 흐름:

```text
operation_locks
  → stale=true 확인
  → operation_unlock_stale
```
