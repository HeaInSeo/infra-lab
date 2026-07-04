# infra-lab

English: [README.md](README.md)

로컬 및 워크스테이션급 작업을 위한 재사용 가능한 VM 기반 Kubernetes 랩이다.
OpenTofu + kubeadm으로 `multipass`와 `libvirt` 두 가지 backend에서 클러스터 라이프사이클을 관리한다.
`ilab`이라는 읽기 전용 CLI가 포함되어 있어 환경, VM, 클러스터 상태를 조회할 수 있다.

[![CI](https://github.com/HeaInSeo/infra-lab/actions/workflows/check.yml/badge.svg)](https://github.com/HeaInSeo/infra-lab/actions/workflows/check.yml)
[![Release](https://img.shields.io/github/v/release/HeaInSeo/infra-lab)](https://github.com/HeaInSeo/infra-lab/releases/latest)

## 사용자 설명서

- MCP 최종 사용자 설명서: [docs/MCP_USER_GUIDE.ko.md](docs/MCP_USER_GUIDE.ko.md)
- 원격 운영 기준: [docs/OPERATIONS.ko.md](docs/OPERATIONS.ko.md)
- profile 기반 환경 관리: [docs/PROFILE_DRIVEN_ENVIRONMENTS.ko.md](docs/PROFILE_DRIVEN_ENVIRONMENTS.ko.md)

## 이 repo가 담당하는 것

- 선택한 backend를 통한 kubeadm 클러스터 VM 라이프사이클
- 클러스터 부트스트랩, kubeconfig 내보내기, base addon 설치
- `state/<env>/` 아래 환경별 state 격리
- 환경 조회를 위한 `ilab` CLI
- CI 품질 게이트: shell, HCL, YAML, Go

## 이 repo가 담당하지 않는 것

- 애플리케이션 및 워크로드 배포
- 프로덕션 하드닝 또는 프로덕션 클러스터 프로비저닝
- repo에 깊게 결합된 프로젝트 전용 설정

## 빠른 시작

### 1. 환경 프로파일 선택

```bash
cp envs/multipass-flannel.env.example envs/multipass-flannel.env
# libvirt라면 TF_VAR_ssh_private_key_path, TF_VAR_ssh_public_key 값 입력
cp envs/libvirt-cilium.env.example envs/libvirt-cilium.env
```

프로파일은 `envs/`에 두고 `BACKEND`, `CNI`, `ADDONS`, backend별 `TF_VAR_*` 값을 설정한다.
실제 값이 들어간 `.env` 파일은 gitignore 처리되고, `.env.example`만 커밋된다.

### 2. 호스트 준비

```bash
HOST_PROFILE=envs/multipass-flannel.env ./scripts/k8s-tool.sh host-setup
```

### 3. 클러스터 생성

```bash
ENV_PROFILE=envs/multipass-flannel.env make env-up
# 또는 직접:
HOST_PROFILE=envs/multipass-flannel.env ./scripts/k8s-tool.sh up
```

State는 `state/multipass-flannel/` 아래에 격리된다:

```
state/multipass-flannel/
  terraform.tfstate   ← OpenTofu state
  kubeconfig          ← 클러스터 접근
  meta                ← 생성 메타데이터 (git commit, backend, CNI, 타임스탬프)
```

### 4. 상태 확인

```bash
ENV_PROFILE=envs/multipass-flannel.env make env-status
# 또는 ilab으로:
ilab env status
ilab k8s status
```

### 5. 종료 및 정리

```bash
ENV_PROFILE=envs/multipass-flannel.env make env-down
FORCE=1 ENV_PROFILE=envs/multipass-flannel.env make env-clean
```

## Backend

| Backend | 비고 |
|---------|------|
| `multipass` | 초기 설정 시 권장. 호스트에 Multipass 필요. |
| `libvirt` | `virsh`, `qemu-img`, libvirt 접근 권한, SSH 키 변수 필요. |

**libvirt** 추가 변수 (프로파일이나 환경변수로 설정):

```bash
TF_VAR_ssh_private_key_path=/home/your-user/.ssh/id_ed25519
TF_VAR_ssh_public_key=ssh-ed25519 AAAA...
```

## CNI

환경 프로파일에 `CNI=` 값을 설정한다:

| 값 | 동작 |
|----|------|
| `flannel` | 기본값. 클러스터 부트스트랩 중 설치됨. |
| `cilium` | 먼저 Flannel을 설치하고, `up` 완료 후 `flannel-to-cilium.sh` 마이그레이션이 자동 실행됨. 현재는 `multipass` backend만 지원. |

## ilab CLI

infra-lab 환경을 조회하는 읽기 전용 운영 인터페이스다.
state를 직접 소유하지 않으며, source of truth는 OpenTofu state, VM, Kubernetes API다.

```bash
make build        # → bin/ilab
make install      # → $(go env GOPATH)/bin/ilab
```

### 명령

```bash
ilab doctor                     # 현재 환경 상태 진단
ilab env list                   # state/ 아래 환경 목록
ilab env status [env]           # 클러스터/VM 상태 확인
ilab vm list                    # 관리 VM 목록
ilab vm list --all              # 비관리 VM 포함 전체 목록
ilab vm version <vm>            # VM에서 /etc/infra-lab/build.json 읽기
ilab vm ssh <vm>                # VM에 인터랙티브 shell 접속
ilab k8s status [env]           # kubectl nodes + pods
```

현재 디렉터리에서 위로 올라가며 repo root를 자동 탐색한다.
`INFRA_LAB_ROOT=/path/to/infra-lab`으로 오버라이드 가능.

### VM 빌드 메타데이터

`env-up` 완료 후 각 VM에 `/etc/infra-lab/build.json`이 기록된다:

```json
{
  "schemaVersion": "infra-lab.vm.v1",
  "infraLabGitCommit": "abc1234",
  "infraLabGitBranch": "main",
  "envName": "multipass-flannel",
  "backend": "multipass",
  "cni": "flannel",
  "role": "control-plane",
  "nodeName": "lab-master-0",
  "kubernetesVersion": "v1.36.2",
  "createdAt": "2026-06-05T00:00:00Z"
}
```

`ilab vm version <node>`으로 이 VM을 어떤 infra-lab 버전과 설정으로 만들었는지 확인할 수 있다.

## Addon

두 가지 범주로 구분한다.

### Base (`up` 후 자동 설치)

- `metrics-server` — 이 랩의 인증서 형태에 맞게 `--kubelet-insecure-tls`와 함께 설치

### Optional (명시적 설치)

- `local-path-storage` — v0.0.28 고정
- `metallb` — 사용 전 `addons/values/metallb/ipaddresspool.yaml` 검토 필요
- `cilium` — 프로파일에서 `CNI=cilium`으로 마이그레이션 경로를 통해 설치, 직접 addon install 금지

```bash
./scripts/k8s-tool.sh addons-install optional local-path-storage
./scripts/k8s-tool.sh addons-verify optional local-path-storage
```

## Kubernetes User Namespace 기준

이 랩의 Kubernetes baseline은 `1.36.2`로 고정한다. NodeVault 같은 상위
프로젝트가 Kubernetes User Namespace를 안정 API(`spec.hostUsers: false`)로
전제하기 위한 기준이다. Ubuntu 24.04 게스트는 현재 Linux 6.8 커널을 제공하고,
containerd는 Ubuntu repository에서 설치한다.

User Namespace 작업 전에 실제 VM 노드에서 아래를 확인한다:

```bash
kubectl get nodes -o wide
kubectl explain pod.spec.hostUsers
```

기대 baseline:

| 항목 | 기준 |
|------|------|
| Kubernetes | `v1.36.x` |
| Linux kernel | `6.3+` (Ubuntu 24.04 게스트는 `6.8.x`) |
| container runtime | `containerd 2.0+` |
| OCI runtime | `crun 1.28`을 containerd 기본 runtime으로 사용(`/usr/local/bin/crun`) |

Kubernetes 문서의 runtime 하한선은 더 낮지만, 이 lab은 `crun 1.28`로 고정한다.
Ubuntu 24.04 기본 `crun 1.14.1` 패키지는 이 VM 경로에서 containerd 재시작 후
Kubernetes 1.36 static pod sandbox 재생성에 실패했다
(`OCI runtime create failed: unknown version specified`). Bootstrap은 공식
`containers/crun` amd64 바이너리를 SHA256 검증 후 설치하고, 이를 containerd
기본 runtime으로 사용한다.

Harbor는 클러스터 내부 상태다. VM 클러스터를 rebuild하면 Harbor와 proxy cache
설정도 사라지므로, 매 rebuild 후 Harbor를 다시 설치하고 GHCR proxy cache를
검증한다:

```bash
source ~/.config/infra-lab/harbor-secrets.env
KUBECONFIG=state/<env>/kubeconfig bash scripts/host/harbor-install.sh
kubectl run ghcr-test \
  --image="${HARBOR_HOSTNAME}/ghcr-io/kube-vip/kube-vip:v0.8.9" \
  --restart=Never
kubectl describe pod ghcr-test | grep -E 'Pulled|Failed|Error'
kubectl delete pod ghcr-test
```

## Makefile 레퍼런스

```bash
# Lint
make check          # shell + yaml + hcl (기본값)
make lint-shell     # bash -n + shellcheck
make lint-yaml      # YAML 파싱 검사
make lint-hcl       # tofu fmt + tofu validate
make lint-go        # gofmt + go vet + go build
make test-go        # go test ./...

# 환경
ENV_PROFILE=envs/<name>.env make env-up
ENV_PROFILE=envs/<name>.env make env-down
ENV_PROFILE=envs/<name>.env make env-status
FORCE=1 ENV_PROFILE=envs/<name>.env make env-clean

# CLI
make build          # bin/ilab
make install        # GOPATH/bin/ilab
```

## 디렉터리 구성

```
.
├── addons/            # base/, optional/ addon 스크립트
│   ├── base/
│   ├── optional/
│   └── values/
├── backends/
│   └── libvirt/       # libvirt 전용 Terraform 모듈
├── bin/               # 빌드된 바이너리 (gitignored)
├── cloud-init/        # VM 부트스트랩 cloud-init 설정
├── docs/
├── envs/              # 환경 프로파일 (*.env.example만 커밋, *.env는 gitignored)
├── ilab/              # Go CLI 소스
├── profiles/          # 프로젝트별 오버레이와 기준선
├── scripts/
│   ├── cluster/       # cluster-init, join-all, flannel-to-cilium, write-build-json
│   ├── host/          # 호스트 설정/정리
│   ├── multipass/
│   ├── runtime/       # lib.sh, run-remote.sh
│   └── k8s-tool.sh   # 메인 진입점
├── state/             # 환경별 런타임 state (gitignored)
│   └── <env>/
│       ├── terraform.tfstate
│       ├── kubeconfig
│       └── meta
├── main.tf
├── variables.tf
├── versions.tf
└── dev.auto.tfvars
```

## State 격리

`HOST_PROFILE` 또는 `ENV_NAME`을 지정하면 모든 환경별 파일이 `state/<env>/` 아래에 격리된다.
프로파일 없이 실행하면 기존 방식(backend 디렉터리에 state)으로 동작한다 — Phase 2 이전 환경과 backward compatible하다.

`ilab doctor`로 현재 상태를 진단하고 다음 단계를 안내받을 수 있다.

K8sGPT CLI 기반 클러스터 진단 절차는 [docs/K8SGPT_CLI_REMOTE_DIAGNOSIS.ko.md](docs/K8SGPT_CLI_REMOTE_DIAGNOSIS.ko.md)를 참고한다.

## 재현성

- Kubernetes 패키지: `cloud-init/k8s.yaml`에서 `1.36.2` 고정
- `local-path-storage`: `v0.0.28` 고정
- Provider 버전: `.terraform.lock.hcl` 커밋으로 고정
- 게스트 이미지: Ubuntu 24.04 LTS

## CI

push 및 PR마다 두 개의 GitHub Actions workflow가 실행된다:

| Workflow | Jobs |
|----------|------|
| `check` | Shell, OpenTofu (fmt + validate), YAML, Go (gofmt + vet + build + test) |
| `workflow-lint` | actionlint, `ubuntu-latest` 금지 |

Runner는 `ubuntu-24.04` 고정. Dependabot이 매월 Actions 버전을 업데이트한다.
