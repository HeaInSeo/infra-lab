# infra-lab 상세 설명

## 1. 이 프로젝트가 무엇인가

`infra-lab`은 특정 애플리케이션 저장소가 아니라, 여러 Kubernetes 실험을 올릴 수 있는 공용 VM 기반 랩 인프라 저장소다.

핵심 목적:

- 반복 가능한 VM 기반 Kubernetes 랩 제공
- kubeadm 기반 baseline 유지
- host / backend / runtime 차이를 최소한의 공용 진입점으로 정리
- 여러 상위 저장소가 같은 랩을 공용으로 재사용할 수 있게 만들기

즉, 이 저장소는 “어떤 앱을 배포하는 저장소”가 아니라 “앱과 실험이 올라갈 랩 플랫폼 저장소”다.

## 2. 왜 따로 존재하는가

이 저장소가 분리되어 있는 이유는 책임 경계 때문이다.

이 저장소가 맡는 것:

- VM lifecycle
- kubeadm bootstrap
- kubeconfig export
- host profile
- addon install / verify
- lab 운영 표준화

이 저장소가 맡지 않는 것:

- 프로젝트별 app 배포 로직
- 제품 business logic
- Cilium 자체 구현
- 서비스별 운영 매뉴얼 전부

상위 PoC나 앱 저장소는 이 랩을 “소비”해야 하고, 랩을 직접 제어하는 코드까지 자기 안에 품지 않는 쪽이 유지보수에 유리하다.

## 3. 현재 아키텍처

현재 모델은 대략 네 층으로 보면 된다.

1. control host
2. lab host
3. VM backend
4. kubeadm cluster

### control host

사용자가 명령을 시작하는 위치다.

- 로컬 워크스테이션일 수 있다.
- 원격 host에 SSH로 명령을 프록시할 수도 있다.

### lab host

실제 VM과 kubeconfig, host-level 브리지, Cilium LB 접근 경로를 가진 장비다.

현재 운영 기준 host:

- `100.123.80.48`

### VM backend

현재 지원 backend:

- `multipass`
- `libvirt`

backend는 VM 생성/삭제와 state 위치에 영향을 준다.

### kubeadm cluster

cluster baseline은 kubeadm 위에 만들어진 소규모 lab cluster다.

현재 대표 형태:

- `1 control-plane + 2 workers`

## 4. 표준 진입점

이 저장소의 표준 진입점은 [scripts/k8s-tool.sh](/opt/go/src/github.com/HeaInSeo/infra-lab/scripts/k8s-tool.sh:1)다.

왜 이것이 중요한가:

- host mode local/remote를 숨긴다.
- backend별 상태 디렉터리를 숨긴다.
- 상위 저장소가 직접 `multipass`, `virsh`, `tofu`를 만지는 것을 줄인다.
- 운영 절차를 한 곳에 모은다.

대표 명령:

- `./scripts/k8s-tool.sh up`
- `./scripts/k8s-tool.sh down`
- `./scripts/k8s-tool.sh status`
- `./scripts/k8s-tool.sh addons-verify`

운영 문서:

- [docs/OPERATIONS.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/OPERATIONS.ko.md:1)

## 5. host profile 개념

host profile은 “어디에서 어떤 랩을 조작하는가”를 고정하는 환경 파일이다.

대표 파일:

- [hosts/remote-lab.env](/opt/go/src/github.com/HeaInSeo/infra-lab/hosts/remote-lab.env:1)

이 파일은 다음을 고정한다.

- `LAB_HOST_MODE=remote`
- `LAB_REMOTE_SSH_TARGET=seoy@100.123.80.48`
- `LAB_REMOTE_REPO_PATH=/opt/go/src/github.com/HeaInSeo/infra-lab`
- `BACKEND=multipass`

이 개념 덕분에 사용자는 “원격 host에서 직접 명령을 쳐야 하는지”를 매번 다시 생각하지 않고도 같은 엔트리포인트를 쓸 수 있다.

