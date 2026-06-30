# infra-lab MCP Approved Safe Mutation

이 문서는 Stage 5 approved safe mutation MCP 도구를 설명한다.

Stage 5에서는 `addon_install`만 허용한다.
`addon_uninstall`은 destructive 작업으로 취급하며 이 단계에서 노출하지 않는다.

## Tools

```text
infra_lab.addon_install_prepare
infra_lab.operation_approve
infra_lab.addon_install_commit
infra_lab.operation_cancel
infra_lab.operation_locks
infra_lab.operation_unlock_stale
infra_lab.operation_status
infra_lab.operation_logs
```

## 정책

```text
- prepare 없이 commit할 수 없다.
- prepare 후 `operation_approve`로 명시 승인할 수 있다.
- APPROVED operation은 operationId만으로 commit할 수 있다.
- token mode client는 commit에 approvalToken을 함께 넘겨도 된다.
- approvalToken은 사용자 승인 증명이 아니라 prepare 대상과 commit 대상의 동일성 보장 장치다.
- env 단위 lock을 획득한 뒤 실행한다.
- audit 기록에 실패하면 addon install은 실행하지 않는다.
- stdout/stderr는 operation store에 저장한다.
- commit은 고정된 addons-install optional <addon> 및 addons-verify optional <addon>만 실행한다.
- raw shell, raw kubectl, raw ssh, raw tofu passthrough는 제공하지 않는다.
```

## 저장 위치

Operation store:

```text
state/.operations/<operationId>/
  operation.json
  stdout.log
  stderr.log
```

환경 변수 override:

```text
INFRA_LAB_OPERATION_STORE
INFRA_LAB_MCP_SECRET_PATH
INFRA_LAB_AUDIT_PATH
```

Lock:

```text
state/.locks/<env>.lock
```

## prepare 예시

입력:

```json
{
  "env": "libvirt-flannel",
  "addon": "metrics-server"
}
```

응답:

```json
{
  "ok": true,
  "command": "addon.install.prepare",
  "contractVersion": "infra-lab.contract/v1",
  "data": {
    "operationId": "op_20260629_010000_addon_install",
    "approvalToken": "sha256:...",
    "expiresAt": "2026-06-29T02:00:00Z",
    "approval": {
      "required": true,
      "status": "required",
      "mode": "token-v1"
    },
    "risk": "MEDIUM",
    "target": {
      "env": "libvirt-flannel",
      "addon": "metrics-server",
      "targetFingerprint": "sha256:..."
    }
  },
  "warnings": [],
  "errors": []
}
```

## commit 예시

입력:

```json
{
  "operationId": "op_20260629_010000_addon_install",
  "approvalToken": "sha256:..."
}
```

commit 실행 경로:

```text
scripts/k8s-tool.sh addons-install optional <addon>
scripts/k8s-tool.sh addons-verify optional <addon>
```

MCP client는 실행 명령 문자열을 만들 수 없다.

명시 승인 흐름:

```text
addon_install_prepare
  → operation_approve
  → addon_install_commit
```

## 직접 테스트

```bash
make build
make build-mcp
make test-mcp
```

실제 addon install commit은 live lab 환경에서만 수행한다.
개발 환경에서는 `addon_install_prepare`, `operation_status`, `operation_logs`를 먼저 검증한다.
