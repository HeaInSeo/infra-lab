# REFERENCE_FROM_MAC_K8S

이 문서는 `mac-k8s-multipass-terraform`이 현재 베이스라인에 어떤 영향을 줬는지 기록합니다.

## 유지한 것

- 단일 엔트리포인트 스크립트 패턴
- `scripts/host`, `scripts/multipass`, `scripts/cluster` 분리
- `null_resource`를 이용한 OpenTofu 기반 로컬 오케스트레이션
- Multipass VM 실행 및 삭제 래퍼
- kubeadm init 및 join 흐름
- 로컬 kubeconfig 내보내기

## 변경한 것

- 저장소 정체성을 테스트 중심 클러스터 저장소에서 공용 K8s 랩 베이스라인으로 변경
- 기본 구성을 실용적인 `1 master + 2 workers` 형태로 조정
- 광범위한 서비스/플랫폼 번들 대신 `base`와 `optional` 인프라 카탈로그로 애드온 구조 변경
- 범위, 소유권, 비목표를 중심으로 문서 전면 재작성
- 호스트 설정 및 애드온 사용을 1급 명령으로 다루도록 명령 체계 확장

## 제거한 것

- MySQL, Redis 설치 헬퍼 같은 서비스 전용 스크립트
- 서비스 전용 cloud-init 조각
- 기본 애드온 스토리였던 monitoring/logging/tracing/service-mesh 번들
- 랩 플랫폼 자체가 아니라 과거 테스트/서비스 흐름에 묶인 문서

## 이유

기존 프로젝트는 Rocky 8 + Multipass + OpenTofu + kubeadm 조합의 동작 자체는 이미 증명했습니다. 새 저장소는 그 메커니즘은 유지하되, 여러 향후 K8s 프로젝트가 공통으로 사용할 수 있는 재사용 가능한 랩 기반으로 책임 범위를 다시 좁힙니다.
