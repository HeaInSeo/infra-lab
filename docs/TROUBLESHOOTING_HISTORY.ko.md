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
