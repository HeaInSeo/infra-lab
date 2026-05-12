# remote-seoy Cilium profile

이 디렉터리는 `remote-seoy` VM lab 환경의 현재 Cilium 운영 기준선을 레포 안에 정착시키기 위한 profile이다.

기준 환경:

- host: `100.123.80.48`
- cluster role: 테스트용 VM Kubernetes lab
- 목적: 이미 운영 중인 Cilium 상태를 재현 가능한 profile로 정리하고, 기존 문서/manifest와 live drift를 줄이기

현재 이 profile의 목표는 `Gateway-only baseline`이다.

- 현재 Cilium 역할은 `CNI + kube-proxy replacement + LB IPAM + L2 announcement + Gateway API ingress` 이다.
- east-west service mesh는 아직 구성되어 있지 않다.
- `GRPCRoute`는 north-south gRPC ingress 후보이지, 내부 메시 자체가 아니다.
- `GAMMA`는 Service `parentRef` 기반 east-west `HTTPRoute` 실험으로 `experimental/` 아래에 분리한다.
- mutual auth / SPIRE / mTLS는 아직 운영 기준선에 넣지 않는다.

Route 분류 원칙:

- `desired/`에는 infra baseline으로 재현하고 싶은 공용 Route만 넣는다.
- 현재는 `harbor-route`와 `dev-space-observability`만 desired baseline에 포함한다.
- `shift-left-observability`처럼 특정 시점 workload가 추가한 Route는 `live-snapshot/`에는 기록하되, baseline desired에는 자동 편입하지 않는다.

IPAM 관련 안전 원칙:

- `remote-seoy` live는 현재 `cluster-pool + vxlan` 기준으로 동작한다.
- 이 profile은 그 사실을 기록하고 재현 기준으로 남기지만, 운영 중인 기존 클러스터의 IPAM mode 변경 경로를 제공하지 않는다.
- 기존 클러스터에서 `ipam.mode`를 바꾸는 `helm upgrade` 경로는 금지한다.
- IPAM 변경이 필요하면 새 클러스터 또는 새 profile 설계로 다뤄야 한다.

디렉터리 역할:

- `live-snapshot/`: 현재 운영 증거. 수집 시점의 read-only 출력과 YAML 스냅샷.
- `desired/`: 향후 `remote-seoy`에서 재현하려는 선언형 기준선. 신규 재현 기준이지, 기존 클러스터에 곧바로 업그레이드하기 위한 값이 아니다.
- `verify/`: read-only 수집 및 검증 스크립트.
- `experimental/`: 아직 운영 기준선에 포함하지 않는 실험 메모.
- `cloud-profiles/`: 향후 cloud profile 분리 방향.

운영 진입점:

- `bash scripts/k8s-tool.sh profile-cilium-collect`
- `bash scripts/k8s-tool.sh profile-cilium-verify`
- `bash scripts/k8s-tool.sh profile-gateway-verify`

`HOST_PROFILE=hosts/remote-lab.env` 와 `LAB_HOST_MODE=remote` 조합으로 실행하면 remote host를 통해 같은 read-only 절차를 태울 수 있다.

제품/클라우드 관점:

- infra-lab에서는 현재 `cluster-pool + vxlan + LB IPAM + L2 announcement + Gateway API` 조합이 lab 성격에 맞다.
- 하지만 제품 app core는 특정 Cilium IPAM 모드에 의존하면 안 된다.
- app core는 `Kubernetes Service DNS / Service / Gateway API` 같은 표준 추상화 위에서 동작해야 한다.
- 퍼블릭 클라우드용 profile은 EKS/AKS/GKE별 CNI/IPAM/Gateway/LoadBalancer 모델을 별도로 검토해야 한다.
