# infra-lab Project Guide

## 1. What this project is

`infra-lab` is a shared VM-based Kubernetes lab infrastructure repository. It is not an application repository for one specific workload.

Its job is to provide:

- a repeatable VM-based Kubernetes lab
- a kubeadm-based baseline
- a small set of operational entrypoints
- a place where multiple higher-level repos can reuse the same lab platform

In other words, this repo is the lab platform, not the application that runs on top of it.

## 2. Why it exists separately

The separation is mainly about responsibility boundaries.

This repo owns:

- VM lifecycle
- kubeadm bootstrap
- kubeconfig export
- host profiles
- add-on install / verify flows
- lab operating conventions

This repo does not own:

- project-specific application deployment
- product business logic
- the Cilium implementation itself
- every operational detail of each workload

Higher-level PoC or application repos should consume this lab, not re-embed the lab lifecycle into themselves.

## 3. Current architecture

The current model can be understood as four layers:

1. control host
2. lab host
3. VM backend
4. kubeadm cluster

### Control host

This is where the operator starts commands.

- It may be the local workstation.
- It may also proxy commands to a remote host over SSH.

### Lab host

This is the machine that actually owns the VMs, kubeconfig, host bridge, and the path into the lab network.

Current default operating host:

- `100.123.80.48`

### VM backend

Currently supported backends:

- `multipass`
- `libvirt`

The backend decides how VMs are created and where state lives.

### kubeadm cluster

The cluster baseline is a small kubeadm-based lab cluster.

Current representative shape:

- `1 control-plane + 2 workers`

## 4. Standard entrypoint

The standard operational entrypoint is [scripts/k8s-tool.sh](/opt/go/src/github.com/HeaInSeo/infra-lab/scripts/k8s-tool.sh:1).

Why this matters:

- it hides local versus remote host mode
- it hides backend-specific state locations
- it reduces direct use of `multipass`, `virsh`, and `tofu`
- it centralizes the operational workflow

Representative commands:

- `./scripts/k8s-tool.sh up`
- `./scripts/k8s-tool.sh down`
- `./scripts/k8s-tool.sh status`
- `./scripts/k8s-tool.sh addons-verify`

Operational guide:

- [docs/OPERATIONS.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/OPERATIONS.md:1)

## 5. Host profiles

A host profile fixes where and how the lab is operated.

Representative file:

- [hosts/remote-lab.env](/opt/go/src/github.com/HeaInSeo/infra-lab/hosts/remote-lab.env:1)

It pins:

- `LAB_HOST_MODE=remote`
- `LAB_REMOTE_SSH_TARGET=seoy@100.123.80.48`
- `LAB_REMOTE_REPO_PATH=/opt/go/src/github.com/HeaInSeo/infra-lab`
- `BACKEND=multipass`

This lets operators use the same entrypoint without rethinking the remote execution path every time.

## 6. Add-on model

Add-ons are split into `base` and `optional`.

### Base

Base add-ons are useful for most lab clusters and have low coupling.

Current representative base add-on:

- `metrics-server`

### Optional

Optional add-ons are important for some lab shapes, but should not be forced into every baseline.

Examples:

- `local-path-storage`
- `metallb`
- `cilium`

Optional does not mean unimportant. It means environment-dependent.

## 7. Where Cilium fits

Cilium is not treated as a mandatory feature of every baseline cluster in this repo.

Current position:

- a generic addon path exists
- a real operating baseline exists on `remote-seoy`
- that operating baseline is documented as a separate profile

Core references:

- [docs/CILIUM.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/CILIUM.ko.md:1)
- [docs/CILIUM_REMOTE_SEOY.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/CILIUM_REMOTE_SEOY.ko.md:1)
- [profiles/remote-seoy/cilium/README.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/README.ko.md:1)

## 8. Why profiles are needed

The key problem we surfaced is that the generic example path and the real operating state are not the same.

For example:

- the generic addon values use `ipam.mode: kubernetes`
- the `remote-seoy` live cluster uses `cluster-pool + vxlan`

Trying to force both into one shared path would be risky.

Profiles exist to:

- pin the real baseline of a specific environment
- separate generic examples from live operating state
- separate desired baseline from experimental work
- avoid coupling higher-level apps to one lab-specific network implementation

## 9. Product and cloud rule

The current `remote-seoy` Cilium combination fits a lab / on-prem-like environment:

- `cluster-pool`
- `vxlan`
- `LB IPAM`
- `L2 announcement`
- `Gateway API`

But that must not become an application assumption.

App core should depend only on portable Kubernetes abstractions:

- `Service DNS`
- `Service`
- `Gateway API`

An app should not need to know whether the platform uses Cilium cluster-pool, MetalLB, or a cloud load balancer.

## 10. Recommended reading order

For someone new to the repo, this order is the most useful:

1. [README.en.md](/opt/go/src/github.com/HeaInSeo/infra-lab/README.en.md:1)
2. [docs/LAB_SCOPE.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/LAB_SCOPE.md:1)
3. [docs/ARCHITECTURE.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/ARCHITECTURE.md:1)
4. [docs/OPERATIONS.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/OPERATIONS.md:1)
5. [docs/CILIUM.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/CILIUM.ko.md:1)
6. [docs/CILIUM_REMOTE_SEOY.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/CILIUM_REMOTE_SEOY.md:1)

## 11. The most important current decision

The most important current decision is this:

- the goal is not to add Cilium service mesh features
- the goal is to align and safely document the already-running `Gateway-only baseline`

If that boundary is blurred:

- live drift cleanup gets mixed with feature expansion
- risky IPAM changes can masquerade as baseline work
- workload-specific routes can leak into the shared infra baseline

So the correct approach remains small, conservative, and reproducible.
