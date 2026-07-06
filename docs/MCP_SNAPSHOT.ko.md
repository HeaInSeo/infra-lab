# infra-lab Snapshot MCP

Stage 2는 agent가 여러 read-only tool을 임의 순서로 호출하지 않고, 표준 snapshot evidence를 한 번에 받을 수 있게 한다.

## Tools

```text
collect_snapshot
summarize_health
```

두 tool은 현재 같은 evidence를 수집한다.

- `collect_snapshot`은 `command: "snapshot.collect"`를 반환한다.
- `summarize_health`는 `command: "health.summarize"`를 반환한다.

## Evidence Sources

MCP 서버는 다음 read-only JSON 명령을 조합한다.

```text
bin/ilab env status --json
bin/ilab vm list --json
bin/ilab k8s status --json
```

특정 env가 주어지면 env/k8s status에 env 이름을 전달한다.

```json
{
  "env": "libvirt-cilium"
}
```

## Response Shape

```json
{
  "ok": true,
  "command": "snapshot.collect",
  "contractVersion": "infra-lab.contract/v1",
  "data": {
    "env": "libvirt-cilium",
    "health": {
      "risk": "LOW",
      "summary": "Snapshot evidence collected"
    },
    "evidence": {
      "envStatus": {},
      "vms": {},
      "k8s": {}
    },
    "findings": []
  },
  "warnings": [],
  "errors": []
}
```

## Risk Policy

```text
LOW:
  evidence 수집 성공, ok:false evidence 없음

MEDIUM:
  일부 evidence가 ok:false 또는 수집 실패

UNKNOWN:
  모든 evidence 수집 실패
```

## Failure Handling

Snapshot tool은 일부 evidence 실패를 전체 tool 실패로 취급하지 않는다.

예를 들어 kubeconfig가 없으면:

```text
- k8s evidence에는 k8s.status ok:false envelope를 보존한다.
- envelope.warnings에 K8S_STATUS_UNAVAILABLE을 넣는다.
- data.findings에 K8S_STATUS_ERROR를 넣는다.
- snapshot 자체는 ok:true를 유지한다.
```

이 정책은 agent가 증거 부족을 명시적으로 보고 판단하게 만들기 위한 것이다.
