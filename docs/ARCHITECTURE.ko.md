# 아키텍처

## 목표

`infra-lab`은 단일 호스트, 단일 런타임 전용 랩에서 여러 호스트와 둘 이상의 VM 런타임을 수용할 수 있는 재사용 가능한 베이스라인으로 확장되는 중입니다.

현재 구현 범위는 의도적으로 아래 두 단계까지만 다룹니다.

1. host 비의존
2. VM runtime 비의존

완전한 범용 IaC 프레임워크를 목표로 하지는 않습니다.

## 계층

### Host 모델

control host 와 실제 lab host 는 서로 다른 장비일 수 있습니다.

- `LAB_HOST_MODE=local`
  현재 장비에서 OpenTofu 와 헬퍼 스크립트를 직접 실행합니다.
- `LAB_HOST_MODE=remote`
  SSH 를 통해 원격 체크아웃에서 같은 `scripts/k8s-tool.sh` 명령을 실행합니다.

핵심 설정:

- `HOST_PROFILE`
- `LAB_REMOTE_SSH_TARGET`
- `LAB_REMOTE_REPO_PATH`

### Backend 모델

선택한 backend 가 OpenTofu state 위치와 VM runtime 을 결정합니다.

- `BACKEND=multipass`
  저장소 루트 state 와 기존 Multipass 흐름을 사용합니다.
- `BACKEND=libvirt`
  `backends/libvirt/` state 와 libvirt provider 흐름을 사용합니다.

### Transport 모델

클러스터 bootstrap 스크립트는 특정 VM runtime 호출 방식에 직접 묶이지 않아야 합니다.

runtime transport layer 는 `scripts/runtime/` 아래에 있습니다.

- `VM_RUNTIME=multipass`
- `VM_RUNTIME=ssh`

현재 사용 방식:

- Multipass backend 는 bootstrap/join/export 에 Multipass transport 를 사용합니다.
- libvirt backend 는 DHCP lease 확보 후 SSH transport 로 bootstrap 합니다.

## 디렉터리 모델

```text
.
├── backends/
│   └── libvirt/
├── hosts/
├── scripts/
│   ├── runtime/
│   ├── multipass/
│   ├── cluster/
│   └── host/
└── docs/
```

## 지원 조합

### Multipass backend

- host mode: local 또는 remote
- runtime: Multipass
- state path: 저장소 루트

### libvirt backend

- host mode: local 또는 remote
- runtime: libvirt provider + SSH bootstrap
- state path: `backends/libvirt/`
- 권한 모델: `qemu:///system`을 쓰면 호스트 사용자에게 libvirt 관리 권한이 없는 경우 `sudo`가 필요할 수 있음
- 준비 완료 기준: DHCP lease 확보 후, 조인된 모든 노드가 `Ready`가 될 때까지 대기

## 현재 남아 있는 공백

- `scripts/cluster/flannel-to-cilium.sh` 같은 일부 헬퍼는 아직 multipass 중심입니다
- backend 별 변수 이름이 아직 완전히 정규화되지는 않았습니다
- kubeconfig 는 별도 복사 작업이 없으면 실행 host 에 남습니다
