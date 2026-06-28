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
