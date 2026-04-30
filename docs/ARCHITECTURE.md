# Architecture

## Goal

`infra-lab` is moving from a single-host, single-runtime lab into a reusable baseline that can run on multiple hosts and more than one VM runtime.

The current implementation intentionally stops at two abstraction steps:

1. host independence
2. VM runtime independence

It does not try to become a fully generic IaC framework.

## Layers

### Host model

The control host and the lab host may be different machines.

- `LAB_HOST_MODE=local`
  Runs OpenTofu and helper scripts on the current machine.
- `LAB_HOST_MODE=remote`
  Runs the same `scripts/k8s-tool.sh` commands over SSH on a remote checkout.

Relevant settings:

- `HOST_PROFILE`
- `LAB_REMOTE_SSH_TARGET`
- `LAB_REMOTE_REPO_PATH`

### Backend model

The selected backend determines where OpenTofu state lives and which VM runtime is used.

- `BACKEND=multipass`
  Uses the repository root state and the existing Multipass workflow.
- `BACKEND=libvirt`
  Uses `backends/libvirt/` state and the libvirt provider workflow.

### Transport model

Cluster bootstrap scripts should not directly depend on one VM runtime.

The runtime transport layer lives under `scripts/runtime/`:

- `VM_RUNTIME=multipass`
- `VM_RUNTIME=ssh`

Current usage:

- Multipass backend uses `multipass` transport for bootstrap/join/export.
- libvirt backend uses SSH transport after DHCP leases become available.

## Directory model

```text
.
├── backends/
│   └── libvirt/
├── hosts/
├── scripts/
│   ├── runtime/
│   ├── multipass/
│   ├── cluster/
│   └── host/
└── docs/
```

## Supported combinations

### Multipass backend

- host mode: local or remote
- runtime: Multipass
- state path: repository root

### libvirt backend

- host mode: local or remote
- runtime: libvirt provider + SSH bootstrap
- state path: `backends/libvirt/`
- privilege model: often `sudo` with `qemu:///system`, unless libvirt ACLs are already delegated to the host user
- readiness model: waits for a DHCP lease first, then waits until all joined nodes report `Ready`

## Known gaps

- helper scripts like `scripts/cluster/flannel-to-cilium.sh` are still multipass-oriented
- backend-specific variables are not fully normalized yet
- kubeconfig stays on the execution host unless copied separately
