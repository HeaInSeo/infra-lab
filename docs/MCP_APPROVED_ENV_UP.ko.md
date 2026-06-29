# infra-lab MCP Approved Env Up

이 문서는 Stage 6 approved env up MCP 도구를 설명한다.

Stage 6에서는 새 환경 생성만 허용한다.
기존 env 덮어쓰기, down, rebuild, clean, scale-in은 허용하지 않는다.

## Tools

```text
infra_lab.env_up_prepare
infra_lab.env_up_commit
infra_lab.operation_status
infra_lab.operation_logs
```

## 정책

```text
- profile validate를 통과한 profile만 prepare할 수 있다.
- state/<env>가 이미 있으면 prepare와 commit 모두 거부한다.
- prepare 없이 commit할 수 없다.
- commit에는 operationId와 approvalToken이 필요하다.
- approvalToken은 prepare 대상과 commit 대상의 동일성 보장 장치다.
- env 단위 lock을 획득한 뒤 실행한다.
- audit 기록에 실패하면 env up은 실행하지 않는다.
- pre-snapshot/post-snapshot을 operation store에 저장한다.
- stdout/stderr는 operation store에 저장한다.
- commit은 고정된 ilab env up <profile> 경로만 실행한다.
- agent는 --approve 같은 raw flag를 주입할 수 없다.
```

## prepare 예시

입력:

```json
{
  "profile": "multipass-flannel"
}
```

응답:

```json
{
  "ok": true,
  "command": "env.up.prepare",
  "contractVersion": "infra-lab.contract/v1",
  "data": {
    "operationId": "op_20260629_010000_env_up",
    "approvalToken": "sha256:...",
    "expiresAt": "2026-06-29T02:00:00Z",
    "planFingerprint": "sha256:...",
    "targetFingerprint": "sha256:...",
    "approval": {
      "required": true,
      "status": "required",
      "mode": "token-v1"
    },
    "risk": "HIGH",
    "target": {
      "env": "multipass-flannel",
      "profile": "multipass-flannel",
      "profileDigest": "sha256:...",
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
  "operationId": "op_20260629_010000_env_up",
  "approvalToken": "sha256:..."
}
```

실행 경로:

```text
bin/ilab env up <profile>
```

MCP client는 실행 명령 문자열이나 추가 flag를 만들 수 없다.

## Operation Store

```text
state/.operations/<operationId>/
  operation.json
  stdout.log
  stderr.log
  pre-snapshot.json
  post-snapshot.json
```

## 직접 테스트

```bash
make build
make build-mcp
make test-mcp
```

개발 환경에서는 `env_up_prepare`, `operation_status`, `operation_logs`를 먼저 검증한다.
`env_up_commit`은 실제 VM/Kubernetes 환경을 생성하므로 live lab에서만 실행한다.
