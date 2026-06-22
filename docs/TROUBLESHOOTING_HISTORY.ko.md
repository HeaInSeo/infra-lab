# TROUBLESHOOTING_HISTORY

이 문서는 `infra-lab`을 실제로 bring-up 하면서 겪었던 문제와 그 해결 과정을 Git 히스토리 기준으로 정리한 학습용 문서입니다.

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

## 9. 2026-04-06 ~ 2026-04-08: 하위 PoC 재검증에서 확인한 책임 경계

`artifact-handoff-poc`의 same-node / cross-node / failure scenario 재검증을 다시 돌리면서, 이번 저장소 쪽에 남겨야 할 교훈도 하나 더 분명해졌습니다.

핵심은 "하위 PoC 실행이 실패했다"는 사실만으로 `infra-lab` 회귀라고 보면 안 된다는 점입니다.

이번 재검증에서 실제로 먼저 확인한 것은 다음이었습니다.

1. `./scripts/k8s-tool.sh status`
2. `kubectl get nodes -o wide`
3. `artifact-handoff` 네임스페이스 Pod 상태

이 확인에서는 다음이 계속 성립했습니다.

- `lab-master-0`
- `lab-worker-0`
- `lab-worker-1`

세 노드가 모두 `Ready`였고, control plane endpoint 도 정상 응답했습니다. 즉 `infra-lab` 자체는 이번 재검증 시점에 다시 깨진 것이 아니었습니다.

반대로, 하위 PoC 쪽에서 실제로 드러난 문제는 아래와 같았습니다.

- host `python3`가 3.6 계열이라 helper script 의 `text=True` 사용이 깨짐
- 이전 artifact cache 와 old pod process 영향으로 첫 cross-node 재실행이 `source=local`로 관찰됨
- sandbox 환경에서는 API server 접근이 `socket: operation not permitted`로 차단될 수 있었음

이 세 가지는 모두 `infra-lab`의 VM bring-up, kubeadm bootstrap, worker join, containerd baseline 문제와는 다른 축이었습니다.

정리하면:

- 3-node `Ready` 상태가 유지되고 있다면, 먼저 workload repo 의 script / cache / pod lifecycle 문제를 의심하는 것이 맞습니다.
- sandbox 의 네트워크 제약으로 `kubectl` 이 막히는 경우도 랩 회귀가 아니라 호출 환경 문제일 수 있습니다.
- `infra-lab` 쪽 트러블슈팅은 VM lifecycle, kubeadm/bootstrap, CNI, runtime 정렬 문제를 다루고, workload-specific validation 문제는 상위 PoC 저장소 문서로 보내는 것이 책임 경계를 유지하는 방법입니다.

이번 메모를 남기는 이유는, 이후에도 "실험이 실패했다"는 현상만 보고 인프라 저장소와 PoC 저장소의 책임을 섞지 않기 위해서입니다.

---

## 2026-04-12: Sprint I-4 과정에서 발생한 인프라 이슈 세 가지

### 이슈 1 — containerd 재시작 후 `SystemdCgroup` 불일치로 신규 Pod 생성 실패

**현상**

containerd 를 `systemctl restart containerd` 로 재시작한 직후, 신규 Pod 가 `ContainerCreating` 상태에서 멈추고 아래 오류를 반복했습니다.

```
runc create failed: expected cgroupsPath to be of format "slice:prefix:name"
for systemd cgroups, got "/kubepods/burstable/pod.../container-id" instead
```

**원인**

실제 cgroup 계층이 cgroupfs 방식(`/sys/fs/cgroup/kubepods/burstable/...`)으로 구성되어 있었는데, containerd `config.toml` 에는 `SystemdCgroup = true` 가 설정되어 있었습니다. 이전에 실행 중이던 containerd 프로세스는 이 불일치를 그냥 넘어갔지만, 재시작 이후 새 프로세스가 엄격하게 검증하면서 실패가 드러났습니다.

확인 방법:

```bash
# cgroup 계층 확인 — kubepods.slice/ 가 없으면 cgroupfs 방식
ls /sys/fs/cgroup/ | grep kube
# kubepods/ 가 보이면 cgroupfs, kubepods.slice/ 면 systemd

# containerd 설정 확인
grep 'SystemdCgroup' /etc/containerd/config.toml
```

**수정**

