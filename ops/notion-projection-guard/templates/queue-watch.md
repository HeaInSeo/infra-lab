# 큐 감시 — 읽기 전용 공통 모듈 템플릿

기존 읽기 전용 감시 프롬프트에 결합한다. 제품별 원본·권한·지연 기준은 현재
비공개 설정을 유지한다. `{{AUTHORITY_AND_SOURCE_MAP}}`, `{{CANONICAL_STATE}}`,
`{{APPROVED_RELEASE}}`는 실제 읽기로 확정한다.

- 서버 직결 경로로 현재 상태를 읽는다. runtime·systemd를 읽지 못한 회차는 UNKNOWN을
  보고하며 이전 정상값을 현재값으로 재사용하지 않는다.
- GitHub의 현재 head·리뷰·CI·병합과 원본 worker의 자기 증빙을 대조한다. 새 결과와
  중앙 관측의 작성 시점을 함께 확인한다. 오래된 투영만으로 worker 장애를 단정하지 않는다.
- canonical active batch의 status·next·planHash·ticket·resolution·receipt를 함께 읽는다.
  `RECONCILED_PARTIAL`·`ABORTED_UNWRITTEN` 등 종료 상태에 보존된 과거 error는
  활성 HALTED와 구분한다. 부분 종료를 전체 동기화 성공으로 보고하지 않는다.
- runtime 파일·승인 manifest·설치 receipt는 배포 명세와 대조한다. 브랜치 최신 head와
  설치된 승인 커밋은 다를 수 있다. 실제 파일 일치는 예약 실행 채택의 증거와 구분한다.
- 실제 HALTED·불확실한 쓰기는 동일 batch와 보존 증거를 담당 중앙 owner에게 보고한다.
  재요청·raw Notion 수정·새 batch·worker 실행·서비스 재시작·merge를 하지 않는다.
- 필요한 보고에는 관측 시각, 정확한 head/batch, 실제 blocker, 담당자와 다음 정상
  단계를 쓴다. 새 사실과 상태 전환을 중심으로 보고하며 전체 제품 완료를 과장하지 않는다.

공통 템플릿에는 실제 session·인증값·Notion 전체 원문을 넣지 않는다. 기존 자동화의
스케줄을 유지하며 실제 설정 변경은 별도로 기록한다.
