# remote-seoy Cilium Guide

## 1. Purpose

This document explains the Cilium configuration currently running on the `remote-seoy` VM lab at `100.123.80.48`.

It has two goals:

1. explain the current live operating state clearly
2. separate the generic addon example path from the actual remote-seoy operating baseline

This is not a service-mesh proposal document.

## 2. What Cilium currently does

On `remote-seoy`, Cilium currently provides:

- pod networking
- kube-proxy replacement
- LoadBalancer IPAM
- L2 announcement
- Gateway API ingress

That means the current role is cluster networking plus north-south ingress.

It does not currently provide:

- east-west service mesh
- policy-heavy CiliumNetworkPolicy operations
- mutual auth / SPIRE / mTLS
- ClusterMesh

## 3. Current live baseline

Current snapshot baseline:

- Cilium version: `1.19.1`
- Kubernetes: `v1.32.13`
- IPAM: `cluster-pool`
- Routing: `tunnel`
- Tunnel protocol: `vxlan`
- kubeProxyReplacement: `true`
- Gateway API: `enabled`
- Hubble: `enabled`
- Gateway: `harbor/lab-gateway`
- LoadBalancer IP pool: `10.113.24.100/28`
- L2 interface: `mpqemubr0`

Relevant snapshot files:

- [profiles/remote-seoy/cilium/live-snapshot/cilium-helm-values.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/cilium-helm-values.yaml:1)
- [profiles/remote-seoy/cilium/live-snapshot/cilium-status.txt](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/cilium-status.txt:1)
- [profiles/remote-seoy/cilium/live-snapshot/gateway.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/gateway.yaml:1)
- [profiles/remote-seoy/cilium/live-snapshot/httproutes.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/httproutes.yaml:1)
- [profiles/remote-seoy/cilium/live-snapshot/lb-pool.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/lb-pool.yaml:1)
- [profiles/remote-seoy/cilium/live-snapshot/l2-announcement-policy.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot/l2-announcement-policy.yaml:1)

## 4. Why this fits the lab

`remote-seoy` is not a public cloud environment. It is an on-prem-like VM lab.

That environment has these characteristics:

- external clients reach the lab through a Gateway / LoadBalancer IP
- the host enters the VM network through a bridge interface
- there is no cloud-managed load balancer or managed ingress controller

In that setting, this combination is practical:

- `cluster-pool`
- `vxlan`
- `LB IPAM`
- `L2 announcement`
- `Gateway API`

So Cilium is doing more than just CNI. It is also acting as the lab networking platform.

## 5. Traffic flow

The rough traffic path is:

1. an external client reaches `10.113.24.96` or `*.10.113.24.96.nip.io`
2. the host routes traffic toward the VM bridge
3. Cilium L2 announcement advertises that IP
4. the Gateway Service owns the IP
5. the Gateway listener accepts the HTTP or HTTPS request
6. hostname-based routing selects the matching `HTTPRoute`
7. traffic is forwarded to the backend `Service`

The important point:

- this is north-south ingress
- it is not east-west service-mesh routing

## 6. Gateway structure

The current live Gateway is `harbor/lab-gateway`.

Key properties:

- namespace: `harbor`
- `gatewayClassName: cilium`
- `HTTP:80`
- `HTTPS:443`
- `allowedRoutes.namespaces.from: All`

Why `from: All` matters:

- routes from namespaces other than `harbor` attach to this Gateway
- examples include `dev-space-observability` and `shift-left-observability`

Cross-namespace parentRef allowance is therefore a Gateway listener policy issue.

## 7. Route classification

The current live snapshot includes three HTTPRoutes:

- `harbor-route`
- `dev-space-observability`
- `shift-left-observability`

They should not all be treated the same way.

### Routes treated as baseline

- `harbor-route`
- `dev-space-observability`

These are the routes the infra baseline currently assumes.

### Workload-attached live route

