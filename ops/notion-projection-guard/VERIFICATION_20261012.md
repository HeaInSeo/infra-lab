# U0 갱신 경로 검증 기록 — 2026-10-12

이번 변경은 Notion 본문 갱신 방식의 코드 수정이다. 사고 대상11개 페이지의 복원은 수행하지 않았으며 중앙 감독도 재개하지 않았다.

## 검증 결과

| 검증 | 관측 결과 |
| --- | --- |
| 회귀 검증 | Node22/24에서32개 PASS; 실제 heain Node22에서 초기30개 PASS |
| 실제 authenticated Notion 순차 갱신 | 비공개2개 페이지 COMPLETED; 전체 보호 본문과 속성 일치; verified snapshot2개 |
| 손상 재조회 응답 주입 | 실제 첫 페이지 update 후 읽은 응답에서4227자 제거; READBACK_MISMATCH/HALTED; Notion write1회, 둘째 페이지 write0회 |
| 주입 후 실제 페이지 재확인 | 두 페이지 전체 본문은 각각 의도한 최소 literal 교체 결과와 동일; 실제 역사 본문은 손상 없음 |
| 운영 페이지 차단 | productionEnabled=false에서 PRODUCTION_HOLD; Notion fetch0/write0 |
| 중앙 설정 readback | NPG-20261012-v1 pinned runner 경로 저장 확인; is_enabled=false; 기존 schedule 동일 |
| Queue Watch | enabled=true, 기존 읽기 전용 prompt 동일 |
| runtime 무결성 | broker/policy/runner 승인 SHA256 일치; code0555, policy/runner/manifest0444; 활성 broker lock0 |
| 코드 commit CI | 57868c0e8b3805187e2e3fe21db9d5c91e9d176f: check, Notion projection guard, workflow-lint 모두 success |

손상 응답은 시험 어댑터에서 주입했다. Notion 서버 자체의 실제 손상으로 오인하지 않는다. 실제 수정은 원래 인증된 Notion connector를 통해 수행했고, 가짜 ACK·수동 상태 해제·손상 운영 페이지 수정은 없었다.

## 실제 연결에서 확인해 보정한 부분

Desktop Commander의 incremental output cursor가 응답을 놓치는 경우가 있었다. 요청을 재전송하지 않고 retained tail에서 같은 request ID만 재조회하도록 수정했다. Notion update의 성공 결과가 structuredContent 없이 content의 JSON page_id로 오는 실제 형식도 수용했다. 인식한 성공 뒤에는 항상 전체 본문을 fresh readback하며 성공 응답 자체로 다음 페이지를 열지 않는다.

두 초기 실패 시험의 상태·원문은 삭제하지 않고 별도 canary archive에 보존했다. 첫 응답 불명 시험은 Notion write0이었다. 두 번째 ACK 형식 시험은 첫 페이지 write1 후 HALTED였으며 둘째 페이지는 그대로였다. 운영 state와 canary state는 분리했고 운영 state 우회는 허용하지 않는다.

## 코드·설정 결속

- PR: https://github.com/HeaInSeo/infra-lab/pull/41
- branch: fix/notion-projection-guard-20261012
- pinned code commit: 57868c0e8b3805187e2e3fe21db9d5c91e9d176f
- runner content blob SHA: 04e1ef441b896a96a232ec9c7e271353ae715f65
- broker SHA256: e17149286da1b039271ca447c081a3db754b3209f364ab2f42739f606074eef9
- runner SHA256: b9c21288db22d5abb7b5aeae94131056c567b247a8588ff2da403ce1078d4ed5
- policy SHA256: 33c4061911eea1e0fa7b639c427f2b8edda023604b4563299688578f2616d193
- 중앙 automation: 6a742f6443e8819184310cd28e6162cd; 설정 갱신 2026-10-12T06:33:59.127342Z
- 정상 runtime: /home/heain/.local/share/notion-projection-guard
- 정상 production state: /home/heain/.local/state/notion-projection-guard

## 남은 종료 조건

11개 페이지의 손상 역사 구간을 복원하고, 사고 이후 최신 실제 worker proof를 보존·대조해야 한다. 이후 production policy 활성화와 중지된 실제 scheduled tick의 검증 경로 채택을 확인해야 중앙 운영 재개가 가능하다. 이번 수동 Code Mode 검증을 scheduled 채택으로 보고하지 않는다.

현재 hosted automation API는 per-task raw Notion tool 차단/라우팅 설정을 제공하지 않는다. 검증 경로 내부의 코드 강제와 실제 connector 호출 경로 연결은 완료했지만 raw capability 자체의 권한 격리는 완료했다고 주장하지 않는다. Notion remote CAS와 보이지 않는 native block identity 동일성도 보장하지 않는다.

이 사고의 실패한 selection 계산 실행 코드는 현재 확보하지 못했다. 따라서 특정 offset/off-by-one을 확정하지 않는다. 확인한 구조적 원인은 모델이 계산한 치환 범위와 연쇄 진행을 실행 가능한 보호 검사 없이 raw connector에 맡긴 것이다.
