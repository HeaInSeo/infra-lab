# TROUBLESHOOTING_HISTORY

This document records the actual problems encountered while bringing up `infra-lab`, organized as a learning-oriented troubleshooting history based on Git history and real incident flow.

Its core goals are:

- to diagnose repeated problems in a grounded order instead of by guesswork
- to preserve why the current baseline became `Ubuntu 24.04 + containerd + kubeadm`

## Summary

Three major problems were encountered in this phase:

1. the original guest baseline did not fit the current Multipass environment, so 3-node bring-up failed with the `rocky-8` image alias
2. after switching to Ubuntu 24.04, worker join was unstable because of remote `join.sh` execution permissions
3. while recovering the control plane, it was easy to misread containerd / CRI / kubelet cgroup alignment and make the situation worse

So the current baseline is not just "we changed to Ubuntu". It is the result of real troubleshooting and recovery.

## Timeline

### 2026-04-03: Initial baseline and first bring-up failure

Related commit:

- `b5b3c2c` `bootstrap multipass k8s lab baseline`

The initial baseline assumed a Rocky-family guest strongly:

- default `multipass_image = "rocky-8"`
- default `vm_user = "rocky"`
- cloud-init centered on RPM-family installation flow

The issue was that the current Multipass environment did not handle the `rocky-8` alias reliably. In that state, `./scripts/k8s-tool.sh up` could not complete a normal 3-node bring-up.

This mattered because if the infrastructure does not come up, the upper-layer PoC validation is blocked as well. In `artifact-handoff-poc`, full 3-node validation was blocked at this stage, and only a partial same-node scenario could be validated on the leftover single node.

### 2026-04-03: Guest baseline switched to Ubuntu 24.04

Related commit:

- `4843572` `Switch lab baseline guest to Ubuntu 24.04`

This change was not just a string replacement. It changed the execution path required by the guest OS switch.

Major changes:

- `dev.auto.tfvars` default changed from `rocky-8` to `24.04`
- `vm_user` changed from `rocky` to `ubuntu`
- `cloud-init/k8s.yaml` changed from RPM/dnf flow to apt-based installation flow
- the default user changed in `scripts/cluster/cluster-init.sh`, `scripts/cluster/join-all.sh`, and `scripts/multipass/multipass-run-remote.sh`
- README and scope documents were updated to the Ubuntu 24.04 baseline

The key lesson was that changing the image name alone is not enough.

All of these move together:

- guest user
- package manager
- Kubernetes repository setup
- `containerd` installation path and initialization

The bootstrap only works when all four align.

### 2026-04-04: Worker join execution-permission fix

Related commit:

- `9a704d6` `Fix worker join script permissions`

The problem was that `${VM_HOME}/join.sh` did not always have the right execution path inside the worker VM.

Before:

```bash
chmod +x ${VM_HOME}/join.sh && sudo bash ${VM_HOME}/join.sh
```

After:

```bash
sudo chmod +x ${VM_HOME}/join.sh && sudo bash ${VM_HOME}/join.sh
```

The difference looks small, but it mattered. Depending on how the file landed remotely, a normal user could fail to change the execution bit. Once worker join drifts, the visible symptom is only "the node did not join", which makes it hard to tell whether the root cause is permissions, token issues, or networking.

So this fix was about remote execution environment alignment, not Kubernetes join semantics themselves.

### 2026-04-04: Control plane instability and runtime misreading

Related commit:

- `0e660a5` `Document Rocky8 runtime troubleshooting note`

The commit itself only added a short README note, but the background mattered more. The real symptoms were:

- `kube-apiserver` repeatedly exited
- `6443` was frequently refused
- `etcd` / `kube-apiserver` static pods kept being recreated
- nodes were visible but stayed `NotReady`

At first it looked like a common cgroup mismatch problem:

- kubelet expected `systemd`
- `crictl info` showed top-level `systemdCgroup=false`

But that value could not be read in isolation.

What actually mattered:

