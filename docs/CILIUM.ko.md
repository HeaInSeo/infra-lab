# Cilium 운영 기준선 가이드

이 문서는 `infra-lab`에서 Cilium을 어떻게 다루는지에 대한 현재 기준을 정리한다.

가장 중요한 원칙은 두 가지다.

1. generic addon 경로와 특정 운영 환경의 live 기준선을 섞지 않는다.
2. 현재 목표는 `service mesh 추가`가 아니라 `remote-seoy`의 Gateway-only baseline을 레포 안에 안전하게 정착시키는 것이다.

관련 기준선:

- generic addon 예제: [addons/optional/cilium/install.sh](/opt/go/src/github.com/HeaInSeo/infra-lab/addons/optional/cilium/install.sh:1), [addons/values/cilium/values.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/addons/values/cilium/values.yaml:1)
- remote-seoy profile: [profiles/remote-seoy/cilium/README.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/README.ko.md:1)
- live 증거: [profiles/remote-seoy/cilium/live-snapshot/README.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/README.ko.md:1)
- desired baseline: [profiles/remote-seoy/cilium/desired/README.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/desired/README.ko.md:1)

## 1. 범위

이 저장소에서 Cilium은 현재 다음 역할까지만 운영 기준선에 포함한다.

- CNI
- kube-proxy replacement
- LB IPAM
- L2 announcement
- Gateway API ingress

이번 기준선에 포함하지 않는 것:

- east-west service mesh
- GAMMA
- mutual auth / SPIRE / mTLS
- ClusterMesh

즉, 현재 `infra-lab`의 Cilium은 `Gateway-only baseline`이다.

## 2. generic addon과 remote-seoy profile의 차이

generic addon 경로는 “처음부터 Cilium을 올릴 수 있는 공용 예제”다.

- [addons/values/cilium/values.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/addons/values/cilium/values.yaml:1)은 여전히 generic/default 예제다.
- 이 파일은 kubeadm `pod-network-cidr`를 따르는 `ipam.mode: kubernetes` 예제를 유지한다.
- 이 파일을 근거로 운영 중인 기존 클러스터의 IPAM mode를 바꾸면 안 된다.

반면 `remote-seoy`는 실제 운영 중인 VM lab의 기준선이다.

- host: `100.123.80.48`
- 목적: 테스트용 VM Kubernetes lab
- 성격: 외부 클라이언트가 Gateway / LoadBalancer IP로 직접 접근하는 on-prem-like lab

따라서 `remote-seoy`는 generic addon과 분리된 profile로 관리한다.

## 3. remote-seoy live 기준선

live 기준선은 [profiles/remote-seoy/cilium/live-snapshot](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot) 아래에 보관한다.

현재 확인된 핵심 상태:

- Cilium: `1.19.1`
- Kubernetes: `v1.32.13`
- IPAM: `cluster-pool`
- routing: `tunnel`
- tunnel protocol: `vxlan`
- kube-proxy replacement: `true`
- Gateway API: `enabled`
- Hubble: `enabled`
- LoadBalancer IP pool: `10.113.24.100/28`
- L2 announcement interface: `mpqemubr0`

현재 live 리소스:

- Gateway: `harbor/lab-gateway`
- HTTPRoute: `harbor-route`, `dev-space-observability`, `shift-left-observability`
- GRPCRoute: 없음
- CiliumNetworkPolicy / CiliumClusterwideNetworkPolicy: 없음

중요:

- 이 상태는 현재 운영 증거다.
- 레포 안의 generic manifest보다 우선하는 “remote-seoy 실제 기준선”은 profile 디렉터리에 있다.

## 4. desired baseline

`desired/`는 앞으로 `remote-seoy`를 재현할 때 참고할 선언형 기준이다.

핵심 파일:

- Helm values: [profiles/remote-seoy/cilium/desired/helm-values.gateway-only.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/desired/helm-values.gateway-only.yaml:1)
- Gateway: [profiles/remote-seoy/cilium/desired/gateway/lab-gateway.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/desired/gateway/lab-gateway.yaml:1)
- Routes: [profiles/remote-seoy/cilium/desired/routes](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/desired/routes)
- LB resources: [profiles/remote-seoy/cilium/desired/lb](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/desired/lb)

주의:

- 이 baseline은 기존 클러스터에 즉시 `helm upgrade` 하라는 뜻이 아니다.
- 신규 재현 또는 문서 기준선으로만 본다.
- 특히 `ipam.mode`는 기존 운영 클러스터에서 변경 대상이 아니다.

## 5. IPAM 안전 원칙

이 저장소에서 IPAM 관련 규칙은 강하게 지킨다.

1. live가 `cluster-pool`이라고 해서 generic addon 값을 바꾸지 않는다.
2. generic addon이 `ipam: kubernetes`라고 해서 live cluster를 바꾸지 않는다.
3. 운영 중인 기존 클러스터의 `ipam.mode`를 변경하는 `helm upgrade` 경로는 만들지 않는다.
4. IPAM 변경은 새 클러스터 또는 새 profile 설계로만 다룬다.

즉:

- generic addon은 generic addon으로 남긴다.
- remote-seoy live는 remote-seoy profile로 문서화한다.
- 둘 사이 차이는 “드리프트”이지만, 이번 작업에서는 “기준선 분리”로 해소한다.

