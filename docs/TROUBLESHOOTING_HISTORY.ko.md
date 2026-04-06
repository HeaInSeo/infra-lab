# TROUBLESHOOTING_HISTORY

이 문서는 `multipass-k8s-lab`을 실제로 bring-up 하면서 겪었던 문제와 그 해결 과정을 Git 히스토리 기준으로 정리한 학습용 문서입니다.

핵심 목적은 다음 두 가지입니다.

- 같은 문제가 다시 발생했을 때 추측이 아니라 근거 있는 순서로 진단하기
- 왜 현재 베이스라인이 `Ubuntu 24.04 + containerd + kubeadm` 형태가 되었는지 맥락을 남기기

## 요약

이번 스프린트에서 실제로 겪은 핵심 문제는 세 가지였습니다.

1. 초기 guest baseline 이 현재 Multipass 환경과 맞지 않아 `rocky-8` 이미지 별칭으로 3-node bring-up 이 실패함
2. Ubuntu 24.04 전환 후 worker join 과정에서 원격 `join.sh` 실행 권한 문제로 worker 합류가 흔들림
3. control plane 회복 과정에서 containerd / CRI / kubelet cgroup 정렬을 잘못 읽으면 오히려 문제가 더 커질 수 있었음

결론적으로 현재 베이스라인은 단순히 "Ubuntu 로 바꿨다" 수준이 아니라, 실제 장애를 겪고 정리한 결과물입니다.

## 히스토리 타임라인

### 2026-04-03: 초기 baseline 과 첫 bring-up 실패

관련 커밋:

- `b5b3c2c` `bootstrap multipass k8s lab baseline`

초기 baseline 은 Rocky 계열 guest 전제를 강하게 갖고 있었습니다.

- 기본 `multipass_image = "rocky-8"`
- 기본 `vm_user = "rocky"`
- cloud-init 도 RPM 계열 설치 흐름 중심

문제는 현재 Multipass 환경에서 `rocky-8` alias 기반 접근이 안정적으로 동작하지 않았다는 점입니다. 이 상태에서는 `./scripts/k8s-tool.sh up` 자체가 3-node 구성까지 정상적으로 진행되지 못했습니다.

이 실패가 중요했던 이유는, 인프라가 안 올라오면 상위 PoC 검증도 전부 막힌다는 점입니다. 실제로 `artifact-handoff-poc`에서는 이 시점에 full 3-node 검증이 막혔고, 남아 있던 단일 노드로 same-node 시나리오만 부분 검증하게 됐습니다.

### 2026-04-03: guest baseline 을 Ubuntu 24.04 로 전환

관련 커밋:

- `4843572` `Switch lab baseline guest to Ubuntu 24.04`

이 커밋은 단순 문자열 변경이 아니라, guest OS 전환에 필요한 실행 경로 전체를 바꿨습니다.

주요 변경:

- `dev.auto.tfvars` 기본값을 `rocky-8` 에서 `24.04` 로 변경
- `vm_user` 를 `rocky` 에서 `ubuntu` 로 변경
- `cloud-init/k8s.yaml` 을 RPM/dnf 흐름에서 apt 기반 설치 흐름으로 변경
- `scripts/cluster/cluster-init.sh`, `scripts/cluster/join-all.sh`, `scripts/multipass/multipass-run-remote.sh` 의 기본 사용자 전환
- README 와 scope 문서의 baseline 설명을 Ubuntu 24.04 기준으로 갱신

여기서 중요한 학습 포인트는, 이미지 이름만 바꾸는 것으로 끝나지 않는다는 점입니다.

- guest 유저 이름
- 패키지 매니저
- Kubernetes repo 설정 방식
- `containerd` 설치 경로와 초기화 방식

이 네 가지가 함께 바뀌어야 부트스트랩이 실제로 동작합니다.

### 2026-04-04: worker join 실행 권한 문제 수정

관련 커밋:

- `9a704d6` `Fix worker join script permissions`

문제는 worker VM 안에서 `${VM_HOME}/join.sh` 를 실행할 때 권한이 충분하지 않았다는 점입니다.

수정 전:

```bash
chmod +x ${VM_HOME}/join.sh && sudo bash ${VM_HOME}/join.sh
```

수정 후:

```bash
sudo chmod +x ${VM_HOME}/join.sh && sudo bash ${VM_HOME}/join.sh
```

이 차이는 작아 보이지만 중요합니다. 원격에서 파일이 어떤 권한과 소유권으로 떨어지느냐에 따라, 일반 사용자 권한으로는 실행 비트 변경이 실패할 수 있습니다. worker join 단계는 한 번 어긋나면 "노드가 안 붙는다"는 현상만 보이기 때문에, 실제 원인이 권한인지 토큰인지 네트워크인지 구분이 어려워집니다.

