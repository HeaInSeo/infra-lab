# ilab Command Data Schema

이 문서는 `ilab --json` envelope의 `data` 필드를 command별로 요약한다.

## version

```json
{
  "infraLabVersion": "dev",
  "gitCommit": "unknown",
  "buildDate": "unknown"
}
```

## capabilities

```json
{
  "infraLabVersion": "dev",
  "contractVersion": "infra-lab.contract/v1",
  "capabilities": ["version.v1"]
}
```

## doctor

```json
{
  "root": "/path/to/infra-lab",
  "prerequisites": [],
  "envs": [],
  "legacyFiles": [],
  "vms": [],
  "health": {"risk": "LOW", "summary": "Required tools are installed"},
  "findings": []
}
```

## env.list

```json
{
  "envs": [
    {
      "name": "libvirt-cilium",
      "source": "state",
      "stateDir": "state/libvirt-cilium",
      "backend": "libvirt",
      "cni": "cilium",
      "status": "present"
    }
  ]
}
```

## env.status

Single env:

```json
{
  "env": "libvirt-cilium",
  "stateDir": "state/libvirt-cilium",
  "profile": {"name": "libvirt-cilium", "path": "state/libvirt-cilium/resolved-profile.yaml"},
  "backend": "libvirt",
  "cni": "cilium",
  "status": "ok",
  "conditions": []
}
```

All envs:

```json
{
  "envs": []
}
```

## profile.list

```json
{
  "profiles": [
    {"name": "multipass-flannel", "source": "repo", "path": "envs/multipass-flannel.yaml"}
  ]
}
```

## profile.show

```json
{
  "profile": {"name": "multipass-flannel", "source": "repo", "path": "envs/multipass-flannel.yaml.example"},
  "spec": {
    "backend": "multipass",
    "cni": "flannel",
    "masters": 1,
    "workers": 2,
    "osImage": "ubuntu-24.04",
    "stateDir": "state/multipass-flannel"
  }
}
```

## profile.validate

Success:

```json
{
  "profile": {"name": "multipass-flannel", "source": "repo", "path": "envs/multipass-flannel.yaml.example"},
  "valid": true,
  "normalized": {
    "backend": "multipass",
    "cni": "flannel",
    "masters": 1,
    "workers": 2,
    "osImage": "ubuntu-24.04",
    "stateDir": "state/multipass-flannel"
  },
  "conditions": []
}
```

Failure uses `ok:false`, `data:null`, and `errors[]`.

## k8s.status

```json
{
  "env": "libvirt-cilium",
  "kubeconfig": "state/libvirt-cilium/kubeconfig",
  "cluster": {"reachable": true, "nodesReady": 3, "podsNotReady": []},
  "nodes": [],
  "pods": [],
  "health": {"risk": "LOW", "summary": "Cluster is reachable"},
  "findings": []
}
```

## vm.list

```json
{
  "vms": [
    {"name": "lab-master-0", "managed": true, "env": "libvirt-cilium", "backend": "libvirt", "state": "running", "ipv4": "192.168.122.10"}
  ]
}
```

## capability.list

```json
{
  "capabilities": [
    {
      "name": "buildah-userns",
      "consumers": ["nodevault"],
      "summary": "in-Pod Buildah build under a Kubernetes User Namespace (pod.spec.hostUsers: false)"
    }
  ]
}
```

## capability.check

`checks[]`의 `static` stage 4개(K8S_VERSION/NODE_RUNTIME/HOST_USERS_API/STORAGECLASS)와 `smoke` stage
2개(USERNS_SMOKE_TEST/BUILDAH_SMOKE_TEST)가 모두 구현되어 있다. `status`는 이 6개 체크로부터 파생된다:
하나라도 `FAIL`이면 `UNSUPPORTED`, `UNKNOWN`/`SKIPPED`가 있으면(스모크 테스트 전제조건 미충족 포함)
`NOT_VERIFIED`, 전부 `PASS`면 `SUPPORTED`. `BUILDAH_SMOKE_TEST`는 `USERNS_SMOKE_TEST`가 `PASS`하지
못하면 실행하지 않고 `SKIPPED`로 표기한다. `health.risk`는 기존 doctor.v1/k8s.status.v1과 같은
LOW/MEDIUM/HIGH 어휘를 유지한다.