## 6. Gateway-only baseline 설명

현재 운영 기준선의 Cilium Gateway 역할은 north-south ingress다.

구성 요소:

- `Gateway`
- `HTTPRoute`
- `LoadBalancer IP`
- `L2 announcement`

현재 live에서는 다음 구성이 기준이다.

- `harbor/lab-gateway`
- `HTTP:80`
- `HTTPS:443`
- `allowedRoutes.namespaces.from: All`
- `harbor-route`
- `dev-space-observability`

다만 `shift-left-observability`는 현재 live snapshot에 존재해도 공용 infra baseline으로는 아직 분류하지 않는다. 이는 “Cilium baseline의 일부”라기보다 “현재 workload가 Gateway에 추가로 부착한 Route”로 보는 편이 안전하다.

`GRPCRoute`는 현재 운영에 없다.

이 점이 중요하다.

- `GRPCRoute`는 north-south gRPC ingress 후보일 뿐이다.
- `GRPCRoute`가 있다고 해서 내부 service mesh가 구성되는 것은 아니다.
- 현재 baseline은 어디까지나 ingress baseline이다.

## 7. Harbor HTTPRoute와 ReferenceGrant

[k8s/harbor/01-httproute.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/k8s/harbor/01-httproute.yaml:1)의 기존 설명은 정정했다.

정리:

- `HTTPRoute`가 같은 namespace의 `Service`를 참조하는 것만으로 `ReferenceGrant`가 필요한 것은 아니다.
- cross-namespace parentRef 허용 여부는 `Gateway listener.allowedRoutes` 설정과 관련된다.
- `ReferenceGrant`는 cross-namespace backend reference 같은 경우에 필요하지, 현재 generic Harbor 예제처럼 same-namespace backendRef에는 필수가 아니다.

## 8. NodeVault GRPCRoute의 의미

[k8s/nodevault/01-grpcroute.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/k8s/nodevault/01-grpcroute.yaml:1)은 다음 의미로만 본다.

- future-facing north-south gRPC ingress 후보
- NodeVault가 in-cluster로 들어올 때 사용할 수 있는 예제

현재 의미가 아닌 것:

- 내부 east-west service mesh
- 운영 기준선에 이미 포함된 live 리소스

현재 live에는 `GRPCRoute`가 없고, `nodevault-controlplane` Service도 없다.

## 9. 실험 항목 분리

운영 기준선에 아직 넣지 않는 실험 항목은 [profiles/remote-seoy/cilium/experimental](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/experimental)에 분리한다.

구성:

- GAMMA: [profiles/remote-seoy/cilium/experimental/gamma/README.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/experimental/gamma/README.ko.md:1)
- mutual auth: [profiles/remote-seoy/cilium/experimental/mutual-auth/README.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/experimental/mutual-auth/README.ko.md:1)

원칙:

- Gateway-only baseline을 먼저 닫는다.
- 실험은 desired baseline과 분리한다.
- 운영 중인 기존 클러스터에는 섞지 않는다.

## 10. cloud profile 방향

[profiles/remote-seoy/cilium/cloud-profiles/README.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/cloud-profiles/README.ko.md:1)에 cloud 확장 방향을 정리했다.

핵심 메시지:

- app core는 Cilium IPAM에 의존하면 안 된다.
- app은 Pod IP 직접 접근이 아니라 `Service DNS / Service / Gateway API` 기준으로 통신해야 한다.
- infra-lab / on-prem에서는 `LB IPAM + L2 announcement`가 적합할 수 있다.
- cloud에서는 cloud LoadBalancer, cloud CNI/IPAM, managed Gateway Controller를 우선 검토한다.
- `cluster-pool + vxlan`은 lab/on-prem-like profile에는 적합하지만 cloud 운영의 기본 전제는 아니다.

## 11. 검증과 스냅샷 수집

새 profile에는 read-only 스크립트만 넣었다.

- 수집: [profiles/remote-seoy/cilium/verify/collect-live-state.sh](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/verify/collect-live-state.sh:1)
- Cilium 검증: [profiles/remote-seoy/cilium/verify/verify-cilium.sh](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/verify/verify-cilium.sh:1)
- Gateway 검증: [profiles/remote-seoy/cilium/verify/verify-gateway.sh](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/verify/verify-gateway.sh:1)

이 스크립트들은 다음만 수행한다.

- `kubectl get ...`
- `helm get values ...`
- `cilium status`
- 로컬 레포 아래 snapshot 파일 갱신

다음은 수행하지 않는다.

- `helm upgrade`
- `helm uninstall`
- `kubectl apply`
- `kubectl delete`

## 12. 운영 메모

현재 `remote-seoy`를 다룰 때는 다음 순서로 생각하는 것이 안전하다.

1. live 증거는 `live-snapshot/`에서 확인한다.
2. 재현 기준은 `desired/`에서 확인한다.
3. generic addon 파일은 공용 예제로만 본다.
4. IPAM 변경이나 service mesh 실험은 baseline 작업과 분리한다.

이 문서의 목적은 “기능을 더 넣는 것”이 아니라, 이미 운영 중인 Cilium 상태를 레포 안에서 안전하게 설명 가능하도록 만드는 것이다.
