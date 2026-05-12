# live-snapshot

이 디렉터리는 `100.123.80.48`에서 동작 중인 `remote-seoy` 클러스터의 Cilium/Gateway 상태를 read-only로 수집한 증거를 보관한다.

원칙:

- 이 디렉터리의 파일은 현재 운영 상태를 설명하는 스냅샷이다.
- 선언형 desired baseline과 섞지 않는다.
- 운영 리소스를 변경하는 명령은 포함하지 않는다.

현재 스냅샷 기준 요약:

- Cilium: `1.19.1`
- Routing: `tunnel + vxlan`
- kube-proxy replacement: `true`
- Gateway API: `enabled`
- Hubble: `enabled`
- LB IP Pool: `10.113.24.100/28`
- L2 interface: `mpqemubr0`
- live Gateway: `harbor/lab-gateway`
- live HTTPRoute: `harbor-route`, `dev-space-observability`, `shift-left-observability`
- live GRPCRoute: 없음
- live CiliumNetworkPolicy / CiliumClusterwideNetworkPolicy: 없음
- live CiliumEnvoyConfig: gateway controller가 생성한 `cilium-gateway-lab-gateway`

이 스냅샷은 `verify/collect-live-state.sh`로 다시 수집할 수 있다.

주요 증거 파일:

- `nodes.txt`
- `cilium-helm-values.yaml`
- `cilium-status.txt`
- `gateway.yaml`
- `httproutes.yaml`
- `grpcroutes.yaml`
- `lb-pool.yaml`
- `l2-announcement-policy.yaml`
- `network-policies.txt`
- `ciliumenvoyconfigs.txt`
