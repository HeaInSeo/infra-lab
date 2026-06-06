# Profile-Driven 환경 생성 구조

## 현재 구조의 문제점

| 문제 | 설명 |
|------|------|
| 설정 분산 | `envs/*.env`, `dev.auto.tfvars`, `cloud-init/k8s.yaml` 등 여러 파일에 중복 저장 |
| 환경 전환 불편 | 다른 CNI나 토폴로지로 바꾸려면 여러 파일을 손으로 수정해야 함 |
| 재현성 부족 | `up` 시점에 실제로 사용된 값을 나중에 확인할 방법이 없음 |
| 자동화 어려움 | 스크립트에 환경별 분기가 산재해 있음 |

---

## 핵심 원칙

1. **profile이 환경의 SoT** — `dev.auto.tfvars`, `cloud-init`, `main.tf` 등을 ilab이 임의로 수정하지 않는다.
2. **destructive 작업은 자동 실행하지 않는다** — `down`, `rebuild --approve`는 명시적 승인 필요.
3. **`switch`라는 이름으로 down→up을 묶지 않는다** — 각 단계를 독립적으로 실행.
4. **legacy 환경(root-level kubeconfig/tfstate)을 자동 마이그레이션하지 않는다**.
5. **ilab 자체 state DB를 만들지 않는다** — OpenTofu state 외에 별도 DB 없음.
6. **ilab이 VM lifecycle을 재구현하지 않는다** — 기존 `k8s-tool.sh`를 호출한다.

---

## YAML Profile 형식 명세

```yaml
name: <string>          # 환경 식별자. 생략 시 파일명 stem으로 자동 설정.

backend: libvirt | multipass

vm:
  osImage: ubuntu-24.04
  imageUrl: <URL>        # libvirt 전용. base qcow2 이미지 URL.
  masters: <int>
  workers: <int>
  master:
    cpu: <int>
    memory: <string>     # "4G", "8192M" 등 tofu 변수 형식
    disk: <string>       # "40G"
  worker:
    cpu: <int>
    memory: <string>
    disk: <string>

kubernetes:
  version: "1.32"        # NOTE: Phase 1에서는 참조용. cloud-init/k8s.yaml 값이 우선됨.
  cni: flannel | cilium

addons:
  base:
    - metrics-server
  optional:
    - cilium              # cni: cilium 일 때

libvirt:                  # backend: libvirt 일 때만 필요
  sshPrivateKeyPath: ~/.ssh/id_ed25519
  sshPublicKey: "ssh-ed25519 AAAA..."
  poolName: lab-pool
  poolPath: /var/lib/libvirt/images

state:
  dir: state/<name>      # 생략 시 "state/<name>" 으로 자동 설정
```

### 탐색 순서 (`ilab env up <name>`)

1. 절대 경로
2. `~/.config/infra-lab/profiles/<name>.yaml`
3. `<repo>/envs/<name>.yaml`
4. `<repo>/envs/<name>.yaml.example` (경고 출력 후 로드)

---

## 명령어 책임 경계

| 명령어 | 동작 | VM 영향 | destructive |
|--------|------|---------|-------------|
| `ilab env use <profile>` | profile 유효성 확인 후 `state/.current-env`에 이름 저장 | 없음 | 아니오 |
| `ilab env plan <profile>` | `tofu plan` 실행, stdout/stderr 그대로 출력 | 없음 | 아니오 |
| `ilab env up <profile>` | `scripts/k8s-tool.sh up` 호출 + resolved-profile 저장 | 생성 | 아니오 |
| `ilab env down <profile>` | `scripts/k8s-tool.sh down` 호출 | 삭제 | 예 (명시적) |
| `ilab env rebuild <profile>` | `--approve` 없으면 plan 출력 후 종료. `--approve` 있으면 down → state 삭제 → up | 재생성 | 예 (`--approve` 필요) |
| `ilab profile list` | `~/.config/infra-lab/profiles/` + `envs/*.yaml` 목록 출력 | 없음 | 아니오 |
| `ilab profile clone <src> <dst>` | src 복사, name + state.dir 업데이트 | 없음 | 아니오 |

