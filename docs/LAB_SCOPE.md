# LAB_SCOPE

## Summary

`infra-lab` is a general-purpose Kubernetes VM lab baseline. Its job is to make it easy to create, destroy, inspect, and lightly extend a repeatable kubeadm cluster on Multipass-backed Ubuntu 24.04 virtual machines.

The repository exists to support many future K8s PoCs, not to encode one PoC's workload or business logic.

## Goals

- Provide a reusable local K8s VM lab foundation
- Keep cluster lifecycle repeatable and scriptable
- Start from a clear baseline: Ubuntu 24.04 guests + Multipass + OpenTofu + kubeadm
- Support a first practical lab shape: 3-node VM cluster
- Leave room for optional infra capabilities such as load balancer, local storage, and future networking variants

## Primary Use Cases

- node-local artifact and local storage experiments
- DaemonSet-based node agent experiments
- same-node reuse versus cross-node fetch experiments
- Cilium and service-routing validation
- storage behavior checks
- operator development and integration testing

## Repository Responsibilities

- Host bootstrap scripts for required local tooling
- VM lifecycle orchestration through OpenTofu and Multipass
- kubeadm bootstrap and join flow
- kubeconfig export for local operator use
- small cluster-oriented helper commands
- base and optional infra add-on management
- documentation for usage boundaries

## Non-Goals

- This is not an application deployment repository
- This is not an artifact-agent, catalog, storage application, or Cilium implementation repository
- This is not a production-grade cluster provisioning tool
- This is not a generic multi-provider abstraction layer
- This is not the place to embed project-specific workloads deeply into the baseline

## Scope Cuts Made Deliberately

Compared with `mac-k8s-multipass-terraform`, this repo intentionally removes or avoids:

- service-specific VM setup such as MySQL or Redis helpers
- service-stack-first add-on bundles
- docs centered on a single past test flow rather than the lab platform itself
- implicit coupling between cluster bring-up and project-specific workloads

## Add-on Policy

### Base

Base add-ons should meet all of these criteria:

- useful for most lab clusters
- low coupling to a specific project
- low conceptual cost for a default baseline

### Optional

Optional add-ons are allowed when they:

- unlock important categories of experiments
- add meaningful cluster behavior
- still remain infra-oriented rather than application-oriented

## Profiles

`profiles/` is reserved for future overlays such as:

- `cilium`
- `storage-lab`
- `operator-dev`
- `gateway-lab`

These profiles should layer on top of the baseline instead of rewriting the baseline.

## Decision Rule

When adding something new, the key question is:

Can another unrelated K8s project reuse this without inheriting project-specific baggage?

If the answer is no, it likely belongs outside the core baseline repo.
