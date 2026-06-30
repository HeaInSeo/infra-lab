# MCP 사용성/운영 안정화 스프린트

이 문서는 infra-lab MCP를 7.5점 수준의 MVP에서 9점대 운영 가능성으로 올리기 위한 보강 스프린트 일정이다.

## 목표

```text
현재: 7.5/10
목표: 9.0/10 이상
```

핵심 개선 과제:

```text
1. operation approve 명시 승인 UX
2. operation cancel / locks / stale-only unlock
3. 실제 lab 반복 검증
4. agent 추천 workflow 문서
```

중요 전제:

```text
VM 생성/삭제/재빌드 live 검증은 로컬 개발 장비에서 수행하지 않는다.
env_up/down/clean/rebuild commit 계열은 원격 lab 장비에서만 수행한다.
로컬에서는 prepare/status/logs/approve/cancel/lock 같은 비파괴 흐름만 검증한다.
```

## 일정

| 기간 | 목표 | 산출물 |
| --- | --- | --- |
| Week 1 | 승인/취소/lock 운영 UX 보강 | `operation_approve`, `operation_cancel`, `operation_locks`, `operation_unlock_stale` |
| Week 1 | 단위/MCP 테스트 | operation status transition, stale lock test, MCP registration test |
| Week 2 | live lab 반복 검증 | addon install, env up, down/clean/rebuild, 실패/복구 기록 |
| Week 2 | agent workflow 정리 | `docs/MCP_AGENT_WORKFLOW.ko.md` |
| Week 3 optional | edge case 안정화 | timeout, lock, audit, failed operation 복구 UX 보강 |

## 완료 기준

### Week 1

```text
- prepare 후 operation_approve 가능
- APPROVED operation은 approvalToken 없이 commit 가능
- PREPARED/APPROVED operation cancel 가능
- lock 목록 read-only 조회 가능
- expiresAt 이후 stale lock만 unlock 가능
- active lock은 unlock 거부
- make test-mcp 통과
```

### Week 2

```text
- 원격 lab 장비에서 수행
- addon_install prepare/approve/commit 3회 이상 반복
- env_up prepare/approve/commit 1회 이상 성공
- env_down prepare/approve/commit 1회 이상 성공
- env_clean 또는 rebuild 1회 이상 검증
- 실패한 operation의 status/logs로 원인 추적 가능
- 검증 결과를 troubleshooting 또는 validation note로 기록
```

원격 검증 기준:

```text
- HOST_PROFILE=hosts/remote-lab.env 또는 동등한 원격 lab 설정 사용
- 원격 checkout에서 make build, make build-mcp 수행
- MCP server도 원격 checkout의 bin/infra-lab-mcp를 기준으로 실행
- destructive commit 전 operation_status target/risk 재확인
```

## 진행률 기준

```text
1/4 완료: operation approve
2/4 완료: cancel/locks/stale unlock
3/4 완료: live lab 반복 검증
4/4 완료: agent workflow 문서
```

## 점수 전망

```text
Week 1 완료: 8.2~8.5/10
Week 2 완료: 8.8~9.1/10
Week 3 안정화 완료: 9.2/10 근처
```