스모크 테스트는 전용 임시 namespace(`ilab-capability-check-<kind>-<타임스탬프>`)에 Pod를 만들어
실행하고, 결과와 무관하게 항상 정리한다(delete는 `--wait=false`로 비동기 실행 — 확인 직후에는
`Terminating` 상태로 잠깐 보일 수 있음).

아래는 seoy-libvirt-cilium 클러스터(kernel 6.8, containerd 2.2.1, crun 1.28)에서 실제로 관찰된 결과다.
정적 체크와 `USERNS_SMOKE_TEST`는 전부 통과했지만, `BUILDAH_SMOKE_TEST`는 커널이 강제하는
mount-namespace 소유권 제약(Buildah가 자신의 mount namespace를 private으로 바꾸려는 시도가
`hostUsers:false`의 root filesystem에서는 거부됨) 때문에 실패했다 — CAP_SYS_ADMIN과 unconfined
seccomp profile을 둘 다 시도해봤지만 결과가 바뀌지 않아, 단순 RBAC/권한 문제가 아니라 이 스택(K8s
User Namespaces + Buildah 기본 저장 경로) 자체의 근본적인 비호환으로 보인다는 점을 `detail`에 그대로
남긴다.

```json
{
  "capability": "buildah-userns",
  "consumers": ["nodevault"],
  "env": "seoy-libvirt-cilium",
  "kubeconfig": "state/seoy-libvirt-cilium/kubeconfig",
  "status": "UNSUPPORTED",
  "health": {"risk": "HIGH", "summary": "one or more required checks failed"},
  "cluster": {"reachable": true, "serverVersion": "v1.36.2", "hostUsersFieldSupported": true},
  "nodes": [
    {
      "name": "lab-worker-0",
      "kernelVersion": "6.8.0-134-generic",
      "containerRuntimeVersion": "containerd://2.2.1",
      "ociRuntime": "crun",
      "ociRuntimeVersion": "1.28"
    }
  ],
  "storage": {"defaultStorageClass": "local-path", "storageClasses": ["local-path"]},
  "checks": [
    {"name": "K8S_VERSION", "stage": "static", "result": "PASS", "detail": "server v1.36.2 >= v1.30 (hostUsers baseline)"},
    {"name": "NODE_RUNTIME", "stage": "static", "result": "PASS", "detail": "all nodes report containerd >= 2.0 with crun as the OCI runtime"},
    {"name": "HOST_USERS_API", "stage": "static", "result": "PASS", "detail": "pod.spec.hostUsers is present in the API schema"},
    {"name": "STORAGECLASS", "stage": "static", "result": "PASS", "detail": "default StorageClass \"local-path\" present"},
    {"name": "USERNS_SMOKE_TEST", "stage": "smoke", "result": "PASS", "detail": "disposable hostUsers:false Pod ran, wrote to emptyDir, and exited 0 (id: uid=0(root) gid=0(root) groups=0(root),10(wheel))"},
    {"name": "BUILDAH_SMOKE_TEST", "stage": "smoke", "result": "FAIL", "detail": "in-Pod Buildah cannot privatize its own mount namespace under hostUsers:false — ... (see repo for full text) ..."}
  ],
  "findings": [
    {"code": "BUILDAH_USERNS_MOUNT_INCOMPATIBLE", "message": "in-Pod Buildah cannot privatize its own mount namespace under hostUsers:false on this cluster — see BUILDAH_SMOKE_TEST detail for the full explanation"}
  ]
}
```

## vm.version

```json
{
  "vm": "lab-master-0",
  "build": {
    "schemaVersion": "infra-lab.build/v1",
    "infraLabGitCommit": "abc123",
    "envName": "libvirt-cilium"
  }
}
```
