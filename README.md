# multipass-k8s-lab

한국어: [README.md](README.md)
English: [README.en.md](README.en.md)

`multipass-k8s-lab`은 로컬 및 워크스테이션급 PoC 작업을 위한 재사용 가능한 VM 기반 Kubernetes 랩 베이스라인입니다. 현재 베이스라인은 의도적으로 범위를 좁게 유지합니다. Multipass + Ubuntu 24.04 게스트 + OpenTofu + kubeadm 조합을 사용하며, 반복 가능한 3노드 클러스터 구성과 소수의 인프라 애드온만 포함합니다.

이 저장소는 단일 프로젝트용 환경이 아닙니다. node-local artifact 및 스토리지 흐름, DaemonSet 스타일 노드 에이전트, same-node 재사용과 cross-node fetch 비교, Cilium 및 네트워킹 실험, 스토리지 테스트, 오퍼레이터 검증 등 앞으로의 다양한 Kubernetes 실험을 위한 공용 랩 인프라 기반입니다.

## 정체성

- 목적: 범용 K8s VM 랩 인프라
- 현재 베이스라인: Ubuntu 24.04 게스트 + Multipass + OpenTofu + kubeadm
- 첫 번째 목표 형태: `1 control-plane + 2 workers`인 3 VM
- 라이프사이클 지원: 호스트 준비, 클러스터 up/down, 상태 확인, 로컬 정리
- 애드온 모델: `base`와 `optional`
- 향후 확장 지점: 이 저장소를 워크로드 저장소로 바꾸지 않으면서 `profiles/`를 통한 프로젝트별 오버레이 지원

## 이 저장소가 담당하는 것

- kubeadm 기반 랩 클러스터를 위한 Multipass VM 라이프사이클
- 베이스라인 클러스터 부트스트랩과 kubeconfig 내보내기
- 반복 가능한 로컬 랩 사용을 위한 소규모 운영 스크립트
- 클러스터 인프라 기능을 위한 최소 애드온 계층
- 범위, 진입점, 베이스라인 운영 모델에 대한 문서

## 이 저장소가 담당하지 않는 것

- 특정 프로젝트의 애플리케이션 배포
- artifact-agent, catalog, storage app, Cilium 자체 구현
- 프로덕션 클러스터 프로비저닝 또는 프로덕션 수준 하드닝
- 메인 저장소에 깊게 결합된 프로젝트 전용 워크로드

자세한 내용: [docs/LAB_SCOPE.ko.md](docs/LAB_SCOPE.ko.md)

## 빠른 시작

1. 호스트를 준비합니다.

```bash
./scripts/k8s-tool.sh host-setup
```

2. 기본값을 확인합니다.

```bash
sed -n '1,200p' dev.auto.tfvars
```

3. 베이스라인 클러스터를 생성합니다.

```bash
./scripts/k8s-tool.sh up
```

4. 상태를 확인합니다.

```bash
./scripts/k8s-tool.sh status
```

5. base 애드온을 설치합니다.

```bash
./scripts/k8s-tool.sh addons-install base
./scripts/k8s-tool.sh addons-verify
```

6. 종료합니다.

```bash
./scripts/k8s-tool.sh down
```

7. 로컬 상태 파일을 제거합니다.

```bash
FORCE=1 ./scripts/k8s-tool.sh clean
```

## 베이스라인 실행 흐름

### 호스트 설정

```bash
./scripts/k8s-tool.sh host-setup
```

설치하거나 확인하는 항목:

- OpenTofu
- Multipass
- Python 3
- 선택 항목인 `kubectl`
- 선택 항목인 `helm`

### 클러스터 생성

```bash
./scripts/k8s-tool.sh up
```

수행 내용:

- OpenTofu 초기화 및 로컬 리소스 적용
- Multipass로 Ubuntu 24.04 VM 실행
- 첫 control-plane 노드에서 `kubeadm init` 실행
- worker 노드 조인
- 로컬 `./kubeconfig` 내보내기

### 상태 확인

```bash
./scripts/k8s-tool.sh status
```

