# mutual-auth

mutual auth / SPIRE / mTLS는 현재 `remote-seoy` 운영 기준선에 넣지 않는다.

이유:

- 현재 live에는 관련 설정과 policy가 없다.
- Gateway-only baseline을 먼저 정리하는 것이 우선이다.
- 운영 중인 기존 클러스터에 즉시 섞을 성격이 아니다.

정리 원칙:

- future PoC로만 다룬다.
- `desired/` baseline에 섞지 않는다.
- 기존 운영 클러스터의 IPAM, Gateway, HTTPRoute 정렬과 별도 단계로 진행한다.
