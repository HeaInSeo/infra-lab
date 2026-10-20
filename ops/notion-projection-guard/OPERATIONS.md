# 중앙 동기화 코드와 서버 운영 자료 관리

## 정본과 보존 범위

`infra-lab/ops/notion-projection-guard`에서 코드, 회귀 시험, 설치·복구 절차,
배포 버전 명세와 재사용 가능한 운영 프롬프트를 관리한다. 기존 ACTIVE PR #41을
사용하며 다른 제품 PR이나 PARKED 작업의 브랜치를 함께 수정하지 않는다.

| GitHub에서 관리 | 보호된 운영 저장소에서 관리 |
| --- | --- |
| broker·runner·complete entry와 시험 | 실제 journal·pending ticket·원본 receipt |
| 고정 커밋, 파일 해시, 민감정보를 제거한 장애 설명 | Notion 전체 원문·속성 스냅샷, worker 증거·세션 자료 |
| 설치·복구 절차와 프롬프트 템플릿 | 실제 장치·경로·권한 설정, 전체 자동화 프롬프트와 인증값 |

이 저장소는 공개다. 원본 운영 폴더나 자동화 프롬프트 전체를 복사해 커밋하지 않는다.
`.gitignore`는 실수 방지를 돕는다. 업로드 전에는 실제 변경 파일도 확인한다.
운영 증거는 권한을 제한한 별도 백업에 보존하고, 복구 시 같은 canonical journal과
그에 연결된 snapshot·receipt·audit를 함께 검증한다. 새 빈 state로 복구하지 않는다.

## 현재 설치 버전

버전 명세: [deployment/runtime-01b8a789.json](deployment/runtime-01b8a789.json).
서버 설치 receipt는 2026-10-19 23:02:49 UTC의
`01b8a7897bc8709450f536744577313c805c66ca`를 가리키며,
2026-10-20 00:07:09 UTC에 broker·runner·policy·approved manifest를 읽어 확인했다.
검토된 이 커밋의 broker·runner 해시와 서버 파일이 일치했다.

명세에 기록한 policy 해시는 기존 비공개 운영 정책의 식별값이다. GitHub의
`policy.json`은 `productionEnabled:false`인 기본값이므로 운영 파일을 덮어쓰는 데
사용하지 않는다. 운영 정책의 원문, 권한 범위와 변경 이력은 보호된 저장소에 보존한다.
이번 문서·검증 도구 추가로 설치 코드의 승인 커밋이나 운영 정책이 바뀌지 않는다.

## 읽기 전용 버전 대조

신뢰하는 고정 GitHub 커밋에서 검증 도구와 배포 명세를 읽는다. 명세와 실제 파일을
동시에 임의 수정하면 신뢰의 기준이 없어지므로 mutable branch 이름을 승인값으로 삼지 않는다.
대조에는 현재 설치 receipt의 절대 경로를 사용한다.

```sh
node ops/notion-projection-guard/verify_runtime.cjs \
  --release ops/notion-projection-guard/deployment/runtime-01b8a789.json \
  --runtime /absolute/path/to/installed/runtime \
  --receipt /absolute/path/to/private/installation.json \
  --source /absolute/path/to/checked-out/ops/notion-projection-guard
```

`--source`는 선택 항목이다. 사용하면 broker·runner와 hosted complete entry의
소스 해시도 대조한다. 출력의 `sourceFilesChecked`로 수행 여부를 구분한다.
도구는 파일을 읽고 해시를 계산하며, broker를 import/실행하거나 journal·lock을
열거나 변경하지 않는다. 기존 작업이 lock을 보유해도 버전 대조는 가능하다.
일치 시 `MATCH`와 exit 0, 불일치·누락·중복 manifest·오래된 receipt에는
`MISMATCH`와 exit 1을 반환한다. 원문·토큰·receipt 전체는 출력하지 않는다.

`MATCH`는 관측한 파일과 설치 기록의 일치다. 코드 리뷰 승인, 설치 허가, 현재 작업의
정상 상태나 예약 자동화의 실제 채택을 판정하지 않는다. 마지막 재조회로 관측 도중의
파일 변경을 검사하지만 이후 변경이나 과거 receipt의 진위까지 보장하지 않는다.

