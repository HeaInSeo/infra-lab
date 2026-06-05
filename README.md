# infra-lab

Korean: [README.ko.md](README.ko.md)

A reusable VM-based Kubernetes lab for local and workstation-grade work.
Manages cluster lifecycle across `multipass` and `libvirt` backends using OpenTofu + kubeadm.
Includes `ilab` — a read-only CLI for inspecting environments, VMs, and cluster state.

[![CI](https://github.com/HeaInSeo/infra-lab/actions/workflows/check.yml/badge.svg)](https://github.com/HeaInSeo/infra-lab/actions/workflows/check.yml)

## What this repo owns

- VM lifecycle for a kubeadm cluster through the selected backend
- Cluster bootstrap, kubeconfig export, and base addon installation
- Per-environment state isolation under `state/<env>/`
- `ilab` CLI for environment inspection
- CI quality gates: shell, HCL, YAML, Go

## What this repo does not own

- Application or workload deployment
- Production hardening or production cluster provisioning
- Deep project-specific configuration embedded in the main repo

## Prerequisites

Run `ilab doctor` at any time to verify the current state of your host.

### Common (all paths)

| Tool | Min version | Verify | Installed by |
|------|-------------|--------|--------------|
| bash | 4.0 | `bash --version` | OS |
| git | any | `git --version` | OS / manual |
| tofu | 1.6 | `tofu version` | `make host-setup` (Rocky) |
| kubectl | 1.28+ | `kubectl version --client` | `make host-setup` (Rocky) |
| python3 | 3.6+ | `python3 --version` | `make host-setup` (Rocky) |

### multipass backend

| Tool | Verify | Installed by |
|------|--------|--------------|
| multipass | `multipass version` | `./scripts/k8s-tool.sh host-setup` |
| jq | `jq --version` | manual (optional — used for state reconciliation) |

### libvirt backend

| Tool | Verify | Notes |
|------|--------|-------|
| virsh | `virsh --version` | `dnf install libvirt-client` |
| qemu-img | `qemu-img --version` | `dnf install qemu-img` |
| SSH key pair | `ls ~/.ssh/id_*.pub` | set `TF_VAR_ssh_private_key_path` in env profile |

### Cilium addon

| Tool | Verify | Installed by |
|------|--------|--------------|
| helm | `helm version` | `./scripts/k8s-tool.sh host-setup` |

### ilab CLI build

| Tool | Min version | Verify |
|------|-------------|--------|
| go | 1.22 | `go version` |

> **host-setup shortcut (Rocky/RHEL):** `HOST_PROFILE=envs/<name>.env ./scripts/k8s-tool.sh host-setup`
> installs tofu, multipass, kubectl, helm, and python3 via dnf/snap.
> Manual install is needed for libvirt tools and Go.

## Quick start

### 1. Pick an environment profile

```bash
cp envs/multipass-flannel.env.example envs/multipass-flannel.env
# For libvirt, fill in TF_VAR_ssh_private_key_path and TF_VAR_ssh_public_key
cp envs/libvirt-cilium.env.example envs/libvirt-cilium.env
```

Profiles live in `envs/` and set `BACKEND`, `CNI`, `ADDONS`, and any backend-specific `TF_VAR_*` values.
They are gitignored — only `.env.example` files are committed.

### 2. Prepare the host

```bash
HOST_PROFILE=envs/multipass-flannel.env ./scripts/k8s-tool.sh host-setup
```

### 3. Create the cluster

```bash
ENV_PROFILE=envs/multipass-flannel.env make env-up
# or directly:
HOST_PROFILE=envs/multipass-flannel.env ./scripts/k8s-tool.sh up
```

State is isolated under `state/multipass-flannel/`:
```
state/multipass-flannel/
  terraform.tfstate   ← OpenTofu state
  kubeconfig          ← cluster access
  meta                ← creation metadata (git commit, backend, CNI, timestamp)
```

### 4. Check status

```bash
ENV_PROFILE=envs/multipass-flannel.env make env-status
# or via ilab:
ilab env status
ilab k8s status
```

### 5. Tear down

```bash
ENV_PROFILE=envs/multipass-flannel.env make env-down
FORCE=1 ENV_PROFILE=envs/multipass-flannel.env make env-clean
```

## Backends

| Backend | Notes |
|---------|-------|
| `multipass` | Recommended for initial setup. Requires Multipass on the host. |
| `libvirt` | Requires `virsh`, `qemu-img`, libvirt access, and SSH key variables. |

### Backend-specific variables

**multipass** — no extra variables required beyond the profile.

**libvirt** — set in the profile or environment:
```bash
TF_VAR_ssh_private_key_path=/home/you/.ssh/id_ed25519
TF_VAR_ssh_public_key=ssh-ed25519 AAAA...
```

## CNI

Set `CNI=` in the environment profile:

| Value | Behavior |
|-------|----------|
| `flannel` | Default. Installed during cluster bootstrap. |
| `cilium` | Flannel is installed first, then `flannel-to-cilium.sh` migration runs automatically after `up`. Only supported with `multipass` backend currently. |

## ilab CLI

A read-only operator interface that inspects infra-lab environments without managing state itself.

```bash
make build        # → bin/ilab
make install      # → $(go env GOPATH)/bin/ilab
```

Source of truth remains the OpenTofu state, VMs, and Kubernetes API.
`ilab` only reads.

### Commands

```bash
ilab doctor                     # diagnose current environment state
ilab env list                   # list environments from state/
ilab env status [env]           # show cluster/VM status
ilab vm list                    # list managed VMs
ilab vm list --all              # list all VMs including unmanaged
ilab vm version <vm>            # read /etc/infra-lab/build.json from VM
ilab vm ssh <vm>                # open interactive shell (multipass shell)
ilab k8s status [env]           # kubectl nodes + pods
```

`ilab` walks up from the current directory to find the repo root automatically.
Override with `INFRA_LAB_ROOT=/path/to/infra-lab`.

### VM build metadata

Each VM receives `/etc/infra-lab/build.json` after `env-up`:

```json
{
  "schemaVersion": "infra-lab.vm.v1",
  "infraLabGitCommit": "abc1234",
  "infraLabGitBranch": "main",
  "envName": "multipass-flannel",
  "backend": "multipass",
  "cni": "flannel",
  "role": "control-plane",
  "nodeName": "lab-master-0",
  "kubernetesVersion": "v1.32.5",
  "createdAt": "2026-06-05T00:00:00Z"
}
```

`ilab vm version <node>` reads this file to show which infra-lab version and configuration built the VM.

## Addons

Addons are separated into two categories.

### Base (auto-installed after `up`)

- `metrics-server` — installed with `--kubelet-insecure-tls` for this lab's certificate shape

### Optional (explicit install)

- `local-path-storage` — pinned to v0.0.28
- `metallb` — review `addons/values/metallb/ipaddresspool.yaml` before use
- `cilium` — install via the migration path (`CNI=cilium` in profile), not directly via addon install

```bash
./scripts/k8s-tool.sh addons-install optional local-path-storage
./scripts/k8s-tool.sh addons-verify optional local-path-storage
```

## Makefile reference

```bash
# Lint
make check          # shell + yaml + hcl (default)
make lint-shell     # bash -n + shellcheck
make lint-yaml      # YAML parse
make lint-hcl       # tofu fmt + tofu validate
make lint-go        # gofmt + go vet + go build
make test-go        # go test ./...

# Environment
ENV_PROFILE=envs/<name>.env make env-up
ENV_PROFILE=envs/<name>.env make env-down
ENV_PROFILE=envs/<name>.env make env-status
FORCE=1 ENV_PROFILE=envs/<name>.env make env-clean

# CLI
make build          # bin/ilab
make install        # GOPATH/bin/ilab
```

## Directory layout

```
.
├── addons/            # base/ and optional/ addon scripts
│   ├── base/
│   ├── optional/
│   └── values/
├── backends/
│   └── libvirt/       # libvirt-specific Terraform modules
├── bin/               # built binaries (gitignored)
├── cloud-init/        # VM bootstrap cloud-init config
├── docs/
├── envs/              # environment profiles (*.env.example committed, *.env gitignored)
├── ilab/              # Go CLI source (module: github.com/HeaInSeo/infra-lab/ilab)
├── profiles/          # project-specific overlays and baselines
├── scripts/
│   ├── cluster/       # cluster-init, join-all, flannel-to-cilium, write-build-json
│   ├── host/          # host setup and cleanup
│   ├── multipass/
│   ├── runtime/       # lib.sh, run-remote.sh
│   └── k8s-tool.sh   # main entrypoint
├── state/             # per-environment runtime state (gitignored)
│   └── <env>/
│       ├── terraform.tfstate
│       ├── kubeconfig
│       └── meta
├── main.tf
├── variables.tf
├── versions.tf
└── dev.auto.tfvars
```

## State isolation

When `HOST_PROFILE` or `ENV_NAME` is set, all per-environment files are isolated under `state/<env>/`.
Without a profile, state falls back to the backend directory (backward compatible with pre-Phase-2 environments).

```bash
# New: isolated under state/multipass-flannel/
HOST_PROFILE=envs/multipass-flannel.env ./scripts/k8s-tool.sh up

# Legacy: state in backend directory root (unchanged)
./scripts/k8s-tool.sh up
```

`ilab doctor` detects both states and explains which files exist and what to do next.

## Reproducibility

- Kubernetes packages pinned to `1.32.5` in `cloud-init/k8s.yaml`
- `local-path-storage` pinned to `v0.0.28`
- Provider versions locked in `.terraform.lock.hcl` (committed)
- Ubuntu 24.04 LTS guest image

## CI

Two GitHub Actions workflows run on every push and pull request:

| Workflow | Jobs |
|----------|------|
| `check` | Shell (shellcheck), OpenTofu (fmt + validate), YAML (parse), Go (gofmt + vet + build + test) |
| `workflow-lint` | actionlint, no `ubuntu-latest` |

Runners are pinned to `ubuntu-24.04`. Dependabot updates Actions versions monthly.