```toml
# /etc/containerd/config.toml
[plugins.'io.containerd.cri.v1.runtime'.containerd.runtimes.runc.options]
  SystemdCgroup = false   # true → false 로 변경
```

변경 후 containerd 재시작:

```bash
sudo systemctl restart containerd
```

**교훈**

- containerd 재시작 전에 `SystemdCgroup` 설정과 실제 cgroup 계층이 일치하는지 반드시 확인합니다.
- 기존 Pod 는 이미 shim 프로세스에 의해 유지되므로 containerd 재시작 시 죽지 않습니다. 문제는 항상 *새 Pod 생성* 단계에서 드러납니다.
- `kubelet config.yaml` 의 `cgroupDriver` 와 containerd 의 `SystemdCgroup` 이 반드시 동일한 드라이버를 가리켜야 합니다.

---

### 이슈 2 — containerd v2 에서 insecure HTTP 레지스트리 pull 실패 (hosts.toml 미적용)

**현상**

Harbor 레지스트리(`harbor.10.113.24.96.nip.io`)는 HTTP(포트 80)로 노출되어 있는데, containerd 가 HTTPS(포트 443)로 연결을 시도해 실패했습니다.

```
fetch failed: Head "https://harbor.10.113.24.96.nip.io/v2/...":
dial tcp 10.113.24.96:443: connect: no route to host
```

**배경 — 왜 hosts.toml 이 동작하지 않았나**

containerd 공식 문서가 권장하는 `hosts.toml` 방식을 `/etc/containerd/certs.d/harbor.10.113.24.96.nip.io/hosts.toml` 에 작성했음에도, containerd v2.2.1 의 CRI image 레이어에서 해당 파일이 적용되지 않았습니다. `config_path` 를 `io.containerd.cri.v1.images.registry` 와 `io.containerd.grpc.v1.cri.registry` 양쪽 모두에 추가해도 동일하게 HTTPS 로 떨어졌습니다.

**임시 해결 — `ctr --plain-http` 사전 pull**

CRI 레이어를 우회해 `ctr` 로 직접 pull 하는 방법이 동작합니다.

```bash
sudo ctr --namespace k8s.io images pull \
  --plain-http \
  --user 'admin:Harbor12345' \
  harbor.10.113.24.96.nip.io/nodeforge/controlplane:latest
```

이미지가 `k8s.io` 네임스페이스에 올라가 있으면 kubelet 은 `IfNotPresent` 정책에서 pull 을 건너뜁니다.

**영구 해결 방향**

- containerd v2.x 버전 별로 `hosts.toml` 적용 방식이 달라지는 경우가 있습니다. 공식 릴리스 노트나 이슈 트래커를 확인하거나, `certs.d/_default/hosts.toml` 에 글로벌 HTTP 허용 규칙을 추가하는 방법을 검토합니다.
- 또는 Harbor 앞에 TLS termination(cert-manager + 자체 CA)을 붙여 HTTPS 로 전환하면 이 문제 자체가 사라집니다.

**교훈**

- `hosts.toml` 작성 후에는 반드시 `ctr --hosts-dir` 또는 `crictl pull` 로 적용 여부를 독립적으로 검증합니다.
- K8s 레벨에서 pull 실패가 나더라도, `ctr --plain-http` 로 격리 테스트를 먼저 해보면 인프라 문제와 K8s 설정 문제를 빠르게 구분할 수 있습니다.

---

### 이슈 3 — NodeForge K8s 배포 방식 철회: buildah 제약과 아키텍처 결정

**당초 계획 (Sprint I-4)**

NodeForge controlplane 을 K8s Deployment(`nodeforge-system` 네임스페이스)로 배포하고 Cilium GRPCRoute 로 NodeKit 과 연결하는 방식을 시도했습니다.

**문제**

NodeForge 가 이미지를 빌드하기 위해 buildah(podbridge5)를 사용하는데, K8s Pod 안에서 buildah 를 실행하려면 다음 제약이 겹칩니다.

- `privileged: true` 필요 (보안 정책 위반 가능성)
- `/dev/fuse` 장치 마운트 필요 (fuse-overlayfs)
- user namespace 지원 필요 (rootless buildah)
- Multipass VM 환경에서는 이 조합이 안정적으로 동작하지 않음

**결정**

NodeForge 는 K8s 클러스터 바깥, seoy 장비에서 독립 프로세스로 실행합니다.

