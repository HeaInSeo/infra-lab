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

정적 discovery만 구현된 상태(issue #36 1단계)에서는 `checks[]`의 `smoke` stage 항목이 항상
`NOT_IMPLEMENTED`이므로 `status`는 `SUPPORTED`가 될 수 없다 — 정적 체크가 전부 통과해도 최대
`DEGRADED`까지만 나온다. `health.risk`는 기존 doctor.v1/k8s.status.v1과 같은 LOW/MEDIUM/HIGH
어휘를 유지하고, 판정 자체는 `status`(SUPPORTED/UNSUPPORTED/NOT_VERIFIED/DEGRADED)를 본다.

```json
{
  "capability": "buildah-userns",
  "consumers": ["nodevault"],
  "env": "libvirt-cilium",
  "kubeconfig": "state/libvirt-cilium/kubeconfig",
  "status": "DEGRADED",
  "health": {"risk": "MEDIUM", "summary": "static discovery passed; smoke tests are not yet implemented"},
  "cluster": {"reachable": true, "serverVersion": "v1.36.2", "hostUsersFieldSupported": true},
  "nodes": [
    {
      "name": "lab-worker-0",
      "kernelVersion": "6.8.0-31-generic",
      "containerRuntimeVersion": "containerd://2.0.1",
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
    {"name": "USERNS_SMOKE_TEST", "stage": "smoke", "result": "NOT_IMPLEMENTED", "detail": "disposable hostUsers:false Pod smoke test scheduled for a later iteration"},
    {"name": "BUILDAH_SMOKE_TEST", "stage": "smoke", "result": "NOT_IMPLEMENTED", "detail": "disposable Buildah build smoke test scheduled for a later iteration"}
  ],
  "findings": []
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
