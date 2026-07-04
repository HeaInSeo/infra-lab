# infra-lab MCP Tool Catalog

`infra_lab.tool_catalog`는 현재 실행 중인 MCP 서버에 실제 등록된 tool 목록을
반환하는 read-only introspection tool이다.

## 목적

`ilab capabilities --json`은 MCP 서버가 의존하는 낮은 레벨 JSON contract 기능을
나열한다. MCP 서버는 이 capability를 조합해 plan, profile write, approved
operation 같은 agent용 tool을 추가로 등록한다.

따라서 사용자가 "MCP에서 지금 무엇이 가능한가"를 보려면 `ilab capabilities`가
아니라 `infra_lab.tool_catalog`를 확인한다.

## Tool

```text
infra_lab.tool_catalog
```

입력은 없다.

## 응답 필드

```json
{
  "ok": true,
  "command": "mcp.tool_catalog",
  "contractVersion": "infra-lab.contract/v1",
  "data": {
    "tools": [
      {
        "name": "infra_lab.env_up_commit",
        "description": "Commit a prepared new environment creation operation after approval.",
        "category": "approved_env_up",
        "risk": "HIGH",
        "destructive": false,
        "requiresApproval": true,
        "source": "mcp-synthetic",
        "stage": "Stage 6",
        "requiredCapabilities": [
          "env.status.v1",
          "profile.validate.v1"
        ]
      }
    ]
  },
  "warnings": [],
  "errors": []
}
```

## 필드 의미

```text
name:
  MCP client가 호출하는 tool 이름.

category:
  read_only, evidence, plan, profile_write, approved_mutation,
  approved_env_up, destructive_execution, operation, introspection.

risk:
  LOW, MEDIUM, HIGH. 실행형 tool은 실제 실행 위험 기준이다.
  plan tool은 실행하지 않지만 대상 작업의 위험을 표시한다.

destructive:
  true면 자원 삭제/재생성/제거 계열 작업이다.

requiresApproval:
  true면 prepare/commit 또는 plan 승인 흐름을 전제로 한다.

source:
  ilab-capability: ilab JSON capability를 직접 MCP tool로 노출.
  mcp-synthetic: MCP 서버가 capability를 조합해 만든 tool.
  mcp-internal: MCP 서버 내부 introspection tool.

requiredCapabilities:
  이 tool을 등록하기 위해 필요한 ilab capability 목록.
```

## 정책

```text
- catalog에는 현재 등록된 tool만 나온다.
- capability가 부족해 등록되지 않은 tool은 catalog에도 나오지 않는다.
- catalog는 raw shell, raw kubectl, raw ssh, raw tofu 권한을 만들지 않는다.
- catalog는 조회 전용이며 lab 상태를 변경하지 않는다.
```