## 6. addon 모델

addon은 base와 optional로 나뉜다.

### base

대부분의 lab에 기본적으로 유용한 것만 둔다.

현재 대표:

- `metrics-server`

### optional

특정 실험에 필요하지만 모든 환경의 기본값으로 강제하고 싶지 않은 항목이다.

예:

- `local-path-storage`
- `metallb`
- `cilium`

중요한 점은, optional이라고 해서 “덜 중요하다”는 뜻이 아니라 “환경별 선택성이 크다”는 뜻이다.

## 7. Cilium이 여기서 차지하는 위치

Cilium은 이 저장소의 baseline에 항상 강제되는 공통 기능이 아니다.

현재 위치:

- generic addon 경로가 존재한다.
- `remote-seoy`에서는 실제 운영 중인 기준선이 존재한다.
- 그 기준선은 별도 profile로 문서화했다.

핵심 문서:

- [docs/CILIUM.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/CILIUM.ko.md:1)
- [docs/CILIUM_REMOTE_SEOY.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/CILIUM_REMOTE_SEOY.ko.md:1)
- [profiles/remote-seoy/cilium/README.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/README.ko.md:1)

## 8. 왜 profile이 필요한가

현재 드러난 핵심 문제는 “generic 예제”와 “실제 운영 상태”가 다르다는 점이다.

예를 들어:

- generic addon 값은 `ipam.mode: kubernetes`
- `remote-seoy` live는 `cluster-pool + vxlan`

이 둘을 하나의 공용 파일로 억지로 통합하려 하면 위험하다.

그래서 profile은 다음 역할을 한다.

- 특정 환경의 실제 기준선을 고정
- generic 예제와 live 운영 상태를 분리
- desired baseline과 experimental 시도를 분리
- 상위 app가 특정 IPAM이나 lab 구현에 묶이지 않게 보호

## 9. 제품/클라우드 관점에서의 원칙

현재 `remote-seoy` Cilium 조합은 lab/on-prem-like 환경에 적합하다.

- `cluster-pool`
- `vxlan`
- `LB IPAM`
- `L2 announcement`
- `Gateway API`

하지만 이것이 제품 app의 전제가 되면 안 된다.

app core는 다음 표준 추상화에만 기대야 한다.

- `Service DNS`
- `Service`
- `Gateway API`

즉, app은 “Cilium cluster-pool을 쓰는지”, “MetalLB인지”, “cloud LoadBalancer인지”를 몰라도 돌아가야 한다.

## 10. 이 프로젝트를 읽는 순서

처음 보는 사람에게는 이 순서를 권한다.

1. [README.md](/opt/go/src/github.com/HeaInSeo/infra-lab/README.md:1)
2. [docs/LAB_SCOPE.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/LAB_SCOPE.ko.md:1)
3. [docs/ARCHITECTURE.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/ARCHITECTURE.ko.md:1)
4. [docs/OPERATIONS.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/OPERATIONS.ko.md:1)
5. [docs/CILIUM.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/CILIUM.ko.md:1)
6. [docs/CILIUM_REMOTE_SEOY.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/CILIUM_REMOTE_SEOY.ko.md:1)

## 11. 지금 기준선에서 중요한 판단

현재 기준선에서 가장 중요한 판단은 이것이다.

- Cilium service mesh 기능을 넣는 것이 목표가 아니다.
- live에서 이미 돌아가는 `Gateway-only baseline`을 레포 안에 안전하게 정리하는 것이 목표다.

이 판단이 흐려지면 다음 문제가 생긴다.

- live drift 정리와 실험 기능 추가가 섞임
- IPAM 변경 같은 위험한 작업이 baseline 정리로 위장됨
- workload-specific route가 공용 infra baseline으로 섞임

따라서 지금은 작게, 보수적으로, 재현 가능하게 정리하는 편이 맞다.
