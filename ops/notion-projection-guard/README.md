# Notion projection guard — 전용 저장소로 이전

코드·시험·운영 문서의 정본은 [HeaInSeo/notion-projection-guard](https://github.com/HeaInSeo/notion-projection-guard)이다.
이 디렉터리에는 서버 설치 버전과 코드 조회 버전을 연결하는 명세만 남긴다.

- [검증한 고정 source 커밋](https://github.com/HeaInSeo/notion-projection-guard/tree/089aea20a24326d71f8c51f38c4e5d7bc7337cb5)
- [전용 이전 PR #1](https://github.com/HeaInSeo/notion-projection-guard/pull/1): draft·미병합, 정상 검토/병합 게이트 유지
- [독립 CI](https://github.com/HeaInSeo/notion-projection-guard/actions): Node 22·24의 271개 시험과 source/이력 보존 검사
- [설치·조회 연결 명세](deployment/source.json)

중앙 감독·읽기 전용 감시는 위 전체 커밋의 루트 경로 `runner.js`·`complete_projection.js`를 조회한다.
세 실행 파일의 bytes는 기존 검토·설치 버전과 일치한다. 실제 두 prompt readback을 확인했고 스케줄·활성 상태를 유지했다.
예약 실행에서 이 경로를 실제 사용했는지는 그 회차의 guarded receipt로 별도 확인한다.

기존 설치 receipt의 출처는 infra-lab `01b8a7897bc8709450f536744577313c805c66ca`로 보존한다.
서버 runtime·policy·approved manifest·canonical journal·원본 snapshot을 이 이전 때문에 교체하지 않는다.

원본 `6ea3729948a9210c557b82094caee44ab3889560`는 이 저장소의
`archive/npg-before-split-20261020` 태그로 보존했다. 관련 이력 35개는 새 저장소의
`import/infra-lab-20261020` 태그와 커밋 대응표로 연결된다.
실행 코드와 NPG 전용 workflow는 새 저장소에서 관리한다. 이 연결 변경은 기존 infra-lab PR #41에서 검토한다.
