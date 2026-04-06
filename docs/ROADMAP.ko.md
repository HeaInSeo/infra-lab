# ROADMAP

## 단기 계획

- 첫 번째 베이스라인을 Rocky Linux 8 호스트에서 검증
- 기본 실용 형태를 `1 master + 2 workers`로 유지
- 빠른 부트스트랩 기본값으로 Flannel 유지
- 애드온 검증 출력 개선

## 향후 프로필

- `profiles/cilium`
- `profiles/storage-lab`
- `profiles/node-agent-lab`
- `profiles/operator-dev`

## 향후 애드온 후보

- ingress 또는 gateway 지원
- local PV 헬퍼
- CSI 실험
- 트러블슈팅을 위한 observability 베이스라인

## 가드레일

- 베이스라인을 프로젝트 워크로드 저장소로 바꾸지 않을 것
- 구체적인 사용 사례가 생기기 전까지 프로필을 추가하지 않을 것
- 기본 경로는 즉시 쓸 수 있을 만큼 작고 단순하게 유지할 것