## 다음 버전의 설치 절차

1. GitHub의 기존 ACTIVE PR에서 변경을 검토하고 해당 고정 커밋의 시험·CI·정상
   검토 게이트를 확인한다. main 반영과 실제 서버 설치는 각각의 기록을 남긴다.
2. 후보 코드와 hosted entry를 같은 고정 커밋에서 준비하고 실제 bytes의 해시를
   확인한다. 설치 명세에는 커밋, broker·runner, 별도 승인된 policy의 해시를 기록한다.
3. 중앙 쓰기를 잠시 멈춘 상태에서 현재 runtime·manifest·policy·journal을 읽는다.
   canonical active batch가 없음을 확인하고 기존 broker lock을 취득한다.
   미확정·HALTED·busy lock은 해당 상태를 보존하고 기존 복구 절차로 처리한다.
4. 기존 runtime·manifest·policy와 journal의 정확한 private 사본을 보존한다.
   코드·manifest는 각각 같은 파일시스템의 임시 파일에 쓰고 fsync·rename·directory
   fsync한다. broker와 runner를 함께 교체하고 manifest를 마지막에 게시한다.
   여러 파일 전체의 교체는 원자적이지 않으므로 중간 장애 시 실행을 멈춘 채 보존
   사본과 설치 기록을 대조한다. 혼합 버전을 사용하거나 임의 재시작하지 않는다.
5. 코드 0555, policy·manifest 0444, state 0700와 증거 파일 0600을 확인한다.
   journal bytes와 policy가 설치 중 보존됐는지 대조하고 설치 receipt를 fsync·readback한다.
   receipt에는 `sourceCommit`, `at`, `before`, `after`, `journalHash`, 정책·worker
   서비스 변경 여부를 기록한다. 이번 도구는 이 receipt 형식과 호환된다.
6. 새 버전 명세로 읽기 전용 대조와 관련 거부 시험을 수행한다. 실제 hosted 자동화의
   고정 runner·complete entry·broker command를 대조하고 보호된 prompt readback을
   남긴다. 기존 스케줄·범위·worker 권한과 정상 검토/병합 게이트를 유지한다.
7. 정상 관측을 수행하고 전체 body·native property 보존 및 실제 guarded receipt를
   확인한 후 중앙 감독을 기존 스케줄로 유지한다. 예약 실행의 채택은 그 회차의
   실제 실행 기록으로 별도 확인한다.

이 문서는 설치 절차를 기록한다. 현재 서버 파일·자동화 설정을 변경하는 설치기는
포함하지 않는다. 이전 단일 사고 복구 helper는 해당 원본 사건에만 적용한다.

## 운영 프롬프트 재사용

- [templates/central-supervisor.md](templates/central-supervisor.md): 중앙 갱신 경로의
  공통 규칙. 실제 사용 시 현재 원본·정책·제품별 게이트를 가진 비공개 설정과 결합한다.
- [templates/queue-watch.md](templates/queue-watch.md): 읽기 전용 관측·보고 규칙.

템플릿의 자리표시자는 실제 읽기로 확정한다. 저장소의 공통 템플릿만으로 기존 전체
운영 프롬프트를 대체하지 않는다. 권한·사용량·BORI 사용자 직접 관리 제한 등 현재
운영 규칙을 함께 보존해야 한다. 이번에는 실제 두 자동화의 설정을 수정하지 않았다.

## 검증

```sh
node --test ops/notion-projection-guard/test_guard.cjs ops/notion-projection-guard/test_runtime.cjs
```

runtime 시험은 live lock 옆의 읽기 전용 실행, runtime·manifest·receipt를 함께
변조한 경우의 거부, 중복 manifest, 오래된 receipt, hosted entry drift와 CLI 실패를
확인한다. 실제 Notion 원문 대신 합성 입력만 사용한다.
실제 서버 대조와 로컬 시험 결과는
[VERIFICATION_20261020_OPERATIONS.md](VERIFICATION_20261020_OPERATIONS.md)에 기록했다.
