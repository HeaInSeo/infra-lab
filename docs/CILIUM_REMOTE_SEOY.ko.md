# remote-seoy Cilium 상세 설명

## 1. 이 문서의 목적

이 문서는 `100.123.80.48`에서 운영 중인 `remote-seoy` VM lab의 Cilium 구성을 자세히 설명한다.

목적은 두 가지다.

1. 현재 운영 중인 상태를 이해하기 쉽게 설명
2. generic addon 예제와 live 운영 상태의 차이를 분명히 하기

이 문서는 “새 기능 제안서”가 아니다. 특히 service mesh 확장 문서가 아니다.

## 2. 현재 Cilium이 하는 일

현재 `remote-seoy`에서 Cilium은 다음 역할을 한다.

- Pod networking
- kube-proxy replacement
- LoadBalancer IPAM
- L2 announcement
- Gateway API ingress

즉, 현재 역할은 “cluster networking + north-south ingress”다.

아직 하지 않는 일:

- east-west service mesh
- CiliumNetworkPolicy 기반 세밀한 정책 운영
- mutual auth / SPIRE / mTLS
- ClusterMesh

## 3. 현재 live 기준선

현재 live snapshot 기준:

- Cilium version: `1.19.1`
- Kubernetes: `v1.32.13`
- IPAM: `cluster-pool`
- Routing: `tunnel`
- Tunnel protocol: `vxlan`
- kubeProxyReplacement: `true`
- Gateway API: `enabled`
- Hubble: `enabled`
- Gateway: `harbor/lab-gateway`
- LoadBalancer IP pool: `10.113.24.100/28`
- L2 interface: `mpqemubr0`

관련 snapshot:

- [profiles/remote-seoy/cilium/live-snapshot/cilium-helm-values.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/cilium-helm-values.yaml:1)
- [profiles/remote-seoy/cilium/live-snapshot/cilium-status.txt](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/cilium-status.txt:1)
- [profiles/remote-seoy/cilium/live-snapshot/gateway.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/gateway.yaml:1)
- [profiles/remote-seoy/cilium/live-snapshot/httproutes.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/httproutes.yaml:1)
- [profiles/remote-seoy/cilium/live-snapshot/lb-pool.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/lb-pool.yaml:1)
- [profiles/remote-seoy/cilium/live-snapshot/l2-announcement-policy.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/l2-announcement-policy.yaml:1)

## 4. 왜 이 구성이 lab에 맞는가

`remote-seoy`는 퍼블릭 클라우드가 아니라, VM 기반 on-prem-like lab이다.

이 환경의 특징:

- 외부 클라이언트가 Gateway / LoadBalancer IP로 직접 접근한다.
- host가 브리지 인터페이스를 통해 VM 네트워크에 들어간다.
- cloud provider가 관리해 주는 LB나 managed ingress가 없다.

이 조건에서는 다음 조합이 실용적이다.

- `cluster-pool`
- `vxlan`
- `LB IPAM`
- `L2 announcement`
- `Gateway API`

즉, Cilium이 단순 CNI를 넘어 “lab용 네트워크 플랫폼 역할”을 같이 수행한다.

## 5. 트래픽이 어떻게 흐르는가

대략적인 흐름은 다음과 같다.

1. 외부 클라이언트가 `10.113.24.96` 또는 `*.10.113.24.96.nip.io`로 접근한다.
2. host는 브리지 인터페이스를 통해 해당 IP 트래픽을 VM 쪽으로 보낸다.
3. Cilium L2 announcement가 그 IP를 광고한다.
4. Gateway Service가 그 IP를 받는다.
5. Gateway listener가 HTTP/HTTPS 요청을 수신한다.
6. hostname에 따라 각 `HTTPRoute`로 분기된다.
7. 최종적으로 namespace 안의 backend `Service`로 전달된다.

여기서 중요한 점:

- 현재는 `north-south ingress` 흐름이다.
- Service 간 내부 east-west mesh routing이 아니다.

## 6. Gateway 구조

현재 live Gateway는 `harbor/lab-gateway`다.

주요 특징:

- namespace: `harbor`
- `gatewayClassName: cilium`
- `HTTP:80`
- `HTTPS:443`
- `allowedRoutes.namespaces.from: All`

왜 `from: All`이 필요한가:

- 현재 `harbor` 외 namespace의 Route도 이 Gateway에 붙기 때문이다.
- 예: `dev-space-observability`, `shift-left-observability`

즉, cross-namespace parentRef 허용은 Gateway listener 정책 문제다.

## 7. 현재 Route 분류

현재 live snapshot의 HTTPRoute는 세 가지다.

- `harbor-route`
- `dev-space-observability`
- `shift-left-observability`

이 셋을 같은 성격으로 보면 안 된다.

### baseline으로 보는 Route

- `harbor-route`
- `dev-space-observability`

