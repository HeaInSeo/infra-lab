# ilab

`ilab`은 infra-lab 환경을 검사하는 읽기 전용 CLI 도구이다.

OpenTofu 상태, kubeconfig, VM 런타임, Kubernetes API를 조합해 환경의 현재 상태를 한눈에 보여준다.
상태를 직접 변경하지 않는다 — 소스 오브 트루스는 항상 tofu 상태, VM 런타임, Kubernetes API다.

---

## 왜 만들었나

`infra-lab`으로 Kubernetes 클러스터를 운영하다 보면 다음과 같은 질문이 반복된다.

- "지금 어떤 환경이 올라와 있지?"
- "VM이 실제로 실행 중인가?"
- "이 kubeconfig가 어느 클러스터를 가리키나?"
- "cluster-init 할 때 어떤 버전을 썼더라?"

이 질문들은 `tofu state show`, `virsh list`, `kubectl get nodes`, `multipass list` 등
여러 도구를 따로 실행해야 답할 수 있다. `ilab`은 이 정보를 한 곳에서 읽어 일관된 방식으로 보여준다.

---

## 구조

```
ilab/
├── main.go               # 진입점
├── cmd/
│   ├── root.go           # cobra 루트 커맨드 및 INFRA_LAB_ROOT 설정
│   ├── doctor.go         # ilab doctor — 사전 요구사항 및 환경 진단
│   ├── env.go            # ilab env list / status
│   ├── vm.go             # ilab vm list / ssh / version
│   └── k8s.go            # ilab k8s status
└── internal/
    └── lab/
        ├── lab.go        # 핵심 로직: 환경 로드, VM 열거, kubeconfig 해석
        └── lab_test.go   # 단위 테스트
```

### 동작 방식

1. `ilab`은 현재 디렉토리에서 위로 올라가며 `scripts/k8s-tool.sh`가 있는 곳을 저장소 루트로 인식한다.  
   `INFRA_LAB_ROOT` 환경변수로 직접 지정할 수도 있다.

2. 환경은 `state/<env-name>/meta` 파일로 식별된다.  
   `meta` 파일은 `k8s-tool.sh up` 실행 후 자동으로 생성된다.

3. VM 목록은 백엔드에 따라 다른 방법으로 읽는다.
   - **multipass**: `multipass list --format json`
   - **libvirt**: `virsh -c qemu:///system list --all --name` + `virsh domifaddr`

4. Kubernetes 상태는 `kubectl`을 호출해 출력한다.  
   kubeconfig는 `KUBECONFIG` 환경변수 → `state/<env>/kubeconfig` 순으로 찾는다.

---

## 빌드

```bash
# 저장소 루트에서
make build        # bin/ilab 생성
make install      # $(go env GOPATH)/bin/ilab 설치
```

또는 직접:

```bash
cd ilab
go build -o ../bin/ilab .
```

---

## 명령어

### `ilab doctor`

현재 호스트의 사전 요구사항과 환경 상태를 진단한다.

```
✓ infra-lab root: /path/to/infra-lab

Prerequisites:
  ✓ git         /usr/bin/git         all — root detection, build metadata
  ✓ tofu        /usr/local/bin/tofu  all — OpenTofu (>= 1.6)
  ✓ kubectl     /usr/bin/kubectl     cluster access, addon management
  ...

Managed environments (state/):
  ENV            BACKEND  CNI      CREATED
  my-env         libvirt  flannel  2026-01-01T00:00:00Z

VMs (all backends):
  NAME          MANAGED  ENV     STATE  IPv4
  lab-master-0  yes      my-env  실행 중  192.168.122.10
```

### `ilab env list`

`state/` 디렉토리에서 관리 환경 목록을 출력한다.

```
ENV      BACKEND  CNI      CREATED               COMMIT
my-env   libvirt  flannel  2026-01-01T00:00:00Z  abc1234
```

### `ilab env status [env]`

환경의 클러스터 노드 상태를 보여준다. 환경 이름을 생략하면 전체 환경을 출력한다.

### `ilab vm list [--all]`

관리 VM을 출력한다. `--all` 플래그를 사용하면 비관리 VM(다른 도구로 생성된 VM 등)도 포함한다.

### `ilab vm ssh <vm-name>`

VM에 대화형 셸로 접속한다.
- multipass 백엔드: `multipass shell <vm-name>`
- libvirt 백엔드: `ssh ubuntu@<ip>`

### `ilab vm version <vm-name>`

VM의 `/etc/infra-lab/build.json`을 읽어 생성 당시의 버전 정보를 출력한다.

```
Node:              lab-master-0
Role:              control-plane
Kubernetes:        v1.32.5
CNI:               flannel
Backend:           libvirt
Env:               my-env
infra-lab branch:  main
infra-lab commit:  abc1234...
Created:           2026-01-01T00:00:00Z
```

### `ilab k8s status [env]`

`kubectl get nodes -o wide` 및 `kubectl get pods -A -o wide`를 실행한다.

---

## 테스트

```bash
cd ilab
go test ./...
```

단위 테스트는 실제 tofu 상태나 virsh, multipass 없이 임시 디렉토리를 사용해 동작한다.
