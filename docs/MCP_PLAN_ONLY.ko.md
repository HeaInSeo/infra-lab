# infra-lab Plan-only MCP

Stage 3은 agent가 변경을 실행하지 않고 계획과 위험도를 확인하게 한다.

## Tools

```text
up_plan
down_plan
rebuild_plan
addon_install_plan
addon_uninstall_plan
```

이 tool들은 실행하지 않는다.

```text
up_plan              = env up 실행 안 함
down_plan            = env down 실행 안 함
rebuild_plan         = down/clean/up 실행 안 함
addon_install_plan   = addon 설치 안 함
addon_uninstall_plan = addon 제거 안 함
```

## Input

예:

```json
{
  "env": "libvirt-cilium",
  "profile": "libvirt-cilium",
  "addon": "metrics-server"
}
```

필드는 tool에 따라 선택적으로 사용된다.

## Response

```json
{
  "ok": true,
  "command": "plan.rebuild",
  "contractVersion": "infra-lab.contract/v1",
  "data": {
    "planId": "plan_20260629_110000_rebuild",
    "planFingerprint": "sha256:...",
    "targetFingerprint": "sha256:...",
    "action": "rebuild",
    "env": "libvirt-cilium",
    "profile": "libvirt-cilium",
    "destructive": true,
    "requiresApproval": true,
    "risk": "HIGH",
    "createdAt": "2026-06-29T11:00:00Z",
    "expiresAt": "2026-06-29T13:00:00Z",
    "reasons": [],
    "steps": [],
    "blocked": false
  },
  "warnings": [],
  "errors": []
}
```

## Plan Store

계획은 생성 시 파일로 저장한다.

우선순위:

```text
1. INFRA_LAB_PLAN_STORE
2. INFRA_LAB_ROOT/state/.plans/
```

파일명:

```text
<planId>.json
```

plan store에 쓸 수 없으면 plan tool은 실패한다.

## Risk Policy

```text
LOW:
  read-only 또는 단순 계획

MEDIUM:
  env_up, addon_install

HIGH:
  env_down, rebuild, addon_uninstall
```

## Approval

현재 Stage 3은 실행하지 않는다.
모든 mutation plan은 `requiresApproval:true`를 반환한다.
