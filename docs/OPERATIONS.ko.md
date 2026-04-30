# 운영 표준

## 목적

이 문서는 `infra-lab`을 실제 운영할 때 사용할 표준 진입점과 명령을 고정합니다.

핵심 원칙:

- 랩 인프라 조작은 항상 `infra-lab/scripts/k8s-tool.sh`를 통해 수행합니다.
- 기본 운영 대상 host 는 `seoy@100.123.80.48`입니다.
- 로컬 워크스테이션 `100.92.45.46`에서는 VM lifecycle 명령을 직접 실행하지 않습니다.
- JUMI, AH, `kube-slint` 같은 상위 작업도 클러스터 준비와 상태 확인은 이 저장소를 기준으로 수행합니다.

## 표준 host profile

기본 프로파일:

- [hosts/remote-lab.env](/opt/go/src/github.com/HeaInSeo/infra-lab/hosts/remote-lab.env:1)

이 프로파일은 아래 값을 고정합니다.

- `LAB_HOST_MODE=remote`
- `LAB_REMOTE_SSH_TARGET=seoy@100.123.80.48`
- `LAB_REMOTE_REPO_PATH=/opt/go/src/github.com/HeaInSeo/infra-lab`
- `BACKEND=multipass`

libvirt 경로를 사용할 때는 명령 호출 시 `BACKEND=libvirt`를 덮어씁니다.

## 표준 명령

원격 랩 상태 확인:

```bash
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh status
```

원격 랩 host 준비 확인:

```bash
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh host-setup
HOST_PROFILE=hosts/remote-lab.env BACKEND=libvirt ./scripts/k8s-tool.sh host-setup
```

원격 Multipass 랩 생성:

```bash
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh up
```

원격 libvirt 랩 생성:

```bash
HOST_PROFILE=hosts/remote-lab.env BACKEND=libvirt ./scripts/k8s-tool.sh up
```

원격 랩 제거:

```bash
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh down
HOST_PROFILE=hosts/remote-lab.env BACKEND=libvirt ./scripts/k8s-tool.sh down
```

애드온 검증:

```bash
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh addons-verify
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh addons-verify optional metallb
```

## 상위 저장소와의 경계

JUMI, AH, `kube-slint` 같은 상위 저장소는 이 문서를 기준으로 랩 클러스터를 사용합니다.

표준 흐름:

1. `infra-lab`에서 host profile 기반으로 클러스터를 준비합니다.
2. `infra-lab`에서 `status`로 노드와 시스템 파드를 확인합니다.
3. 그 다음 상위 저장소 작업을 진행합니다.

금지할 것:

- 상위 저장소에서 직접 `multipass`, `virsh`, `tofu apply`를 호출해 랩을 조작하는 것
- 로컬 워크스테이션에서 직접 VM lifecycle 을 실행하는 것
- `multipass-k8s-lab` 경로명을 새 기준처럼 계속 사용하는 것

## 운영 규칙

- 기준 저장소 경로는 `/opt/go/src/github.com/HeaInSeo/infra-lab`입니다.
- 기준 브랜치는 `main`입니다.
- 랩 lifecycle 검증은 가능하면 host profile 을 명시한 `k8s-tool.sh` 경유로 통일합니다.
- backend 별 smoke test 결과와 장애 이력은 `docs/TROUBLESHOOTING_HISTORY.ko.md`에 계속 누적합니다.
