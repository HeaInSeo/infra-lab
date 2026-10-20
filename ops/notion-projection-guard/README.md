# Notion projection guard — 전용 저장소 연결

코드·시험·운영 문서의 정본은 [HeaInSeo/notion-projection-guard](https://github.com/HeaInSeo/notion-projection-guard)이다.
이 디렉터리에는 서버 설치 버전과 코드 조회 버전을 연결하는 명세만 남긴다.

- [보정된 고정 source 커밋](https://github.com/HeaInSeo/notion-projection-guard/tree/356532363319c8d107a9253223ea7127802560fe)
- [전용 PR #1](https://github.com/HeaInSeo/notion-projection-guard/pull/1): 정상 current-head 검토·CI·병합 게이트 유지
- [독립 CI](https://github.com/HeaInSeo/notion-projection-guard/actions): Node 22·24의 286개 시험, provenance regression 13개와 원본 35개 이력의 항목별 대조
- [설치·조회 연결 명세](deployment/source.json)

최초 이전 `089aea20`의 실행·시험 blob 15개는 불변 import와 같다. 현재 source에는
수정 시각이 없는 읽기를 거부하는 broker P1 보정과 기존 revision 시험 fixture 4파일의
시각 형식 보정이 들어 있다. 이 5파일을 설치본과 동일 bytes라고 표시하지 않는다.
CI는 작성자·시각·메시지·tree·ancestry·tag 및 고정 경로 집합을 검증한다.

`runner.js`·`complete_projection.js`의 bytes는 기존 승인본과 같다. broker는
현재 설치본과 다르다. Source 조회 전환은 broker 배포나 설치 승인 hash 갱신을 뜻하지 않는다.
원래 source `089aea20`의 실제 예약 실행은 2026-10-20T00:53:59.289Z에
body9/native4와 보호 내용 UNCHANGED로 검증됐다. 이 증거를 보정된 broker의
설치·예약 실행 채택으로 확대하지 않는다.

기존 설치 receipt의 출처는 infra-lab `01b8a7897bc8709450f536744577313c805c66ca`로 보존한다.
보정된 broker의 배포는 별도 검토·원자적 설치와 새 receipt를 필요로 한다.
서버 policy·canonical journal·원본 snapshot을 이 연결 변경으로 교체하지 않는다.

원본 `6ea3729948a9210c557b82094caee44ab3889560`는 이 저장소의
`archive/npg-before-split-20261020` 태그로 보존했다. 관련 이력 35개는 새 저장소의
`import/infra-lab-20261020` 태그와 커밋 대응표로 연결된다.
실행 코드와 NPG 전용 workflow는 새 저장소에서 관리한다. 이 연결 변경은 기존 infra-lab PR #41에서 검토한다.
