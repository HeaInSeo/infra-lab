# infra-lab MCP Container Image Build/Push

이 문서는 agent가 infra-lab MCP로 컨테이너 이미지를 빌드하고 registry에 push할 때의 승인형 흐름을 정의한다.

## 범위

MCP는 raw shell, raw docker, raw podman proxy를 제공하지 않는다. 대신 승인된 operation만 다음 고정 실행을 수행한다.

```text
<builder> build -f <dockerfile> -t <image> <contextDir>
<builder> push <image>
```

지원 builder:

```text
podman
docker
```

`builder`를 생략하면 서버가 `podman`, `docker` 순서로 사용 가능한 binary를 선택한다.

## Tool

```text
container_image_build_push_prepare
operation_approve
container_image_build_push_commit
operation_status
operation_logs
```

prepare 입력:

```json
{
  "name": "nodevault-controlplane",
  "contextDir": "/opt/go/src/github.com/HeaInSeo/NodeVault",
  "dockerfile": "Dockerfile",
  "image": "harbor.lab.local/nodevault/controlplane:20260712",
  "builder": "podman"
}
```

`dockerfile`과 `builder`는 선택이다. `dockerfile` 기본값은 `Dockerfile`이다.

## 안전 정책

```text
- contextDir은 허용된 build root 하위여야 한다.
- 기본 허용 root는 infra-lab checkout의 상위 workspace다.
- 추가 허용 root는 INFRA_LAB_IMAGE_BUILD_ROOTS로 지정한다.
- dockerfile은 contextDir 내부 상대 경로만 허용한다.
- image는 registry host와 tag를 포함해야 한다.
- prepare 시 source fingerprint를 기록하고, commit 시 fingerprint가 달라지면 거부한다.
- commit은 approval token 또는 operation_approve를 요구한다.
- build/push stdout/stderr는 operation_logs로 조회한다.
```

기본 workspace 예:

```text
/opt/go/src/github.com/HeaInSeo
```

추가 root 예:

```text
INFRA_LAB_IMAGE_BUILD_ROOTS=/srv/build-contexts:/opt/projects
```

## NodeVault 예시

NodeVault controlplane 이미지를 Harbor에 push하는 흐름:

```text
container_image_build_push_prepare
operation_approve
container_image_build_push_commit
operation_status
operation_logs
```

권장 image tag:

```text
harbor.lab.local/nodevault/controlplane:<git-sha 또는 날짜 태그>
```

`latest`는 반복 테스트에는 편하지만, agent가 배포 이력을 추적해야 하는 작업에는 고정 tag를 권장한다.
