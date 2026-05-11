# GAMMA

`GAMMA`는 Gateway API를 service mesh 문맥으로 확장하는 실험 축이다.

이 저장소에서의 위치:

- 현재 운영 기준선에는 포함하지 않는다.
- 필요 시 Service `parentRef` 기반 east-west `HTTPRoute` 실험으로만 다룬다.
- `GRPCRoute` north-south ingress와 혼동하지 않는다.

현재 판단:

- `remote-seoy` live에는 GAMMA 리소스가 없다.
- Gateway-only baseline 정리 후 별도 PoC로 검토한다.