### `ilab env up`이 k8s-tool.sh를 호출하는 방식

`HOST_PROFILE`은 사용하지 않는다. `ENV_NAME`과 각 `TF_VAR_*` 환경변수를 직접 주입한다.

```bash
ENV_NAME=libvirt-flannel \
BACKEND=libvirt \
CNI=flannel \
TF_VAR_masters=1 \
TF_VAR_workers=2 \
TF_VAR_master_cpus=2 \
TF_VAR_master_memory=4G \
TF_VAR_master_disk=40G \
TF_VAR_worker_cpus=2 \
TF_VAR_worker_memory=4G \
TF_VAR_worker_disk=50G \
TF_VAR_ssh_private_key_path=/home/user/.ssh/id_ed25519 \
TF_VAR_ssh_public_key="ssh-ed25519 AAAA..." \
TF_VAR_libvirt_pool_name=lab-pool \
TF_VAR_libvirt_pool_path=/var/lib/libvirt/images \
TF_VAR_libvirt_base_image_url=https://... \
bash scripts/k8s-tool.sh up
```

---

## 상태 구조

```
state/
  .current-env              # ilab env use 가 기록하는 현재 profile 이름
  <env-name>/
    meta                    # k8s-tool.sh 가 쓰는 key=value 파일 (SoT는 여기)
    kubeconfig
    terraform.tfstate
    resolved-profile.yaml   # ilab env up 성공 후 기록 (재현성용)
```

### resolved-profile.yaml

`ilab env up` 성공 시 `state/<env>/resolved-profile.yaml`에 저장된다.  
profile의 모든 필드 + 추가 메타데이터를 포함한다.

```yaml
# profile 필드 (인라인)
name: libvirt-flannel
backend: libvirt
# ...

# 추가 메타데이터
infraLabGitCommit: abc1234def...
createdAt: "2026-06-06T00:00:00Z"
```

이 파일은 "이 환경이 어떤 값으로 만들어졌는지"를 나중에 확인하기 위한 참조용이다.  
ilab이 이 파일을 읽어 동작을 바꾸지는 않는다.

---

## Phase 2 이후 할 일

| 항목 | 설명 |
|------|------|
| K8s 버전 변수화 | `cloud-init/k8s.yaml`의 버전을 profile의 `kubernetes.version`에서 주입 |
| multipass 이미지 변수화 | profile의 `vm.osImage`를 `TF_VAR_multipass_image`로 변환 |
| `ilab env status` → profile 연동 | status 출력에 profile 정보 포함 |
| profile 스키마 검증 | backend별 필수 필드 체크 (예: libvirt는 sshPublicKey 필수) |
| `~/.config/infra-lab/profiles/` 자동 생성 | `ilab profile clone` 시 디렉토리 없으면 자동 생성 (현재는 오류) |
| addons 연동 | profile의 `addons.base/optional`을 `addons/manage.sh`에 전달 |

---

## 미결 설계 이슈

1. **`ilab env plan`의 backend_dir 결정** — 현재는 `backends/<backend>/`를 직접 가리킨다. k8s-tool.sh의 내부 로직과 완전히 동일하지 않을 수 있어, Phase 2에서 k8s-tool.sh에 `plan` 서브커맨드를 추가하는 방안도 고려.

2. **`ilab env use`의 필요성** — 현재 `state/.current-env`를 읽는 명령어가 없다. Phase 2에서 `ilab env up` (인자 없이) 실행 시 current-env를 사용하는 방향으로 발전 가능.

3. **libvirt sshPublicKey를 profile에 넣는 보안 고려** — 공개 키라 문제없지만, 운영 환경에서는 `~/.config/infra-lab/profiles/`에 두는 것이 repo에 커밋되는 위험을 줄임.