```
seoy 장비 (standalone process)
  └── nodeforge binary
        ├── buildah 로 이미지 빌드
        ├── Harbor 에 push (skopeo 또는 buildah push)
        ├── L3: K8s dry-run — Job 스펙 스키마 검증
        ├── L4: K8s smoke run — 실제 Job 실행해 이미지 동작 확인
        └── RegisteredToolDefinition 생성 → NodeKit 반환
              └── kubeconfig (lab-master-0 접근, L3/L4 전용)
```

- 이미지 build/push 에는 seoy 장비의 buildah + skopeo 를 직접 사용. 커널 capability 제약이 없습니다.
- K8s 접근은 L3/L4 검증 전용입니다. "Job 오케스트레이션"이 아니라 "검증용 Job 실행"입니다.
- `nodeforge-builds`, `nodeforge-smoke` 네임스페이스는 L3/L4 검증 Job 수용 목적으로 유지합니다.
- `nodeforge-system` 네임스페이스, Deployment, Gateway, GRPCRoute 는 삭제했습니다.

**교훈**

- buildah / rootless 컨테이너 빌드 도구를 K8s Pod 안에서 실행하면 privileged, fuse, user namespace 세 가지가 동시에 맞아야 합니다. VM 기반 랩 환경에서는 이 조합이 쉽게 깨집니다.
- "K8s에 올린다"는 선택이 항상 옳지 않습니다. 이미지 빌드처럼 OS 수준 권한이 필요한 작업은 호스트 프로세스로 두고 K8s API 만 호출하는 패턴이 더 안전합니다.

---

## 2026-04-13: Sprint I-4 연동 테스트 — NodeForge 버그 3건 수정

### 배경

NodeForge standalone 구축(I-4) 완료 후 NodeKit → NodeForge 전체 파이프라인 연동 테스트를 실행했습니다.
`grpcurl`로 `BuildService/BuildAndRegister` 를 직접 호출해 L2→L3→L4→등록 전 단계를 검증했습니다.

---

### 버그 1: Harbor HTTPS 없이 containerd 가 이미지 pull 불가

**현상**

L4 smoke run 에서 K8s Pod 가 `harbor.10.113.24.96.nip.io/...` 이미지를 pull 할 때 오류:

```
failed to resolve image: dial tcp 10.113.24.96:443: connect: no route to host
```

containerd v2.2.1 은 어떤 `hosts.toml` 설정을 해도 Harbor 를 HTTPS 로 접근하려 했고, Harbor 는 포트 443 을 열지 않은 상태였습니다.

**원인**

- containerd v2.2.1 의 `[registry]` 섹션과 `grpc.v1.cri.registry` 둘 다 "unknown key"로 무시됨
- `cri.v1.images.registry.config_path` 만 유효하나, CRI pull 경로에서는 여전히 HTTPS 강제
- Harbor 는 HTTP:80 전용이었음

**해결**

cert-manager 를 이용해 Harbor 에 HTTPS(443) 엔드포인트를 추가했습니다.

```
ClusterIssuer(selfsigned-issuer)
  └─ Certificate(lab-ca, cert-manager ns) → ClusterIssuer(lab-ca-issuer)
        └─ Certificate(harbor-tls, harbor ns) → Secret(harbor-tls)
              └─ Gateway listener 추가 (HTTPS:443, TLS terminate)
```

1. `ClusterIssuer` self-signed CA 생성
2. CA 인증서를 모든 K8s 노드의 시스템 신뢰 목록에 배포 (`update-ca-certificates`)
3. containerd 재시작 → CA 신뢰 적용
4. Cilium `Gateway` 에 HTTPS:443 리스너 추가, `harbor-tls` Secret 마운트
5. `HTTPRoute` 를 HTTP/HTTPS 리스너 양쪽에 연결

**교훈**

- containerd v2.2.1 + Cilium Gateway API 환경에서 insecure registry 설정은 신뢰할 수 없음
- cert-manager 가 이미 설치된 경우 HTTPS 추가가 훨씬 간단하고 영구적

---

### 버그 2: BuildDockerfileContent 가 로컬 태그만 하고 Harbor 에 push 하지 않음

**현상**

NodeForge 로그에 "image pushed to harbor.xxx/smoke-test-tool:latest" 메시지가 나왔으나 Harbor 에 이미지가 없었음.
L4 smoke run Pod 가 `not found` 오류로 실패.