`./kubeconfig`가 존재하고 `kubectl`이 있으면 노드 및 파드 상태를 출력합니다. 그렇지 않으면 `multipass list`로 폴백합니다.

### 종료

```bash
./scripts/k8s-tool.sh down
```

VM과 OpenTofu가 관리하는 관련 로컬 리소스를 제거합니다.

### 정리

```bash
FORCE=1 ./scripts/k8s-tool.sh clean
```

`.terraform/`, `*.tfstate`, `./kubeconfig` 같은 로컬 상태 파일을 제거합니다. 호스트 패키지는 제거하지 않습니다.

## 애드온

이 저장소는 인프라 애드온을 두 범주로 구분합니다.

### Base

Base 애드온은 랩 클러스터의 합리적인 기본값이며 특정 PoC에 저장소를 종속시키지 않습니다.

- `metrics-server`

### Optional

Optional 애드온은 특정 랩 형태에서 유용하지만 항상 켜 두기보다는 명시적으로 설치해야 하는 항목입니다.

- `local-path-storage`
- `metallb`

예시:

```bash
./scripts/k8s-tool.sh addons-install base
./scripts/k8s-tool.sh addons-install optional local-path-storage
./scripts/k8s-tool.sh addons-install optional metallb
./scripts/k8s-tool.sh addons-verify
```

`metallb`를 사용하기 전에 [addons/values/metallb/ipaddresspool.yaml](addons/values/metallb/ipaddresspool.yaml)을 검토해야 합니다.

## 디렉터리 구성

```text
.
├── addons/
│   ├── base/
│   ├── optional/
│   ├── values/
│   └── manage.sh
├── cloud-init/
├── docs/
├── profiles/
├── scripts/
│   ├── host/
│   ├── multipass/
│   ├── cluster/
│   └── k8s-tool.sh
├── main.tf
├── variables.tf
├── versions.tf
└── dev.auto.tfvars
```

## `mac-k8s-multipass-terraform`과 다른 이유

이 저장소는 이전 프로젝트를 참고하지만 서비스 지향 범위를 그대로 가져오지는 않습니다. 기존 저장소는 클러스터 베이스라인 작업과 서비스 설치 흔적, 더 넓은 스택 애드온이 섞여 있었습니다. 이 저장소는 VM과 kubeadm 라이프사이클 패턴은 유지하면서도 책임 범위를 재사용 가능한 랩 인프라로 다시 좁혔습니다.

## 향후 방향

- 대체 네트워킹 프로필로서 Cilium
- 로컬 스토리지 및 local PV 실험용 헬퍼
- node-local agent 실험용 헬퍼
- operator 검증 프로필
- `profiles/` 또는 향후 `labs/` 규칙 아래의 프로젝트 오버레이

## 참고

- 첫 번째 베이스라인에서는 빠른 부트스트랩을 위해 기본 CNI로 Flannel을 사용합니다.
- Cilium은 의도적으로 향후 프로필 또는 선택적 경로로 미뤘으며, 베이스라인에 강제로 포함하지 않습니다.
- 현재 환경에서 공개 Multipass 이미지 카탈로그가 Ubuntu 중심이기 때문에 게스트 베이스라인은 Ubuntu 24.04를 사용합니다.
- 호스트 헬퍼 스크립트는 여전히 기존 Rocky 8 워크스테이션 설정 흐름을 기준으로 하고 있습니다.
- 이전 Rocky 8 경로도 이번 스프린트에서 봤던 것과 같은 종류의 control-plane 불안정을 겪을 수 있습니다. static pod가 반복 재생성되거나 API server가 불안정하면 `etcd`나 `kube-apiserver` 매니페스트를 먼저 바꾸기 전에 `containerd --version`, `systemctl show -p ExecStart containerd`, `/etc/containerd/config.toml`, `crictl info`, kubelet `cgroupDriver`를 확인하십시오.
- 실제 장애 흐름과 회복 순서는 [docs/TROUBLESHOOTING_HISTORY.ko.md](docs/TROUBLESHOOTING_HISTORY.ko.md)에 정리했습니다.