이 둘은 현재 infra baseline 설명에서 반복적으로 기대하는 Route다.

### workload-attached live Route

- `shift-left-observability`

이 Route는 live에는 존재하지만, 공용 infra baseline에 자동 편입하지 않는다.

이유:

- 특정 시점의 workload 부착 상태일 수 있다.
- baseline에 넣는 순간 “이 lab은 항상 그 workload를 제공한다”는 신호가 된다.
- 현재 작업 목표는 workload 확장이 아니라 Cilium 운영 기준선 정리다.

그래서 이 Route는:

- `live-snapshot/`에는 기록
- `desired/`에는 미포함

으로 유지한다.

## 8. GRPCRoute를 왜 baseline에 넣지 않는가

현재 live에는 `GRPCRoute`가 없다.

기존 예제 파일:

- [k8s/nodevault/01-grpcroute.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/k8s/nodevault/01-grpcroute.yaml:1)

이 파일의 성격:

- future-facing
- north-south gRPC ingress 후보
- NodeVault in-cluster 전환 시 사용할 수 있는 예제

이것이 의미하지 않는 것:

- service mesh 완성
- 현재 운영 baseline

즉, `GRPCRoute`는 존재하더라도 “내부 메시”와 동의어가 아니다.

## 9. generic addon과 왜 다른가

generic addon 예제는 다음을 전제로 남아 있다.

- 공용 예제
- 환경 독립적 시작점
- kubeadm `pod-network-cidr`에 가까운 설명

대표 파일:

- [addons/values/cilium/values.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/addons/values/cilium/values.yaml:1)

반면 `remote-seoy` live는 실제로 다음 기준을 쓴다.

- `cluster-pool`
- `vxlan`
- `lab-gateway`
- `10.113.24.100/28`
- `mpqemubr0`

이 차이를 공용 파일 하나로 덮으면 위험하다.

특히 위험한 것:

- live를 generic addon 쪽으로 되돌리는 것
- generic addon 값을 live에 덮어써서 IPAM 변경을 유발하는 것

그래서 profile 분리가 필요하다.

## 10. IPAM을 왜 건드리면 안 되는가

현재 live의 IPAM은 `cluster-pool`이다.

이번 기준선 정리의 핵심 안전 원칙:

- 이것을 “문서화”는 해도
- 기존 운영 클러스터에서 “변경”하려고 하지는 않는다

왜냐하면:

- IPAM mode 변경은 네트워크 핵심 경로 변경이다.
- baseline 문서화 작업이 아니라 재구축/마이그레이션 작업이 된다.
- 현재 목표를 벗어난다.

따라서 현재는 다음만 한다.

- live 사실 기록
- desired baseline에 신규 재현 기준으로 명시
- 기존 generic addon 경로는 그대로 유지

## 11. Hubble과 Envoy의 의미

현재 live에는 Hubble이 켜져 있고, Gateway controller가 만든 `CiliumEnvoyConfig`도 존재한다.

이것이 뜻하는 것:

- Gateway API ingress가 Cilium/Envoy datapath 위에서 실제로 동작한다.
- ingress 계층의 관찰성과 proxy 계층이 준비돼 있다.

하지만 이것만으로 다음을 의미하지는 않는다.

- service mesh 완성
- east-west L7 policy 운영
- mTLS mesh

즉, “Envoy가 있다”와 “service mesh를 하고 있다”는 다른 말이다.

## 12. read-only 운영 절차

현재 이 profile에는 read-only 절차만 연결했다.

표준 명령:

- `HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh profile-cilium-collect`
- `HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh profile-cilium-verify`
- `HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh profile-gateway-verify`

이 명령들이 하는 일:

- 현재 live 증거를 수집
- cilium status / helm values / gateway / route / lb 상태 확인
- snapshot 파일 갱신

이 명령들이 하지 않는 일:

- `helm upgrade`
- `kubectl apply`
- `kubectl delete`
- IPAM 변경

## 13. 앞으로의 확장 방향

미래에 검토할 수 있는 축은 있다.

- GAMMA
- mutual auth / SPIRE
- ClusterMesh
- cloud별 CNI/Gateway profile

하지만 이들은 모두 baseline 밖에 둔다.

현재 순서가 중요하다.

1. live drift를 줄인다.
2. Gateway-only baseline을 닫는다.
3. 그 다음 실험 축을 profile / experimental 문서로 확장한다.

## 14. 이 문서를 읽고 나서 봐야 할 파일

운영 증거:

- [profiles/remote-seoy/cilium/live-snapshot](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot)

재현 기준:

- [profiles/remote-seoy/cilium/desired](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/desired)

운영 기준 요약:

- [docs/CILIUM.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/CILIUM.ko.md:1)

프로젝트 전체 설명:

- [docs/PROJECT_GUIDE.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/PROJECT_GUIDE.ko.md:1)
