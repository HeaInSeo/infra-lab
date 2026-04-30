# infra-lab

English: [README.en.md](README.en.md)
한국어: [README.md](README.md)

`infra-lab` is a reusable VM-based Kubernetes lab baseline for local and workstation-grade PoC work. The current baseline stays intentionally narrow: OpenTofu + kubeadm, with the VM backend expanding toward both Multipass and libvirt.

This repository is not a single-project environment. It is a shared lab infrastructure base for future Kubernetes experiments such as node-local artifact and storage flows, DaemonSet-style node agents, same-node reuse versus cross-node fetch behavior, Cilium and networking work, storage tests, operator validation, and other cluster-level PoCs.

## Identity

- Purpose: general-purpose K8s VM lab infrastructure
- Current baseline: Ubuntu 24.04 guests + OpenTofu + kubeadm
- First target shape: 3 VMs, `1 control-plane + 2 workers`
- Lifecycle support: host setup, cluster up/down, status, local clean
- Backend model: `multipass`, `libvirt`
- Add-on model: `base` and `optional`
- Future extension point: `profiles/` for project-specific overlays without turning this repo into a workload repo

## What This Repo Owns

- VM lifecycle for a kubeadm-based lab cluster through the selected backend
- Baseline cluster bootstrap and kubeconfig export
- Small operational scripts for repeatable local lab usage
- A minimal add-on layer for cluster infrastructure capabilities
- Documentation for scope, entrypoints, and baseline operating model

## What This Repo Does Not Own

- Application deployment for a specific project
- Artifact-agent, catalog, storage app, or Cilium implementation itself
- Production cluster provisioning or production hardening
- Deeply embedded project-specific workloads in the main repo

More detail: [docs/LAB_SCOPE.md](docs/LAB_SCOPE.md)

The standard operating commands and host-profile baseline are documented in [docs/OPERATIONS.md](docs/OPERATIONS.md).

## Quick Start

1. Prepare the host:

```bash
./scripts/k8s-tool.sh host-setup
```

To run against a remote lab host, point the entry script at the default host profile:

```bash
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh status
```

The default operating profile is already provided as [hosts/remote-lab.env](hosts/remote-lab.env). If you need another environment, copy [hosts/remote-lab.env.example](hosts/remote-lab.env.example) into a separate profile file and adjust it.

2. Review defaults:

```bash
sed -n '1,200p' dev.auto.tfvars
```

3. Choose a backend and create the baseline cluster:

```bash
./scripts/k8s-tool.sh up
BACKEND=libvirt ./scripts/k8s-tool.sh up
```

If you change `name_prefix`, `masters`, `workers`, `vm_user`, or VM sizing in `dev.auto.tfvars`, rerunning `up` reapplies the OpenTofu plan against that updated shape.
The libvirt backend also requires variables such as `TF_VAR_ssh_private_key_path` and `TF_VAR_ssh_public_key`.
When the libvirt backend uses `qemu:///system`, the command may need `sudo` unless the host user already has libvirt management privileges.

4. Check status:

```bash
./scripts/k8s-tool.sh status
```

5. Verify the base add-ons:

```bash
./scripts/k8s-tool.sh addons-verify
```

`up` now installs the base add-on `metrics-server` by default. `addons-verify` now expects the base add-ons to actually exist. To target one optional add-on explicitly, use a scoped command such as `./scripts/k8s-tool.sh addons-verify optional cilium`.

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

Host setup behavior depends on the backend:

- `BACKEND=multipass`: prepares the Multipass-oriented path, including OpenTofu and Python prerequisites
- `BACKEND=libvirt`: verifies libvirt / virsh / qemu-img availability

The current host setup helpers are still oriented toward Rocky/RHEL-family hosts.

### Cluster up

```bash
./scripts/k8s-tool.sh up
```

What happens:

- OpenTofu initializes and applies resources from the selected backend state directory
- The selected backend launches Ubuntu 24.04 guests
- `kubeadm init` runs on the first control-plane node
- worker nodes join
- the flow waits until all joined nodes report `Ready`
- kubeconfig is exported on the execution host

### Status

```bash
./scripts/k8s-tool.sh status
```

If `./kubeconfig` exists and `kubectl` is present, node and pod status is shown. Otherwise the command falls back to `multipass list` or `virsh list --all`, depending on the backend.

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

Base add-ons are reasonable defaults for a lab cluster and do not make the repository specific to one PoC. They are now treated as part of the default baseline installed after `up`.

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
./scripts/k8s-tool.sh addons-verify optional metallb
```

Use `addons-install base` to repair or reapply the default base add-ons if needed.

`metallb` requires review of [addons/values/metallb/ipaddresspool.yaml](addons/values/metallb/ipaddresspool.yaml) before use.

## Directory Layout

```text
.
├── addons/
│   ├── base/
│   ├── optional/
│   ├── values/
│   └── manage.sh
├── backends/
│   └── libvirt/
├── cloud-init/
├── docs/
├── hosts/
├── profiles/
├── scripts/
│   ├── runtime/
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
- The libvirt backend relies on DHCP leases from the `default` libvirt network and does not hardcode a guest interface name.
- As of April 30, 2026, the libvirt backend has been live-tested on `100.123.80.48` with `1 control-plane + 2 workers`.
- Host helper scripts are still oriented around the existing Rocky 8 workstation setup flow.
- The host / backend / transport split is documented in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
- The earlier Rocky 8 based path may also hit the same class of control-plane instability seen during this sprint. If static pods churn or the API server becomes unstable, check runtime alignment first: `containerd --version`, `systemctl show -p ExecStart containerd`, `/etc/containerd/config.toml`, `crictl info`, and kubelet `cgroupDriver`, before changing `etcd` or `kube-apiserver` manifests.
- A troubleshooting timeline based on the actual incident history is available at [docs/TROUBLESHOOTING_HISTORY.md](docs/TROUBLESHOOTING_HISTORY.md).
