# Operations

## Purpose

This document fixes the standard entrypoints and commands for operating `infra-lab`.

Core rules:

- Always manage lab infrastructure through `infra-lab/scripts/k8s-tool.sh`.
- The default lab host is `seoy@100.123.80.48`.
- Do not run VM lifecycle commands directly on the local workstation `100.92.45.46`.
- Higher-level repos such as JUMI, AH, and `kube-slint` should rely on this repo for cluster bring-up and status checks.

## Standard host profile

Default profile:

- [hosts/remote-lab.env](/opt/go/src/github.com/HeaInSeo/infra-lab/hosts/remote-lab.env:1)

This profile pins:

- `LAB_HOST_MODE=remote`
- `LAB_REMOTE_SSH_TARGET=seoy@100.123.80.48`
- `LAB_REMOTE_REPO_PATH=/opt/go/src/github.com/HeaInSeo/infra-lab`
- `BACKEND=multipass`

Override `BACKEND=libvirt` at invocation time when using the libvirt path.

## Standard commands

Check remote lab status:

```bash
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh status
```

Verify remote lab host prerequisites:

```bash
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh host-setup
HOST_PROFILE=hosts/remote-lab.env BACKEND=libvirt ./scripts/k8s-tool.sh host-setup
```

Bring up the remote Multipass lab:

```bash
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh up
```

Bring up the remote libvirt lab:

```bash
HOST_PROFILE=hosts/remote-lab.env BACKEND=libvirt ./scripts/k8s-tool.sh up
```

Tear down the remote lab:

```bash
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh down
HOST_PROFILE=hosts/remote-lab.env BACKEND=libvirt ./scripts/k8s-tool.sh down
```

Verify addons:

```bash
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh addons-verify
HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh addons-verify optional metallb
```

## Boundary with higher-level repos

Higher-level repos such as JUMI, AH, and `kube-slint` should treat this document as the operating baseline.

Standard flow:

1. Prepare the cluster from `infra-lab` with a host profile.
2. Confirm nodes and system pods from `infra-lab` with `status`.
3. Run the higher-level repo workflow after the lab is healthy.

Avoid:

- calling `multipass`, `virsh`, or `tofu apply` directly from higher-level repos
- running VM lifecycle actions directly on the local workstation
- continuing to treat `multipass-k8s-lab` as the primary path name

## Operating rules

- The canonical repo path is `/opt/go/src/github.com/HeaInSeo/infra-lab`.
- The canonical branch is `main`.
- Prefer host-profile-driven `k8s-tool.sh` commands for regression checks and lab lifecycle work.
- Keep smoke-test findings and failure history in `docs/TROUBLESHOOTING_HISTORY.md`.