**원인**

`podbridge5.BuildDockerfileContent` 는 `Output` 필드에 Harbor 주소를 지정해도 로컬 빌드+태그만 수행합니다.
registry 로 push 하는 단계가 없었습니다. 서비스 코드는 build 반환 즉시 `PUSH_SUCCEEDED` 이벤트를 보내 성공처럼 보였습니다.

**수정** (`pkg/build/builder.go`)

`Build()` 메서드에 `buildah push --tls-verify=false --digestfile /tmp/...` exec 호출을 추가했습니다.
`--digestfile` 옵션으로 레지스트리가 실제로 할당한 digest 를 파일에 받아 반환합니다.

```
로컬 build digest ≠ Harbor 원격 digest (압축·manifest 재계산 차이)
→ --digestfile 없이 로컬 digest 를 사용하면 K8s 가 "not found" 오류를 냄
```

---

### 버그 3: Harbor 단일 컴포넌트 경로 거부

**현상**

`buildah push harbor.10.113.24.96.nip.io/smoke-test-tool:latest` 가 Harbor 에서 400 Bad Request.

```
"bad request: invalid repository name: smoke-test-tool"
```

**원인**

Harbor 는 `<project>/<repository>` 형식을 요구합니다. `smoke-test-tool` 는 단일 컴포넌트라 거부됩니다.

**수정** (`pkg/build/service.go`)

destination 형식을 `harbor.xxx/library/<tool-name>:latest` 로 변경했습니다. `library` 프로젝트는 Harbor 기본 public 프로젝트입니다.

---

### 버그 4: request_id 가 빈 문자열일 때 smoke Job 이름이 RFC 1123 위반

**현상**

L3 dry-run 에서:

```
Job.batch "dry-nfsmoke-" is invalid: metadata.name: Invalid value: "dry-nfsmoke-"
```

**원인**

`sanitizeName("")` 은 빈 문자열을 반환합니다. 결과 Job 이름 `nfsmoke-` 는 끝에 하이픈이 붙어 RFC 1123 에 위반됩니다.

**수정** (`pkg/build/service.go`)

`reqID` 가 비거나 sanitize 결과가 빈 문자열이면 16진수 타임스탬프 suffix 로 대체합니다:

```go
jobSuffix := sanitizeName(reqID)
if jobSuffix == "" {
    jobSuffix = fmt.Sprintf("%04x", time.Now().UnixMilli()%0xFFFF)
}
```

---

### 최종 연동 테스트 결과

```
grpcurl BuildService/BuildAndRegister (tool_name: smoke-test-tool)

L2 build   ✅  buildah build → buildah push → Harbor library/smoke-test-tool:latest
L3 dry-run ✅  K8s server-side dry-run passed
L4 smoke   ✅  K8s Job nfsmoke-xxxx 실행 → "smoke-ok" 출력 → 성공
등록        ✅  cas=894646b2e0b16db023f2d1c18116bc5bf8896b60023741569b7158e26deb09d6
```

전체 BuildAndRegister 파이프라인이 처음으로 end-to-end 성공했습니다.

---

### 2026-06-21: NodeVault 인-Pod 빌드가 27시간 이상 멈춤 (조사 진행 중)

**현상**

`nodevault-controlplane` Pod에서 제출된 두 빌드(`test-alpine-tool`, `test-bad-dockerfile`)가
거의 동시(24ms 차이)에 시작된 뒤, 이후 27시간 이상 로그가 전혀 추가되지 않고 멈춤.
두 빌드 모두 성공/실패 로그 없이 무한 대기 상태.

**처음 세운 가설과 반증 (오류 정정)**

1. (오류) Cilium L2 announcement policy가 죽은 `mpqemubr0` 인터페이스를 참조해서 Harbor가
   네트워크적으로 단절됐다고 추정했음. → 실제 live policy(`lab-default-l2`, 2026-06-16 재생성)는
   `mpqemubr0`을 전혀 참조하지 않음(정규식 `^eth.*|^enp.*|^ens.*` 기반 인터페이스 매칭).
   28일 전 stale snapshot 문서(`profiles/remote-seoy/cilium/live-snapshot/`)를 live 상태로
   잘못 신뢰한 결과였음.
