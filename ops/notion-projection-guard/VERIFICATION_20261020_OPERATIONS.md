# 2026-10-20 운영 자료 정리 검증

## 범위

기존 ACTIVE PR #41에 운영 문서, 공통 프롬프트 템플릿, 설치 버전 명세,
읽기 전용 검증 도구와 회귀 시험을 추가한다. broker·runner·complete entry·policy의
실행 bytes는 기존 검토 커밋 `01b8a7897bc8709450f536744577313c805c66ca`와 동일하다.
실제 자동화 프롬프트·스케줄·서비스와 canonical state는 이번 작업으로 변경하지 않았다.

## 수행한 검증

- Node 24.19.0: `node --test ops/notion-projection-guard/test_guard.cjs ops/notion-projection-guard/test_runtime.cjs`
  **271 PASS / 0 FAIL**. 기존 264개와 새 runtime 대조 시험 7개다.
- 로컬 소스의 broker·runner·complete entry SHA256과 Git blob을 기존 고정 커밋에서
  읽은 값 및 배포 명세와 대조했다. 원본 Notion 자료는 시험에 사용하지 않았다.
- **2026-10-20 00:11:07.262 UTC / 09:11:07 KST**, 실제 서버에서 새 검증 모듈을
  메모리로 실행했다. 도구 파일을 서버에 설치하지 않았고, broker 실행이나 journal·lock
  변경 없이 현재 runtime·승인 manifest·기존 설치 receipt를 읽었다. 결과는 다음과 같다.

```json
{
  "status": "MATCH",
  "sourceCommit": "01b8a7897bc8709450f536744577313c805c66ca",
  "installedAt": "2026-10-19T23:02:49.009Z",
  "sourceFilesChecked": false
}
```

서버 대조는 설치 파일과 receipt에 한정했다. 별도의 로컬 소스 대조에서 세 소스 파일의
해시를 확인했다. GitHub CI의 Node 22·24 matrix에도 두 시험 파일을 연결했다.
해당 새 커밋의 CI 결과와 리뷰는 GitHub의 실제 실행 기록을 기준으로 확인한다.

## 한계와 보존

과거 커밋의 clean 리뷰는 새 검증 도구의 리뷰를 대신하지 않는다. PR은 기존 draft와
정상 병합 게이트를 유지한다. 파일 MATCH는 제품 검수나 예약 자동화의 실제 채택을
증명하지 않는다. 운영 정책 원문, journal·원본 snapshot·receipt·전체 prompt와
인증자료는 이 공개 변경에 포함하지 않았다.