- the runtime was `io.containerd.runc.v2`
- with `containerd 1.7.28`, `runc.options.SystemdCgroup = true` mattered more than the top-level field
- forcing top-level `systemd_cgroup=true` too quickly could break the CRI plugin itself

The key lesson was that the symptom may look like kube-apiserver failure while the real cause is runtime alignment, and that reading only one field from `crictl info` can make the situation worse.

## Actual Recovery Order

Recovery happened in this order:

1. revert the incorrectly touched top-level containerd setting
2. restart `containerd`
3. restart `kubelet`
4. confirm control-plane static pod stabilization
5. confirm flannel/CNI recovery
6. confirm all 3 nodes return to `Ready`

The important point is that `etcd` and `kube-apiserver` manifests were not edited first. If static pods are being recreated, changing control-plane manifests immediately can invert cause and effect.

In this case, runtime alignment had to be fixed first.

## Diagnostic Order For Similar Problems

1. `kubectl get nodes -o wide`
2. `kubectl get pods -A -o wide`
3. `sudo crictl info`
4. `sudo systemctl show -p ExecStart containerd`
5. `runc.options.SystemdCgroup` in `/etc/containerd/config.toml`
6. `ss -lntp | egrep '6443|2379|2380|10257|10259'`
7. `kubectl -n kube-flannel get pods -o wide`

This order is intended to check node state, runtime state, control-plane ports, and CNI recovery together, instead of staring only at the symptom "the API server is down".

## Why This Document Also Mattered To artifact-handoff-poc

The incident path in this repository did not stay only inside the infrastructure repo. Sprint 1 validation in `artifact-handoff-poc` was directly affected in this order:

- the 3-node lab failed to come up
- only same-node validation was partially possible in a single-node environment
- cross-node peer fetch validation became possible after control-plane recovery
- finally, cross-node validation was confirmed in the real 3-node lab

So this troubleshooting history is not only an infra memo. It directly shaped whether the upper-layer PoC could be validated.

## Lessons Worth Keeping

- a stable guest image in the current catalog matters more than an image that looks theoretically acceptable
- what looks like kubeadm failure may actually come from guest user, permissions, cloud-init, package path, or runtime alignment
- do not change settings based on a single `crictl info` field; read runtime options and CRI plugin health together
- even if control-plane static pods are failing, manifest edits should remain the last step
- upper-layer workload results should always be interpreted separately from infrastructure readiness

## 9. 2026-04-06 ~ 2026-04-08: Boundary Confirmation During PoC Revalidation

While rerunning same-node / cross-node / failure scenarios in `artifact-handoff-poc`, one more lesson became worth preserving in this repository.

The key point is that "the lower-layer PoC run failed" does not automatically mean `infra-lab` regressed.

During that revalidation window, the first things checked were:

1. `./scripts/k8s-tool.sh status`
2. `kubectl get nodes -o wide`
3. pod status in the `artifact-handoff` namespace

Those checks continued to show:

- `lab-master-0`
- `lab-worker-0`
- `lab-worker-1`

All three nodes remained `Ready`, and the control-plane endpoint answered normally. So `infra-lab` itself had not broken again at that time.

Instead, the actual issues found in the upper-layer PoC were:

- host `python3` was 3.6, so helper scripts using `text=True` failed
- old artifact cache and old pod processes made the first cross-node rerun appear as `source=local`
- sandbox access could block API-server calls with `socket: operation not permitted`

All three are different from `infra-lab` VM bring-up, kubeadm bootstrap, worker join, or containerd-baseline issues.

So the practical reading is:

- if the 3-node `Ready` state still holds, suspect the workload repo's scripts, cache, or pod lifecycle first
- if sandbox networking blocks `kubectl`, that may be an execution-environment issue rather than a lab regression
- troubleshooting in `infra-lab` should stay focused on VM lifecycle, kubeadm/bootstrap, CNI, and runtime alignment, while workload-specific validation issues should be documented in the upper-layer PoC repository

This note exists so future failures are not immediately interpreted by mixing infrastructure and workload responsibilities.