2. (오류) Harbor가 네트워크적으로 전반적으로 도달 불가능하다고 추정했음. → `https://harbor.lab.local/v2/`
   경로는 seoy 호스트와 Pod 내부 양쪽 모두에서 정상(401 Unauthorized, 수십 ms 내 응답).
   단, HTTP:80 listener는 2026-06-16 클러스터 재구축 이후 실제로 사라짐(Gateway가 443만 노출) —
   이 사실 자체는 맞지만, Buildah는 기본적으로 HTTPS를 사용하므로(`registries.conf`에
   `harbor.lab.local`용 `insecure=true` override 없음) 빌드 멈춤의 원인은 아닌 것으로 확인됨.

**실제로 확인된 사실**

- Harbor가 제시하는 TLS 인증서는 `harbor-lab-ca`(2026-06-16 발급, 자체서명)이며,
  `nodevault-controlplane` Pod 안에는 이 CA를 신뢰하는 경로가 전혀 없음
  (`/etc/containers/certs.d/`, `/etc/pki/ca-trust/source/anchors/` 모두 비어 있음,
  Deployment에도 CA를 주입하는 볼륨 마운트 없음, NodeVault 코드에도
  `certs.d`/`harbor-ca`/`InsecureSkipVerify` 참조가 전혀 없음).
- 다만 실제 TLS 검증을 수행하는 클라이언트(curl)로 동일 조건을 재현하면
  `unable to get local issuer certificate`로 0.15초 안에 빠르게 실패함 — 즉 CA 미신뢰
  자체만으로는 "27시간 멈춤" 현상이 설명되지 않음.
- NodeVault `pkg/build/builder.go`의 `podbridge5Builder`는 Service 시작 시 단 하나의
  `storage.Store` 인스턴스를 생성해 모든 빌드 요청이 이를 공유하며, Go 레벨 mutex로
  보호되지 않음.
- `pkg/build/submit_tool_build.go:167`의 `startSubmittedBuild`는 빌드마다
  `context.WithCancel(context.Background())`만 사용 — 데드라인이 전혀 없음.
  `CancelToolBuild` RPC를 명시적으로 호출하지 않으면 영원히 취소되지 않음.
- 거의 동시에(24ms 차이) 제출된 두 빌드가 같은 `store`에 동시 접근했고,
  `/var/lib/nodevault/containers` 밑 `overlay/l`, `overlay-images`, `overlay-containers`에는
  lock 파일만 있고 실제 레이어 데이터가 전혀 없음 — 두 빌드 모두 매우 초기 단계에서
  멈춘 것으로 보임.

**현재 가장 유력한 가설 (미확정)**

`test-bad-dockerfile`(의도적으로 잘못된 Dockerfile로 보임)을 처리하는 과정에서
podbridge5/Buildah 내부 로직이 lock을 쥔 채로 멈췄고, 같은 `store`를 공유하는
`test-alpine-tool` 빌드가 그 lock을 기다리며 같이 멈춰 있을 가능성이 있음.
두 빌드 모두 deadline이 없는 context이므로 영원히 풀리지 않음.

**다음 단계 (제안, 아직 미실행 — 운영자 승인 필요)**

1. `nodevault-controlplane` Pod 재시작으로 멈춘 빌드 상태 초기화 (현재 운영 중인 Pod이므로 승인 필요).
2. 빌드를 동시에 두 개 제출하지 않고 하나씩 순차 제출해서 재현되는지 확인.
3. (코드 수정 제안, 별도 승인 필요) `builder.go`에 store 접근 직렬화(mutex) 추가,
   `submit_tool_build.go`의 빌드 context에 최대 빌드 시간 데드라인 추가.

**참고**: 이 항목은 라이브 클러스터 진단 결과를 기록한 것으로, 위 항목들과 달리 아직
커밋으로 해결되지 않은 진행 중 조사 내용입니다. 결론이 나면 이 섹션을 갱신하거나
별도 "수정" 섹션을 추가할 것.

**[2026-06-21 추가 업데이트] 근본 원인 확정: overlay mount `userxattr: invalid argument`**

Pod를 재시작한 뒤 빌드를 동시에 두 개가 아니라 **하나만** 공식 통합 테스트
(`go test -tags "integration exclude_graphdriver_btrfs containers_image_openpgp
exclude_graphdriver_devicemapper" -run TestBuildAndRegister_SimpleDockerfile ...`)로
제출해 재현한 결과, 빌드는 **멈추지 않고 0.03초 안에 즉시, 깨끗하게 실패**했습니다:

