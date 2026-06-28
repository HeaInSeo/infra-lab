# ilab JSON Contract

`ilab --json`은 MCP와 자동화 도구가 읽는 안정적인 JSON 응답 형식이다.

## Envelope

모든 JSON 응답은 다음 envelope를 따른다.

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

규칙:

- stdout에는 JSON만 출력한다.
- `contractVersion`, `command`, `ok`, `warnings`, `errors`는 항상 존재한다.
- `warnings`와 `errors`는 비어 있어도 배열이다.
- 실패 응답은 `ok:false`, `data:null`, `errors:[...]` 형태다.
- `data` 내부에서는 `warnings` 필드명을 쓰지 않고 `conditions`, `findings`, `health`를 사용한다.

## Exit Codes

```text
0    ok:true
1    domain error
2    usage/contract error
3    runtime/system error
124  ilab-managed timeout
```

## Implemented Capabilities

```text
version.v1
capabilities.v1
doctor.v1
env.list.v1
env.status.v1
profile.list.v1
profile.show.v1
profile.validate.v1
k8s.status.v1
vm.list.v1
vm.version.v1
```

Unsupported `--json` commands return `CAPABILITY_UNSUPPORTED`.
