# experimental

이 디렉터리는 `remote-seoy` 운영 기준선에 아직 넣지 않는 Cilium 관련 실험 메모를 분리한다.

이번 기준선에서 제외하는 항목:

- GAMMA 기반 east-west HTTPRoute
- mutual auth / SPIRE / mTLS
- ClusterMesh

원칙:

- 운영 기준선은 먼저 `Gateway-only baseline`으로 닫는다.
- 실험 결과가 축적되기 전까지는 `desired/`나 generic addon 경로에 섞지 않는다.