```
[BUILD_EVENT_KIND_FAILED] build image: imagebuildah.BuildDockerfiles: imagebuildah.BuildDockerfiles:
mounting an overlay over build context directory: creating overlay scaffolding for build context
directory: mount overlay:/var/tmp/buildah-context-.../merge, data: lowerdir=/,
upperdir=/var/tmp/buildah-context-.../upper, workdir=/var/tmp/buildah-context-.../work,
userxattr: invalid argument
```

이로써 이전에 세운 "두 빌드가 동시에 제출되어 store lock을 두고 경쟁/교착 상태에
빠졌다"는 가설은 단일 빌드만으로도 즉시 재현되므로 **불필요한 가설이었음이 확인**됐습니다.
(다만 두 빌드가 동시에 이 실패 경로를 타면서 lock 정리가 제대로 안 되어 실제 27시간
멈춤으로 이어졌을 가능성은 남아 있음 — 아래 "남은 의문" 참고)

**원인 분석**

Buildah가 build context 디렉터리를 overlay로 감싸는 과정에서 `lowerdir=/`
(컨테이너 자신의 root 파일시스템)을 lower layer로 사용하려 시도합니다.
이 root 파일시스템 자체가 이미 containerd(`containerd://2.2.1`)의 overlayfs
storage driver로 마운트된 상태이므로, 그 위에 다시 overlay를 쌓는
"overlay-on-overlay 중첩" 시도가 됩니다. `lab-worker-0` 노드
(Ubuntu 24.04.4, 커널 `6.8.0-117-generic`)에서 이 중첩 마운트 시
`userxattr` 옵션이 거부되어 `invalid argument`로 실패합니다.

커널 자체는 충분히 최신(6.8)이라 `userxattr` 기능 자체가 없는 문제는 아니며,
containerd의 overlay 마운트 옵션과 Buildah가 기대하는 중첩 overlay 옵션 간의
호환성 문제로 보입니다.

**남은 의문**

- 이 "즉시 실패" 경로와, 2026-06-20에 관찰된 "27시간 동안 로그 없이 멈춤" 현상이
  정확히 같은 코드 경로인지는 아직 100% 확인되지 않았습니다. 가능성: 동일한
  overlay 실패가 두 빌드에서 동시에 발생했을 때, 에러 처리/락 해제 경로에 버그가 있어
  cleanup이 안 끝나고 멈췄을 수 있습니다.
- 이 부분은 코드 수정(재현 + 수정) 없이는 결론을 내리기 어려우며, 별도 승인 후
  진행 여부를 결정해야 합니다.

**다음 단계 제안 (미실행, 승인 필요)**

1. `pkg/build/builder.go` 또는 podbridge5 쪽에서 build-context overlay 단계를
   우회/비활성화할 수 있는 옵션이 있는지 확인 (예: `--jobs=1`, context를 tar로
   복사하는 방식 등 overlay 없이 build context를 준비하는 대안).
2. 위 옵션이 없다면 containerd의 overlay 마운트 옵션(`metacopy`, `index` 등)을
   조정해 nested overlay가 가능하도록 노드 설정을 바꿔야 할 수도 있음 — 이는
   infra-lab의 cloud-init/containerd 설정 변경이 필요한 더 큰 작업.
3. `submit_tool_build.go`의 빌드 context에 데드라인을 추가하는 것은 이 특정
   실패와 무관하게 여전히 권장되는 방어적 수정.

**[2026-06-22 후속 업데이트] 위 1번 제안 사항 구현 완료**

위 "다음 단계 제안"의 1번(build-context overlay 우회)은 NodeVault
`pkg/build/builder.go`에서 build-context lowerdir을 컨테이너 root(`/`)가 아닌
전용 디렉터리(`/tmp/nodevault-build/context`, `/tmp` emptyDir 마운트 위)로 분리하는
방식으로 구현·검증 완료되었습니다. `lowerdir=/`을 쓰지 않게 되어 containerd의
root overlay 위에 또 다른 overlay를 중첩하는 상황 자체가 사라졌습니다(2번 제안의
containerd 설정 변경은 불필요해졌습니다). 3번(빌드 데드라인 추가)은 여전히 미실행
상태입니다. 상세 내용: `HeaInSeo/NodeVault` issue #4.

