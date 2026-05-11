# cloud-profiles

이 디렉터리는 향후 퍼블릭 클라우드별 profile 확장을 위한 자리다.

핵심 메시지:

- app core는 Cilium IPAM에 의존하지 않는다.
- app은 Pod IP 직접 접근이 아니라 `Kubernetes Service DNS / Service / Gateway API` 기준으로 통신한다.
- infra-lab / on-prem 계열 환경에서는 `LB IPAM + L2 announcement`를 사용할 수 있다.
- 퍼블릭 클라우드에서는 cloud LoadBalancer, cloud CNI/IPAM, managed Gateway Controller를 우선 검토한다.
- EKS / AKS / GKE profile은 나중에 별도 디렉터리로 확장한다.
- `cluster-pool + vxlan`은 lab / on-prem-like profile에는 적합하지만 cloud 운영의 기본 전제는 아니다.
