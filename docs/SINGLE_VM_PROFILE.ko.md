# Single-VM Profile

`kind: single-vm`은 Kubernetes를 설치하지 않고 libvirt Ubuntu VM 1대만 만드는 profile 종류다.
기존 profile에서 `kind`를 생략하면 `kubernetes`로 처리되므로 기존 Kubernetes 환경 동작은 유지된다.

## 예시

```yaml
kind: single-vm
name: ebpf-dev
backend: libvirt

vm:
  osImage: ubuntu-24.04
  imageUrl: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
  count: 1
  cpu: 4
  memory: 8G
  disk: 80G

ssh:
  user: ubuntu
  privateKeyPath: ~/.ssh/id_ed25519

workspace:
  path: /home/ubuntu/workspace/ebpf-lab
  dirs:
    - c-libbpf
    - rust-aya
    - notes
    - scripts

bootstrap:
  scripts:
    - lab/infra-lab/bootstrap/install-core.sh
    - lab/infra-lab/bootstrap/install-rust-aya.sh
    - lab/infra-lab/bootstrap/verify-ebpf.sh

libvirt:
  poolName: lab-pool
  poolPath: /var/lib/libvirt/images

state:
  dir: state/ebpf-dev
```

## 명령

```bash
ilab profile validate /path/to/ebpf-dev.yaml
ilab env up /path/to/ebpf-dev.yaml
ilab env ssh ebpf-dev
ilab env info ebpf-dev --json
```

`env up`은 OpenTofu(`tofu`)로 `backends/single-vm` 모듈을 적용한다. VM이 SSH 가능해질 때까지 기다린 뒤 `bootstrap.scripts`를 workspace의 `scripts/` 디렉터리에 복사하고 실행 권한을 부여한다.

bootstrap script는 자동 실행하지 않는다. eBPF/Rust 설치는 네트워크와 시간이 많이 걸리므로 사용자가 VM에 접속한 뒤 필요한 순서로 직접 실행한다.

## JSON 정보

`ilab env info ebpf-dev --json`은 SSH private key 내용을 출력하지 않고 path만 출력한다.

```json
{
  "ssh": {
    "host": "192.0.2.10",
    "user": "ubuntu",
    "port": 22,
    "identityFile": "~/.ssh/id_ed25519"
  },
  "workspace": {
    "path": "/home/ubuntu/workspace/ebpf-lab"
  }
}
```
