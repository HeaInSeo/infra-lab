# multipass-k8s-lab

English: [README.en.md](/opt/go/src/github.com/HeaInSeo/multipass-k8s-lab/README.en.md)
한국어: [README.md](/opt/go/src/github.com/HeaInSeo/multipass-k8s-lab/README.md)

`multipass-k8s-lab` is a reusable VM-based Kubernetes lab baseline for local and workstation-grade PoC work. The current baseline is intentionally narrow: Multipass + Ubuntu 24.04 guests + OpenTofu + kubeadm, with a repeatable 3-node cluster flow and a small set of infrastructure add-ons.

This repository is not a single-project environment. It is a shared lab infrastructure base for future Kubernetes experiments such as node-local artifact and storage flows, DaemonSet-style node agents, same-node reuse versus cross-node fetch behavior, Cilium and networking work, storage tests, operator validation, and other cluster-level PoCs.

## Identity

- Purpose: general-purpose K8s VM lab infrastructure
- Current baseline: Ubuntu 24.04 guests + Multipass + OpenTofu + kubeadm
- First target shape: 3 VMs, `1 control-plane + 2 workers`
- Lifecycle support: host setup, cluster up/down, status, local clean
- Add-on model: `base` and `optional`
- Future extension point: `profiles/` for project-specific overlays without turning this repo into a workload repo

## What This Repo Owns

- Multipass VM lifecycle for a kubeadm-based lab cluster
- Baseline cluster bootstrap and kubeconfig export
- Small operational scripts for repeatable local lab usage
- A minimal add-on layer for cluster infrastructure capabilities
- Documentation for scope, entrypoints, and baseline operating model

## What This Repo Does Not Own

- Application deployment for a specific project
- Artifact-agent, catalog, storage app, or Cilium implementation itself
- Production cluster provisioning or production hardening
- Deeply embedded project-specific workloads in the main repo

More detail: [docs/LAB_SCOPE.md](/opt/go/src/github.com/HeaInSeo/multipass-k8s-lab/docs/LAB_SCOPE.md)

## Quick Start

1. Prepare the host:

```bash
./scripts/k8s-tool.sh host-setup
```

2. Review defaults:

```bash
sed -n '1,200p' dev.auto.tfvars
```

3. Create the baseline cluster:

```bash
./scripts/k8s-tool.sh up
```

4. Check status:

```bash
./scripts/k8s-tool.sh status
```

5. Install base add-ons:

```bash
./scripts/k8s-tool.sh addons-install base
./scripts/k8s-tool.sh addons-verify
```

6. Tear down:

```bash
./scripts/k8s-tool.sh down
```

7. Remove local state:

```bash
FORCE=1 ./scripts/k8s-tool.sh clean
```

## Baseline Execution Path

### Host setup

```bash
./scripts/k8s-tool.sh host-setup
```

Installs or verifies:

- OpenTofu
- Multipass
- Python 3
- optional `kubectl`
- optional `helm`

### Cluster up

```bash
./scripts/k8s-tool.sh up
```

What happens:

- OpenTofu initializes and applies local resources
- Multipass launches Ubuntu 24.04 VMs
- `kubeadm init` runs on the first control-plane node
- worker nodes join
- local `./kubeconfig` is exported

### Status

```bash
./scripts/k8s-tool.sh status
```

If `./kubeconfig` exists and `kubectl` is present, node and pod status is shown. Otherwise the command falls back to `multipass list`.

### Down

```bash
./scripts/k8s-tool.sh down
```

Destroys VMs and related local OpenTofu-managed resources.

### Clean

```bash
FORCE=1 ./scripts/k8s-tool.sh clean
```

Removes local state files such as `.terraform/`, `*.tfstate`, and `./kubeconfig`. It does not remove host packages.

## Add-ons

The repo separates infrastructure add-ons into two categories.

### Base

Base add-ons are reasonable defaults for a lab cluster and do not make the repository specific to one PoC.

- `metrics-server`

### Optional

Optional add-ons are useful for certain lab shapes, but should be explicit rather than always on.

- `local-path-storage`
- `metallb`

Examples:

```bash
./scripts/k8s-tool.sh addons-install base
./scripts/k8s-tool.sh addons-install optional local-path-storage
./scripts/k8s-tool.sh addons-install optional metallb
./scripts/k8s-tool.sh addons-verify
```

`metallb` requires review of [addons/values/metallb/ipaddresspool.yaml](/opt/go/src/github.com/HeaInSeo/multipass-k8s-lab/addons/values/metallb/ipaddresspool.yaml) before use.

## Directory Layout

```text
.
├── addons/
│   ├── base/
│   ├── optional/
│   ├── values/
│   └── manage.sh
├── cloud-init/
├── docs/
├── profiles/
├── scripts/
│   ├── host/
│   ├── multipass/
│   ├── cluster/
│   └── k8s-tool.sh
├── main.tf
├── variables.tf
├── versions.tf
└── dev.auto.tfvars
```

## Why This Looks Different From `mac-k8s-multipass-terraform`

This repo references the earlier project but does not inherit its service-oriented scope. The old repository mixed cluster baseline work with service install traces and broader stack add-ons. This repository keeps the VM and kubeadm lifecycle pattern, then narrows the responsibility back to reusable lab infrastructure.

## Future Direction

- Cilium as an alternative networking profile
- local storage and local PV experiment helpers
- node-local agent experiment helpers
- operator validation profiles
- project overlays under `profiles/` or a future `labs/` convention

## Notes

- The default CNI in the first baseline is Flannel for simplicity and quick bootstrap.
- Cilium is intentionally deferred as a future profile or optional path, not forced into the baseline.
- The guest baseline uses Ubuntu 24.04 because the public Multipass image catalog is Ubuntu-first in the current environment.
- Host helper scripts are still oriented around the existing Rocky 8 workstation setup flow.
- The earlier Rocky 8 based path may also hit the same class of control-plane instability seen during this sprint. If static pods churn or the API server becomes unstable, check runtime alignment first: `containerd --version`, `systemctl show -p ExecStart containerd`, `/etc/containerd/config.toml`, `crictl info`, and kubelet `cgroupDriver`, before changing `etcd` or `kube-apiserver` manifests.