- `shift-left-observability`

This route exists in the live snapshot, but it is not automatically promoted into the shared infra baseline.

Why:

- it may reflect a workload attached at a specific point in time
- promoting it would imply that the lab always provides that workload endpoint
- the current objective is Cilium baseline alignment, not workload expansion

So this route is:

- recorded in `live-snapshot/`
- intentionally omitted from `desired/`

## 8. Why GRPCRoute is not part of the baseline

There is no live `GRPCRoute` at the moment.

Existing example file:

- [k8s/nodevault/01-grpcroute.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/k8s/nodevault/01-grpcroute.yaml:1)

Its role is:

- future-facing
- a north-south gRPC ingress candidate
- an example for a future in-cluster NodeVault transition

It does not mean:

- service mesh is implemented
- the current operating baseline includes gRPC ingress

`GRPCRoute` is not equivalent to internal mesh.

## 9. Why it differs from the generic addon path

The generic addon path remains a shared example:

- common starting point
- generic explanation
- closer to kubeadm pod-network-cidr assumptions

Representative file:

- [addons/values/cilium/values.yaml](/opt/go/src/github.com/HeaInSeo/infra-lab/addons/values/cilium/values.yaml:1)

The real `remote-seoy` live baseline uses:

- `cluster-pool`
- `vxlan`
- `lab-gateway`
- `10.113.24.100/28`
- `mpqemubr0`

Forcing both models into one shared path would be risky.

Especially risky:

- reverting the live cluster toward the generic addon model
- using generic values to create an IPAM-changing upgrade path

That is why the profile split exists.

## 10. Why IPAM must not be changed in place

The current live IPAM mode is `cluster-pool`.

The key safety rule of this work is:

- document that fact
- do not treat it as an in-place change target

Why:

- changing IPAM mode changes a core network path
- that is a rebuild or migration problem, not baseline documentation
- it exceeds the scope of this cleanup effort

So the safe approach is:

- record live facts
- document a desired baseline for fresh reproduction
- leave the generic addon path unchanged

## 11. Hubble and Envoy

Hubble is enabled in the live cluster, and the Gateway controller also generates a `CiliumEnvoyConfig`.

What that means:

- Gateway API ingress is running on the Cilium/Envoy datapath
- the ingress proxy and observability layers are present

What that does not mean:

- service mesh is complete
- east-west L7 policy is in use
- mTLS mesh is enabled

Having Envoy does not imply having a service mesh.

## 12. Read-only operating flow

The profile intentionally exposes only read-only operational entrypoints.

Standard commands:

- `HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh profile-cilium-collect`
- `HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh profile-cilium-verify`
- `HOST_PROFILE=hosts/remote-lab.env ./scripts/k8s-tool.sh profile-gateway-verify`

These commands:

- collect the current live evidence
- inspect cilium status, helm values, gateway, route, and LB state
- refresh snapshot files

They do not:

- run `helm upgrade`
- run `kubectl apply`
- run `kubectl delete`
- change IPAM

## 13. Future expansion

Future axes do exist:

- GAMMA
- mutual auth / SPIRE
- ClusterMesh
- cloud-specific CNI and Gateway profiles

But they stay outside the baseline.

The order matters:

1. reduce live drift
2. close the Gateway-only baseline
3. expand experimental axes separately

## 14. What to read next

Operating evidence:

- [profiles/remote-seoy/cilium/live-snapshot](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/live-snapshot)

Reproduction baseline:

- [profiles/remote-seoy/cilium/desired](/opt/go/src/github.com/HeaInSeo/infra-lab/profiles/remote-seoy/cilium/desired)

Shorter baseline summary:

- [docs/CILIUM.ko.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/CILIUM.ko.md:1)

Project-wide explanation:

- [docs/PROJECT_GUIDE.md](/opt/go/src/github.com/HeaInSeo/infra-lab/docs/PROJECT_GUIDE.md:1)
