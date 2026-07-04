# infra-lab Read-only MCP

이 문서는 Stage 1 read-only MCP 서버 사용법을 기록한다.

## 빌드

```bash
make build
make build-mcp
```

또는 검증까지 함께 실행한다.

```bash
make test-mcp
```

## 실행

```bash
INFRA_LAB_ROOT=/path/to/infra-lab bin/infra-lab-mcp --transport stdio
```

현재 transport는 `stdio`만 지원한다.

## MCP 설정 예시

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

## Bootstrap

MCP 서버는 시작 시 다음 명령을 호출한다.

```bash
bin/ilab version --json
bin/ilab capabilities --json
```

다음 capability가 없으면 서버 시작을 실패 처리한다.

```text
version.v1
capabilities.v1
```

다른 tool은 `ilab capabilities --json`에 있는 capability에 맞춰 등록한다.

## 제공 Tool

현재 read-only tool:

```text
infra_lab.version
infra_lab.capabilities
infra_lab.doctor
infra_lab.env_list
infra_lab.env_status
infra_lab.k8s_status
infra_lab.vm_list
infra_lab.vm_list_all
infra_lab.vm_version
infra_lab.profile_list
infra_lab.profile_show
infra_lab.profile_validate
infra_lab.tool_catalog
infra_lab.collect_snapshot
infra_lab.summarize_health
```

`infra_lab.tool_catalog`는 현재 MCP 서버에 실제 등록된 tool 목록과 각 tool의
필요 capability, category, risk, destructive 여부, 승인 필요 여부를 반환한다.
`ilab capabilities --json`은 낮은 레벨 capability만 보여주므로, agent가 실제로
볼 수 있는 MCP tool 목록은 이 catalog를 기준으로 확인한다.

금지:

```text
raw shell
raw script
raw kubectl
raw ssh
raw tofu
```

## Smoke Test

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"infra_lab.version","arguments":{}}}' \
  | INFRA_LAB_ROOT="$PWD" bin/infra-lab-mcp --transport stdio
```

응답의 `result.content[0].text`에는 `ilab --json` envelope가 문자열로 들어간다.
