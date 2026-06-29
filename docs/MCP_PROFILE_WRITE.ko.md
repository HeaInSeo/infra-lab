# infra-lab MCP Profile Write

이 문서는 Stage 4 profile write MCP 도구를 설명한다.

Stage 4는 profile 파일만 생성한다.
VM, Kubernetes, tofu state, kubeconfig는 변경하지 않는다.

## Tools

```text
infra_lab.profile_save_as
infra_lab.profile_validate_and_save
infra_lab.profile_clone
```

## 정책

```text
- 저장 위치는 기본적으로 ~/.config/infra-lab/profiles/ 이다.
- INFRA_LAB_PROFILE_DIR로 테스트용 저장 위치를 지정할 수 있다.
- repo envs/에는 쓰지 않는다.
- 기존 profile은 덮어쓰지 않는다.
- 저장 전 ilab profile validate <path> --json으로 검증한다.
- 검증 실패 시 최종 profile 파일을 남기지 않는다.
- write 성공 시 audit log를 남긴다.
- audit 실패 시 profile write는 실패한다.
```

Audit path 우선순위:

```text
1. INFRA_LAB_AUDIT_PATH
2. INFRA_LAB_ROOT/state/.audit/operations.jsonl
3. INFRA_LAB_CONFIG_HOME/audit/operations.jsonl
4. XDG_CONFIG_HOME/infra-lab/audit/operations.jsonl
5. ~/.config/infra-lab/audit/operations.jsonl
```

## profile_save_as

새 profile을 생성한다.
입력하지 않은 값은 안전한 local lab 기본값을 사용한다.

예:

```json
{
  "name": "codex-flannel",
  "backend": "multipass",
  "cni": "flannel",
  "masters": 1,
  "workers": 2,
  "osImage": "ubuntu-24.04"
}
```

기본 생성값:

```text
backend: multipass
cni: flannel
masters: 1
workers: 2
osImage: ubuntu-24.04
state.dir: state/<name>
```

## profile_validate_and_save

현재 구현에서는 `profile_save_as`와 같은 저장 경로를 사용한다.
의도는 명확한 validate-and-save tool 이름을 MCP client에 제공하는 것이다.

## profile_clone

기존 profile YAML을 복제해 새 이름으로 저장한다.
복제 시 `name`과 `state.dir`은 새 profile 기준으로 바꾼다.

예:

```json
{
  "source": "multipass-flannel",
  "name": "codex-flannel-clone"
}
```

source 조회 순서:

```text
1. explicit path
2. user profile dir
3. repo envs/<source>.yaml
4. repo envs/<source>.yaml.example
```

## 응답 예시

```json
{
  "ok": true,
  "command": "profile.save_as",
  "contractVersion": "infra-lab.contract/v1",
  "data": {
    "operationId": "op_20260629_010000_profile_save_as",
    "profile": {
      "name": "codex-flannel",
      "source": "user",
      "path": "/home/user/.config/infra-lab/profiles/codex-flannel.yaml"
    },
    "validation": {
      "ok": true,
      "command": "profile.validate"
    },
    "auditPath": "state/.audit/operations.jsonl"
  },
  "warnings": [],
  "errors": []
}
```

## 직접 테스트

```bash
make build
make build-mcp
make test-mcp
```

MCP client에서 다음을 호출한다.

```text
infra_lab.profile_save_as
infra_lab.profile_clone
```
