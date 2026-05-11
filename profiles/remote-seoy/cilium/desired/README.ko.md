# desired

이 디렉터리는 `remote-seoy` 환경에서 앞으로 재현하고 싶은 `Gateway-only` 기준선을 담는다.

중요:

- 이 디렉터리는 기존 운영 클러스터에 대한 즉시 업그레이드 지침이 아니다.
- 신규 재현 또는 기준선 문서화 목적이다.
- 특히 `ipam.mode`는 기존 운영 클러스터에서 변경 대상이 아니다.
- 현재 운영 중인 cluster-pool 기준선을 기록하되, 기존 generic addon 값을 덮어쓰는 경로로 사용하지 않는다.

현재 desired baseline 범위:

- Cilium core: `cluster-pool + tunnel(vxlan) + kubeProxyReplacement`
- Gateway API ingress
- LB IPAM + L2 announcement
- Harbor / dev-space-observability HTTPRoute

이번 baseline에 포함하지 않는 것:

- east-west service mesh
- GAMMA
- mutual auth / SPIRE / mTLS
- GRPCRoute 기반 운영 경로