즉, 이 커밋은 Kubernetes join 로직 자체가 아니라 원격 실행 환경의 권한 정렬 문제를 고친 것입니다.

### 2026-04-04: control plane 불안정과 runtime 해석 오류

관련 커밋:

- `0e660a5` `Document Rocky8 runtime troubleshooting note`

이 커밋 자체는 README 에 한 줄 메모를 추가한 것이지만, 배경이 더 중요합니다. 실제 장애 상황은 다음과 같았습니다.

- `kube-apiserver` 가 반복적으로 종료됨
- `6443` 연결이 자주 refused 됨
- `etcd` / `kube-apiserver` static pod 가 반복 재생성됨
- 노드는 보이지만 `NotReady` 상태가 이어짐

처음에는 흔한 cgroup mismatch 문제처럼 보였습니다.

- kubelet 은 `systemd` 를 기대하는데
- `crictl info` 에서 top-level `systemdCgroup=false` 가 보임

하지만 실제로는 그 값을 단독으로 읽으면 안 됐습니다.

실제로 중요했던 점:

- runtime 은 `io.containerd.runc.v2`
- `containerd 1.7.28` 환경에서는 top-level 값보다 `runc.options.SystemdCgroup = true` 가 더 중요함
- top-level `systemd_cgroup=true` 를 섣불리 강제하면 오히려 CRI plugin 자체가 깨질 수 있음

이 구간의 핵심 학습 포인트는 "증상은 kube-apiserver 실패인데 원인은 runtime 정렬"일 수 있다는 점, 그리고 "`crictl info` 한 줄만 보고 수정하면 더 악화될 수 있다"는 점입니다.

## 실제 회복 순서

문제 회복은 다음 순서로 진행됐습니다.

1. 잘못 건드린 top-level containerd 설정을 되돌림
2. `containerd` 재시작
3. `kubelet` 재시작
4. control plane static pod 안정화 확인
5. flannel/CNI 회복 확인
6. 3-node 모두 `Ready` 상태 확인

여기서 중요한 것은 `etcd` 나 `kube-apiserver` manifest 를 먼저 뜯지 않았다는 점입니다. static pod 가 계속 재생성된다고 해서 바로 control plane manifest 를 수정하는 쪽으로 가면, 원인과 결과를 뒤집을 수 있습니다.

이번 사례에서는 런타임 정렬을 먼저 바로잡는 것이 맞았습니다.

## 다시 같은 문제가 나면 보는 순서

1. `kubectl get nodes -o wide`
2. `kubectl get pods -A -o wide`
3. `sudo crictl info`
4. `sudo systemctl show -p ExecStart containerd`
5. `/etc/containerd/config.toml` 의 `runc.options.SystemdCgroup`
6. `ss -lntp | egrep '6443|2379|2380|10257|10259'`
7. `kubectl -n kube-flannel get pods -o wide`

이 순서는 "API server 가 죽었다"는 현상만 보는 대신, 노드 상태, 런타임, control plane 포트, CNI 회복 상태를 함께 보도록 의도한 것입니다.

## 왜 이 문서가 `artifact-handoff-poc` 에도 중요했는가

이 저장소의 장애는 인프라 저장소 안에서 끝나는 문제가 아니었습니다. `artifact-handoff-poc`의 Sprint 1 검증은 다음 순서로 영향을 받았습니다.

- 3-node 랩 준비 실패
- same-node 만 단일 노드에서 부분 검증
- control plane 회복 후 cross-node peer fetch 검증 가능
- 최종적으로 실제 3-node 랩에서 cross-node 성립 확인

즉, 이 트러블슈팅 히스토리는 단순한 infra 메모가 아니라 상위 PoC 검증 성패를 갈랐던 경로입니다.

## 남겨 둘 교훈

- Multipass guest baseline 은 "이론상 가능한 이미지"보다 "현재 카탈로그에서 안정적으로 재현 가능한 이미지"가 우선입니다.
- kubeadm 장애처럼 보여도 실제 원인은 guest user, 권한, cloud-init, package path, containerd runtime 정렬일 수 있습니다.
- `crictl info` 의 단일 필드만 보고 수정하지 말고 runtime 옵션과 CRI plugin 생존 여부를 같이 봐야 합니다.
- control plane static pod 문제를 보더라도 manifest 수정은 마지막 단계여야 합니다.
- 상위 워크로드 PoC 결과를 해석할 때는 항상 인프라 준비 상태와 분리해서 봐야 합니다.
