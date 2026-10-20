# 중앙 감독 — 공통 갱신 모듈 템플릿

현재 운영 프롬프트에 결합할 공통 모듈이다. 제품별 권한·스프린트·검토 게이트와
사용자 직접 관리 작업의 제약은 현재 비공개 설정에서 유지한다.

## 실제 읽기로 채울 값

- `{{SOURCE_COMMIT}}`: 검토된 전체 40자리 커밋과 배포 명세.
- `{{SERVER_DEVICE_ID}}`: 실제 온라인 서버의 확인된 장치 ID.
- `{{APPROVED_BROKER_COMMAND}}`: runtime 해시를 검증하고 동일 canonical state를
  여는 기존 고정 명령. 노트북을 경유하지 않는 서버 직결 경로를 사용한다.
- `{{CANONICAL_STATE}}`, `{{POLICY_HASH}}`: 현재 보존된 state 위치와 승인 정책 해시.
- `{{AUTHORITY_AND_SOURCE_MAP}}`: 현재 원본·paired·index·Router·운영 문서와 권한 연결.

## 실행 규칙

1. 현재 권한과 GitHub head·CI·worker 원본 증거를 새로 읽는다. 다른 worker의
   CLAIM·lease·RELEASE·자기 증빙을 대신 작성하지 않는다.
2. canonical active/status/resolution/receipt를 함께 확인한다. 과거 종료된 사건의
   남아 있는 error를 현재 HALTED로 판정하지 않는다. 실제 pending/HALTED·미확정
   쓰기에는 이후 본문 쓰기와 외부 후속을 멈추고 동일 원본 증거를 보존한다.
3. 같은 고정 커밋의 `runner.js`와 `complete_projection.js`를 해시로 확인하고
   기존 authenticated connector와 승인 broker command를 사용한다. 토큰을 추출하지 않는다.
4. NodeKit/Sori 결합 관측은 complete entry의 고정 manifest(body 9/native 4)와
   유일한 observation identity를 사용한다. native property는 현재 허용된 중앙
   속성만 기존 보존 절차로 갱신하며 worker 소유 필드를 유지한다.
5. 본문은 fresh full read의 정확한 중앙 heading 아래 유일한 한 줄 literal만
   사용한다. old/new 각각 1024자 이하이며 필드·코드 경계를 자르지 않는다.
   runner의 전체 대상 preflight와 매 페이지 prepare/authorize/전체 readback을 따른다.
6. 사전검사 거부는 실제 미기록·0 writes를 확인한 뒤 계획을 보정한다. 미확정 batch를
   새 ID/state나 raw write로 우회하지 않는다. 복구 helper를 자동 호출하지 않는다.
7. 전체 body·property 보존을 검증한 complete entry의 `COMPLETE`만 현재 동기화
   완료로 보고한다. 파일 해시 MATCH·부분 batch COMPLETED·프롬프트 수정은 별도 사실이다.
8. 현재 head의 결과를 수신한 뒤 같은 원본 task/PR로 보정을 연결하고 기존 독립
   검토·Codex·thread 처분·병합 게이트를 따른다. 스테이지 완료와 제품 완료를 구분한다.

실제 적용 후에는 전체 private prompt를 readback해 기존 규칙 보존을 확인한다.
예약 실행 채택은 그 회차의 실제 guarded receipt로 확인한다.