## 2026-06-22: NodeVault(구 NodeForge) in-Pod 빌더 — Harbor 자체서명 CA 신뢰 갭

### 배경

NodeVault(이 저장소 11번 항목의 NodeForge가 이후 K8s in-Pod 빌드 방식으로 다시 전환된
프로젝트)의 overlay/userns 버그를 고친 뒤 라이브 통합 테스트를 재실행하자, build가 pull
단계까지 도달한 뒤 다음 에러로 막혔습니다.

```
initializing source docker://harbor.lab.local/nodevault/controlplane:latest:
pinging container registry harbor.lab.local: Get "https://harbor.lab.local/v2/":
tls: failed to verify certificate: x509: certificate signed by unknown authority
```

### 원인

`harbor-install.sh`가 각 **노드**의 containerd 트러스트(certs.d)에 Harbor CA를 배포하므로,
kubelet의 일반 `imagePullPolicy` pull은 문제없이 동작합니다. 하지만 NodeVault처럼 **Pod
내부**에서 Buildah/Podman이 직접 Harbor에 pull/push하는 워크로드는 이 노드 레벨 신뢰
설정을 공유하지 않습니다 — Pod는 별도 파일시스템/프로세스이기 때문입니다. NodeVault가
쓰는 podbridge5(Buildah Go API 래퍼)는 `types.SystemContext`의 `DockerCertPath`를 채우지
않으므로 `go.podman.io/image/v5`의 기본 탐색 경로인
`/etc/containers/certs.d/<registry-domain>/*.crt`로 떨어지는데, 거기에는 아무것도
마운트되어 있지 않았습니다.

### 수정

`infra-lab`/Harbor 설치 절차의 결함이 아니라 **각 소비 워크로드가 자기 책임으로
마운트해야 하는 부분**입니다 — infra-lab 쪽에서 일괄 해결할 수 없습니다(어떤 Pod가 자체
레지스트리 클라이언트를 갖는지는 워크로드마다 다릅니다). NodeVault 쪽 수정은 K8s Secret +
volumeMount만으로 충분했습니다(코드 변경 없음):

```bash
kubectl create secret generic <name>-harbor-ca \
  --from-file=ca.crt=~/.config/infra-lab/certs/harbor-ca.crt \
  -n <namespace>
```

```yaml
volumeMounts:
  - name: harbor-ca
    mountPath: /etc/containers/certs.d/harbor.lab.local
    readOnly: true
volumes:
  - name: harbor-ca
    secret:
      secretName: <name>-harbor-ca
```

라이브 클러스터에서 Secret 생성 + 매니페스트 재적용 후 공식 통합 테스트로 검증했고,
build → push → digest → L3 dry-run → L4 smoke run → 카탈로그 등록까지 전 구간이 TLS
에러 없이 통과했습니다. 전체 분석과 검증 로그는 `HeaInSeo/NodeVault` issue #2(overlay),
#3(이 Harbor CA 갭), #4(후속 build-context 디렉터리 정리)에 기록되어 있습니다.

### 남은 갭 (비차단)

`net/http`/시스템 CA 풀을 쓰는 클라이언트(NodeVault의 ORAS referrer push 등)는 위 마운트로
해결되지 않습니다 — 시스템 트러스트 스토어(`/etc/ssl/certs`, `update-ca-certificates`)에
별도로 추가해야 합니다. NodeVault에서는 이 경로가 이미 의도적으로 비차단
(`integrity_health=Partial`) 처리되어 있어 당장 막힌 곳은 아니지만, 일반 해결책은 아직
없습니다.

### 교훈

- "노드가 Harbor CA를 신뢰한다"와 "Pod 내부 프로세스가 Harbor CA를 신뢰한다"는 별개의
  신뢰 경계입니다. kubelet의 `imagePullPolicy` pull이 성공한다고 해서 Pod 안에서 직접
  레지스트리에 붙는 코드도 성공한다고 가정하면 안 됩니다.
- containers/image 계열 라이브러리(Buildah, Podman, skopeo)와 순수 `net/http` 기반
  클라이언트는 서로 다른 CA 신뢰 경로를 탐색합니다 — 하나를 고쳤다고 둘 다 고쳐지지
  않습니다.
