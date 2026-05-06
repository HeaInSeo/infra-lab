# Cilium 설정 가이드

이 문서는 infra-lab 클러스터에서 Cilium이 어떤 역할을 하고, 어떻게 구성되어 있으며, 어떤 문제가 있는지를 상세히 기술한다.

---

## 목차

1. [역할 및 개요](#1-역할-및-개요)
2. [아키텍처](#2-아키텍처)
3. [설정 파일 상세](#3-설정-파일-상세)
4. [설치 방법](#4-설치-방법)
5. [Gateway API 구성](#5-gateway-api-구성)
6. [검증](#6-검증)
7. [알려진 문제 및 제한 사항](#7-알려진-문제-및-제한-사항)
8. [환경별 커스터마이징](#8-환경별-커스터마이징)
9. [제거](#9-제거)
10. [트러블슈팅](#10-트러블슈팅)

---

## 1. 역할 및 개요

Cilium은 이 프로젝트에서 세 가지 역할을 동시에 담당한다.

| 역할 | 기존 컴포넌트 | Cilium 대체 |
|------|-------------|------------|
| CNI (Pod 네트워킹) | Flannel | Cilium CNI (eBPF 기반) |
| kube-proxy 대체 | kube-proxy | `kubeProxyReplacement: true` |
| LoadBalancer IP 광고 | MetalLB | Cilium L2 Announcements |
| Ingress/Gateway | (없음) | Gateway API (HTTPRoute + GRPCRoute) |

**Cilium은 optional addon**으로 분류되어 있다. 클러스터 기본값은 Flannel + kube-proxy이며, Cilium은 마이그레이션 스크립트(`scripts/cluster/flannel-to-cilium.sh`)를 통해 전환한다.

### 버전 정보

| 컴포넌트 | 버전 |
|---------|-----|
| Cilium | 1.16.5 |
| Gateway API CRD | v1.2.0 (`install.sh` 기본값) |
| 대상 Kubernetes | v1.32 |
| IPAM 모드 | kubernetes (kubeadm pod-network-cidr 사용) |

---

## 2. 아키텍처

### 2.1 컴포넌트 구성

```
┌─────────────────────────────────────────────────────────────┐
│ seoy 호스트 (Rocky Linux 8)                                   │
│                                                             │
│  kubectl / helm / browser                                   │
│  sudo ip route replace 10.113.24.96/32 dev mpqemubr0        │
└──────────────────────────┬──────────────────────────────────┘
                           │ mpqemubr0 bridge
┌──────────────────────────▼──────────────────────────────────┐
│ Kubernetes 클러스터 (kubeadm, Ubuntu 24.04 VM)               │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ Cilium L2 LB                                        │    │
│  │  CiliumLoadBalancerIPPool: 10.113.24.96/32          │    │
│  │  CiliumL2AnnouncementPolicy: ARP 광고 (eth*, enp*, ens*) │
│  └────────────────────┬────────────────────────────────┘    │
│                       │ EXTERNAL-IP: 10.113.24.96           │
│  ┌────────────────────▼────────────────────────────────┐    │
│  │ cilium-gateway (nodevault-system NS)                │    │
│  │  GatewayClass: cilium                               │    │
│  │  Listener: HTTP :80                                 │    │
│  └──────────┬───────────────────────┬──────────────────┘    │
│             │ host: harbor.*         │ host: nodevault.*     │
│  ┌──────────▼──────────┐  ┌─────────▼──────────────────┐   │
│  │ HTTPRoute (harbor NS)│  │ GRPCRoute (nodevault-system) │  │
│  │ → harbor:80          │  │ → nodevault-controlplane:50051│ │
│  └─────────────────────┘  │ ⚠ 미래 예정 (현재 미적용)   │   │
│                           └────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 트래픽 흐름

1. 호스트에서 `10.113.24.96`으로 요청 발생
2. `ip route`로 mpqemubr0 브리지를 통해 VM 네트워크로 진입
3. Cilium L2 ARP 광고로 해당 IP를 가진 노드의 Cilium이 패킷 수신
4. `cilium-gateway` Service(LoadBalancer)로 전달
5. 요청 hostname에 따라 HTTPRoute 또는 GRPCRoute로 분기
6. 각 namespace의 backend Service로 최종 전달

### 2.3 관련 파일 목록

```
addons/
  optional/cilium/
    install.sh          # Cilium Helm 설치 + Gateway API CRD 설치
    uninstall.sh        # Cilium 제거
    verify.sh           # 설치 상태 검증
  values/cilium/
    values.yaml         # Cilium Helm values
    l2pool.yaml         # CiliumLoadBalancerIPPool + CiliumL2AnnouncementPolicy

scripts/cluster/
  flannel-to-cilium.sh  # Flannel → Cilium 마이그레이션 (권장 설치 경로)

k8s/
  gateway/01-gateway.yaml      # Gateway 리소스
  harbor/01-httproute.yaml     # Harbor HTTPRoute + ReferenceGrant
  nodevault/01-grpcroute.yaml  # NodeVault GRPCRoute (미래 예정)
```

---

## 3. 설정 파일 상세

### 3.1 Cilium Helm values (`addons/values/cilium/values.yaml`)

```yaml
ipam:
  mode: kubernetes
```

`kubernetes` IPAM 모드는 kubeadm에서 지정한 `--pod-network-cidr`(기본 `10.244.0.0/16`)을 그대로 사용한다. Cilium 자체 IPAM(`cluster-pool`)을 쓰지 않으므로 kubeadm 기본 설정과 충돌하지 않는다.

```yaml
kubeProxyReplacement: true
```

kube-proxy 없이 Cilium eBPF가 Service 라우팅을 처리한다. `install.sh`에서 API 서버 주소(`k8sServiceHost`, `k8sServicePort`)를 함께 넘겨야 하며, 이를 위해 설치 스크립트가 자동으로 감지한다.

```yaml
operator:
  replicas: 1
```

단일 노드 operator. HA가 불필요한 lab 환경 최적화이다.

```yaml
gatewayAPI:
  enabled: true
```

HTTPRoute, GRPCRoute, GatewayClass 등 Gateway API 리소스를 Cilium이 처리하도록 활성화한다. 이 옵션이 없으면 Cilium이 Gateway를 인식하지 않는다.

```yaml
l2announcements:
  enabled: true
```

ARP 기반 L2 IP 광고를 활성화한다. `CiliumLoadBalancerIPPool`과 `CiliumL2AnnouncementPolicy`가 적용되어야 실제로 동작한다.

```yaml
debug:
  enabled: false
```

운영 기본값. 디버깅이 필요한 경우 `CILIUM_NS=kube-system kubectl set env ds/cilium CILIUM_DEBUG=true` 로 임시 활성화할 수 있다.

---

### 3.2 L2 LB 풀 (`addons/values/cilium/l2pool.yaml`)

```yaml
apiVersion: "cilium.io/v2alpha1"
kind: CiliumLoadBalancerIPPool
metadata:
  name: lab-default-pool
spec:
  blocks:
    - cidr: "10.113.24.96/32"
```

**LoadBalancer Service에 할당 가능한 외부 IP 범위를 정의한다.** `/32`이므로 IP가 정확히 1개이다. `cilium-gateway` Service가 이 IP를 할당받아 `EXTERNAL-IP: 10.113.24.96`으로 표시된다.

```yaml
apiVersion: "cilium.io/v2alpha1"
kind: CiliumL2AnnouncementPolicy
metadata:
  name: lab-default-l2
spec:
  loadBalancerIPs: true
  interfaces:
    - ^eth.*
    - ^enp.*
    - ^ens.*
```

**해당 IP를 ARP로 광고할 정책을 정의한다.** `interfaces` 목록은 정규식이며 VM의 실제 인터페이스명과 매칭된다. Ubuntu 24.04 VM에서는 주로 `enp*` 계열이 매칭된다.

`loadBalancerIPs: true`는 LoadBalancer 타입 Service에 할당된 외부 IP를 광고한다는 의미이다. `externalIPs`도 별도로 설정 가능하지만 이 프로젝트에서는 사용하지 않는다.

---

## 4. 설치 방법

### 4.1 사전 조건

| 항목 | 요구 사항 |
|------|---------|
| Kubernetes | v1.32 (kubeadm 구성) |
| Helm | 설치 및 PATH 설정 |
| KUBECONFIG | 유효한 클러스터 접근 |
| 인터넷 접근 | Cilium Helm chart, Gateway API CRD 다운로드 |
| 기존 CNI | Flannel (flannel-to-cilium.sh 사용 시) |

### 4.2 Flannel → Cilium 마이그레이션 (권장)

infra-lab의 기본 클러스터는 Flannel로 구성된다. Cilium으로 전환할 때는 전용 마이그레이션 스크립트를 사용한다.

```bash
# Multipass 환경
NAME_PREFIX=lab MASTERS=1 WORKERS=2 \
  bash scripts/cluster/flannel-to-cilium.sh

# SSH 환경 (libvirt 등)
VM_RUNTIME=ssh \
  MASTER_ENDPOINTS=192.168.100.10 \
  WORKER_ENDPOINTS=192.168.100.11,192.168.100.12 \
  bash scripts/cluster/flannel-to-cilium.sh
```

**마이그레이션 중 Pod 네트워크가 일시 단절된다.** Flannel 제거부터 Cilium DaemonSet 준비 완료까지 약 2~5분이 소요된다.

### 4.3 마이그레이션 스크립트 상세 (`scripts/cluster/flannel-to-cilium.sh`)

스크립트는 7단계로 실행된다.

**Step 1 — MetalLB 제거**

```bash
if kubectl -n metallb-system get deployment controller >/dev/null 2>&1; then
  bash addons/optional/metallb/uninstall.sh
fi
```

MetalLB가 설치되어 있으면 먼저 제거한다. Cilium L2 Announcements가 동일한 역할을 대체하므로 충돌 방지를 위해 필수이다.

**Step 2 — Flannel 제거**

```bash
kubectl delete -f "${FLANNEL_MANIFEST}" --ignore-not-found
kubectl delete namespace kube-flannel --ignore-not-found
```

Flannel DaemonSet과 namespace를 삭제한다. 이 시점부터 Pod 간 통신이 단절된다.

**Step 3 — 각 노드 CNI 파일 정리**

```bash
rm -f /etc/cni/net.d/10-flannel.conflist
ip link delete flannel.1 2>/dev/null || true
iptables-save | grep -v FLANNEL | iptables-restore || true
```

Flannel이 남긴 CNI 설정 파일, 가상 인터페이스(`flannel.1`), iptables 규칙을 각 노드에서 직접 제거한다. 이 정리가 불완전하면 Cilium 설치 후 네트워크 충돌이 발생할 수 있다.

**Step 4 — Cilium 설치**

```bash
bash addons/optional/cilium/install.sh
```

`install.sh`를 호출한다 (아래 4.4 참고).

**Step 5 — 노드 Ready 대기**

```bash
kubectl wait --for=condition=Ready nodes --all --timeout=300s
```

**Step 6 — coredns 재시작**

```bash
kubectl -n kube-system rollout restart deployment/coredns
```

Flannel IP를 갖고 있던 coredns Pod가 Cilium 네트워크에서 새 IP를 받도록 재시작한다. 이 단계를 생략하면 DNS 해석에 문제가 생길 수 있다.

**Step 7 — 검증**

```bash
bash addons/optional/cilium/verify.sh
```

---

### 4.4 직접 설치 (`addons/optional/cilium/install.sh`)

Flannel 없이 처음부터 Cilium으로 구성하거나, 마이그레이션 스크립트 없이 수동 실행할 때 사용한다.

```bash
# 기본 실행
bash addons/optional/cilium/install.sh

# 버전 지정
CILIUM_VERSION=1.16.5 GATEWAY_API_VERSION=v1.2.0 \
  bash addons/optional/cilium/install.sh
```

스크립트 내부 실행 순서:

1. **Gateway API CRD 설치**
   ```bash
   kubectl apply -f \
     "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml"
   ```
   Cilium Helm 설치 전에 CRD가 먼저 있어야 한다. 순서를 바꾸면 `gatewayAPI.enabled: true` 처리가 실패한다.

2. **Helm repo 추가 및 업데이트**
   ```bash
   helm repo add cilium https://helm.cilium.io/ --force-update
   helm repo update cilium
   ```

3. **K8s API 서버 주소 자동 감지**
   ```bash
   K8S_API_HOST=$(kubectl get endpoints kubernetes \
     -o jsonpath='{.subsets[0].addresses[0].ip}')
   K8S_API_PORT=$(kubectl get endpoints kubernetes \
     -o jsonpath='{.subsets[0].ports[0].port}')
   ```
   `kubeProxyReplacement: true` 모드에서는 Cilium이 API 서버 주소를 알아야 한다. `hostname -I` 대신 `kubernetes` endpoint에서 직접 읽으므로 신뢰할 수 있다.

4. **Cilium Helm 설치**
   ```bash
   helm upgrade --install cilium cilium/cilium \
     --version "${CILIUM_VERSION}" \
     --namespace kube-system \
     --values addons/values/cilium/values.yaml \
     --set k8sServiceHost="${K8S_API_HOST}" \
     --set k8sServicePort="${K8S_API_PORT}" \
     --wait --timeout 5m
   ```

5. **DaemonSet rollout 대기**
   ```bash
   kubectl -n kube-system rollout status ds/cilium --timeout=300s
   ```

6. **L2 LB 풀 적용**
   ```bash
   kubectl apply -f addons/values/cilium/l2pool.yaml
   ```
   DaemonSet이 준비된 후 적용해야 CRD가 존재한다.

---

## 5. Gateway API 구성

### 5.1 Gateway (`k8s/gateway/01-gateway.yaml`)

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: cilium-gateway
  namespace: nodevault-system
spec:
  gatewayClassName: cilium
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
```

**핵심 포인트:**

- `gatewayClassName: cilium` — Cilium이 이 Gateway를 관리한다.
- `allowedRoutes.namespaces.from: All` — 모든 namespace의 Route가 이 Gateway에 붙을 수 있다. `harbor` namespace의 HTTPRoute가 `nodevault-system` namespace의 Gateway를 참조하는 구조이기 때문에 필요하다.
- Listener가 HTTP 단일 포트(80)이다. TLS 리스너가 없다 → [알려진 문제 #3, #4 참고](#7-알려진-문제-및-제한-사항).

적용 후 확인:

```bash
kubectl get gateway cilium-gateway -n nodevault-system
# EXTERNAL-IP 컬럼에 10.113.24.96 이 표시되면 정상
```

### 5.2 Harbor HTTPRoute (`k8s/harbor/01-httproute.yaml`)

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: harbor
  namespace: harbor
spec:
  parentRefs:
    - name: cilium-gateway
      namespace: nodevault-system
  hostnames:
    - "harbor.10.113.24.96.nip.io"
  rules:
    - backendRefs:
        - name: harbor
          port: 80
```

`nip.io`를 hostname으로 사용하므로 별도 DNS 설정 없이 IP 기반 도메인이 자동으로 동작한다. 다만 호스트 OS에서 `10.113.24.96`으로 라우팅이 설정되어 있어야 한다.

**ReferenceGrant:**

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: harbor-from-gateway
  namespace: harbor
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      namespace: harbor
  to:
    - group: ""
      kind: Service
```

`harbor` namespace의 HTTPRoute가 같은 namespace의 Service를 참조하므로 ReferenceGrant가 필요하다. Gateway API v1 표준 패턴이다.

### 5.3 NodeVault GRPCRoute (`k8s/nodevault/01-grpcroute.yaml`)

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  name: nodevault-grpc
  namespace: nodevault-system
spec:
  parentRefs:
    - name: cilium-gateway
      namespace: nodevault-system
  hostnames:
    - "nodevault.10.113.24.96.nip.io"
  rules:
    - backendRefs:
        - name: nodevault-controlplane
          port: 50051
```

> ⚠ **현재 이 리소스는 미적용 상태이다.** NodeVault는 `seoy` 호스트에서 바이너리로 직접 실행 중이며, `nodevault-controlplane` Service가 클러스터에 존재하지 않는다. 이 YAML은 향후 in-cluster 전환 시 사용할 정의로 보관되어 있다.

gRPC는 HTTP/2가 필요하다. 이 Gateway는 HTTP(h2c) 리스너이므로 클라이언트가 반드시 h2c 업그레이드를 지원해야 한다:

```go
conn, err := grpc.NewClient(
    "nodevault.10.113.24.96.nip.io:80",
    grpc.WithTransportCredentials(insecure.NewCredentials()),
)
```

TLS 없이 h2c로 연결한다. `grpcurl`로 테스트할 때도 `-plaintext` 플래그를 사용한다.

### 5.4 호스트에서 LB IP 접근

Cilium이 L2 ARP로 `10.113.24.96`을 광고하지만, 호스트 OS는 해당 IP로의 라우팅이 없으면 패킷을 버린다. 호스트에서 접근하려면 수동으로 라우트를 설정해야 한다.

```bash
# mpqemubr0: multipass VM 브리지 인터페이스
sudo ip route replace 10.113.24.96/32 dev mpqemubr0
```

이 설정은 재부팅 후 사라진다. 영속적으로 유지하려면 `NetworkManager` 또는 systemd-networkd 설정이 필요하다.

Gateway와 Route를 모두 적용한 후 동작 확인:

```bash
curl http://harbor.10.113.24.96.nip.io
grpcurl -plaintext nodevault.10.113.24.96.nip.io:80 list
```

---

## 6. 검증

```bash
bash addons/optional/cilium/verify.sh
```

또는 addon 관리 스크립트를 통해:

```bash
bash addons/manage.sh verify optional cilium
```

verify.sh가 확인하는 항목:

| 항목 | 확인 명령 |
|------|---------|
| cilium DaemonSet 존재 | `kubectl -n kube-system get ds cilium` |
| cilium-operator Deployment 존재 | `kubectl -n kube-system get deployment cilium-operator` |
| 모든 cilium Pod Running | phase != Running 개수가 0 |
| GatewayClass `cilium` 등록 | `kubectl get gatewayclass cilium` |
| CiliumLoadBalancerIPPool 존재 | CRD row count >= 1 |
| CiliumL2AnnouncementPolicy 존재 | CRD row count >= 1 |

모든 항목 PASS 시 exit code 0, 하나라도 실패하면 exit code 1을 반환한다.

추가 확인 명령:

```bash
# Cilium 상태 (cilium CLI 사용 시)
cilium status --wait

# Gateway에 할당된 EXTERNAL-IP 확인
kubectl get gateway cilium-gateway -n nodevault-system

# L2 풀 상태 확인
kubectl get ciliumloadbalancerippools.cilium.io

# L2 광고 정책 확인
kubectl get ciliuml2announcementpolicies.cilium.io

# 실제 ARP 광고 확인 (호스트에서)
arping -I mpqemubr0 10.113.24.96
```

---

## 7. 알려진 문제 및 제한 사항

### 문제 1: Gateway API 버전 불일치

**위치:** `addons/optional/cilium/install.sh:10`, `k8s/gateway/01-gateway.yaml:8` 주석

`k8s/gateway/01-gateway.yaml` 주석에는 "Gateway API CRDs v1.3.0 설치"라고 적혀 있으나, `install.sh`의 기본값은 `v1.2.0`이다.

```bash
# install.sh
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.2.0}"   # 실제 설치 버전

# 01-gateway.yaml 주석
# 설치 순서: 1. Gateway API CRDs v1.3.0 설치 ...       # 주석의 버전
```

**영향:** GRPCRoute는 v1.2.0에서 Experimental에서 Standard로 졸업되었으므로 현재 기능은 정상 동작한다. 그러나 v1.3.0에 추가된 기능(BackendLBPolicy 등)을 사용하려는 경우 혼동이 생긴다.

**수정 방법:** 코드와 주석 중 하나로 통일한다.

```bash
# install.sh에서 버전을 v1.3.0으로 올리거나
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.3.0}"

# 주석을 v1.2.0으로 수정
# 설치 순서: 1. Gateway API CRDs v1.2.0 설치 ...
```

---

### 문제 2: L2 IP Pool이 /32 (IP 1개)

**위치:** `addons/values/cilium/l2pool.yaml:11`

```yaml
blocks:
  - cidr: "10.113.24.96/32"
```

현재 IP 풀에 IP가 1개뿐이다. `cilium-gateway` Service가 이 IP를 가져가면 추가 LoadBalancer Service를 생성할 수 없다.

**영향:** 현재는 Gateway 하나로 HTTPRoute/GRPCRoute를 공유하므로 문제없다. 그러나 새로운 LoadBalancer 타입 Service를 직접 만들면 IP 할당에 실패한다.

**수정 방법:** CIDR을 범위로 확장한다.

```yaml
blocks:
  - cidr: "10.113.24.96/28"   # 10.113.24.96 ~ 10.113.24.111 (14개 가용)
```

또는 MetalLB 방식처럼 범위를 명시한다:

```yaml
blocks:
  - start: "10.113.24.96"
    stop: "10.113.24.110"
```

---

### 문제 3: Harbor가 HTTP로 노출됨 (TLS 없음)

**위치:** `k8s/gateway/01-gateway.yaml:26-32`, `k8s/harbor/01-httproute.yaml`

Gateway에 HTTP 리스너만 존재하고 HTTPS 리스너가 없다. Harbor registry 접근 시 인증 토큰이 평문 전송된다.

**영향:** `docker login harbor.10.113.24.96.nip.io` 시 자격 증명이 암호화되지 않는다. 개인 랩에서는 허용 가능하지만, 팀 공유 환경에서는 보안 문제이다.

**수정 방법 (cert-manager + Let's Encrypt 또는 self-signed):**

```yaml
# 01-gateway.yaml에 HTTPS 리스너 추가
listeners:
  - name: http
    protocol: HTTP
    port: 80
  - name: https
    protocol: HTTPS
    port: 443
    tls:
      mode: Terminate
      certificateRefs:
        - kind: Secret
          name: harbor-tls
          namespace: harbor
```

---

### 문제 4: GRPCRoute + HTTP 리스너 (h2c 의존)

**위치:** `k8s/gateway/01-gateway.yaml:26`, `k8s/nodevault/01-grpcroute.yaml`

gRPC는 HTTP/2가 필요하다. 현재 Gateway는 HTTP/1.1 기본 포트(80)에서 h2c(평문 HTTP/2)로 동작한다. 클라이언트가 반드시 h2c를 사용해야 하며, 일반 HTTP/1.1 클라이언트로는 접근 불가능하다.

**영향:** gRPC 클라이언트 코드에서 명시적으로 `insecure.NewCredentials()`와 h2c를 사용해야 한다. TLS 기반 gRPC(기본)와 혼용하면 연결 실패가 발생한다.

**근본 해결:** HTTPS 리스너에 TLS를 붙이면 ALPN으로 HTTP/2를 협상하므로 h2c 의존성이 사라진다.

---

### 문제 5: GRPCRoute가 현재 미적용 상태

**위치:** `k8s/nodevault/01-grpcroute.yaml:3-9`

파일 상단 주석에 명시되어 있듯이, NodeVault는 현재 `seoy` 호스트에서 바이너리로 실행 중이다. `nodevault-controlplane` Service가 클러스터에 없으므로 이 GRPCRoute를 `kubectl apply` 하면 Gateway가 `BackendNotFound` 상태가 된다.

**영향:** `kubectl apply -f k8s/nodevault/` 실행 시 미완성 상태가 클러스터에 적용될 수 있다.

**수정 방법:** 파일을 별도 디렉토리로 분리하거나, 적용 전에 명시적 주석을 강화한다.

```bash
# k8s/nodevault/ 를 별도로 관리
k8s/
  nodevault/
    01-grpcroute.yaml      # 현재: ⚠ 적용 금지 (노드볼트 in-cluster 전환 후 사용)
    deploy/                # 향후 in-cluster 배포 매니페스트
```

---

### 문제 6: `flannel-to-cilium.sh` 멱등성 부족

**위치:** `scripts/cluster/flannel-to-cilium.sh:90-106`

CNI 파일 정리 단계가 중간에 실패하면 스크립트 전체가 중단된다(`set -euo pipefail`). 재실행 시 이미 삭제된 리소스를 다시 삭제하려고 시도하면 오류가 발생할 수 있다.

특히 `ip link delete flannel.1`은 이미 삭제된 인터페이스에 실행하면 오류를 반환한다.

```bash
# 현재 (|| true가 있어 이 부분은 괜찮음)
ip link delete flannel.1 2>/dev/null || true

# 하지만 iptables-restore 실패는 스크립트를 중단시킬 수 있음
iptables-save 2>/dev/null | grep -v FLANNEL | iptables-restore 2>/dev/null || true
```

**영향:** 마이그레이션 중 오류 발생 시 재실행이 어렵다.

---

### 문제 7: IP 및 hostname 하드코딩

**위치:** `addons/values/cilium/l2pool.yaml:11`, `k8s/harbor/01-httproute.yaml:27`, `k8s/nodevault/01-grpcroute.yaml:30`

환경별로 달라져야 할 값들이 하드코딩되어 있다.

```yaml
cidr: "10.113.24.96/32"            # l2pool.yaml
harbor.10.113.24.96.nip.io         # 01-httproute.yaml
nodevault.10.113.24.96.nip.io      # 01-grpcroute.yaml
```

**영향:** 다른 환경(다른 IP 대역)에서 재사용할 때 여러 파일을 수동으로 수정해야 한다.

**수정 방법:** 환경별 overlay 구조 도입 (ROADMAP에 `profiles/` 구조가 계획됨).

```
profiles/
  local-multipass/
    cilium/
      l2pool.yaml     # CIDR: 192.168.64.240/32
      patch-routes.yaml
  remote-seoy/
    cilium/
      l2pool.yaml     # CIDR: 10.113.24.96/32
```

---

## 8. 환경별 커스터마이징

다른 IP 환경에서 사용할 때 수정이 필요한 파일과 항목:

### LB IP 변경

```yaml
# addons/values/cilium/l2pool.yaml
spec:
  blocks:
    - cidr: "YOUR_LB_IP/32"   # 또는 범위 지정
```

```yaml
# k8s/harbor/01-httproute.yaml
hostnames:
  - "harbor.YOUR_LB_IP.nip.io"

# k8s/nodevault/01-grpcroute.yaml
hostnames:
  - "nodevault.YOUR_LB_IP.nip.io"
```

### 호스트 라우팅 업데이트

```bash
sudo ip route replace YOUR_LB_IP/32 dev YOUR_BRIDGE_INTERFACE
```

Multipass 환경: `mpqemubr0`
libvirt 환경: `virbr0` 또는 설정에 따라 다름

### Cilium 버전 변경

```bash
CILIUM_VERSION=1.17.0 bash addons/optional/cilium/install.sh
```

버전을 올릴 때 [Cilium 릴리즈 노트](https://github.com/cilium/cilium/releases)에서 breaking change를 확인할 것.

### Gateway API 버전 변경

```bash
GATEWAY_API_VERSION=v1.3.0 bash addons/optional/cilium/install.sh
```

주의: `install.sh`와 `uninstall.sh` 양쪽의 버전을 일치시켜야 한다. uninstall 시 CRD 삭제에 같은 버전의 manifest를 사용하기 때문이다.

---

## 9. 제거

```bash
bash addons/optional/cilium/uninstall.sh

# 또는
bash addons/manage.sh uninstall optional cilium
```

제거 순서:

1. `CiliumLoadBalancerIPPool` + `CiliumL2AnnouncementPolicy` 삭제
2. Cilium Helm release 제거
3. Gateway API CRD 삭제

> ⚠ Cilium 제거 후 클러스터 네트워킹이 중단된다. Flannel로 되돌리려면 `cluster-init.sh`를 재실행하거나 클러스터를 재구성해야 한다. 단순 Cilium 제거만으로는 kube-proxy가 복구되지 않는다.

Cilium 제거 후 Flannel 복원:

```bash
kubectl apply -f \
  https://raw.githubusercontent.com/flannel-io/flannel/v0.26.0/Documentation/kube-flannel.yml
```

---

## 10. 트러블슈팅

### Gateway에 EXTERNAL-IP가 할당되지 않음

```bash
kubectl get gateway cilium-gateway -n nodevault-system
# EXTERNAL-IP가 <pending> 상태
```

확인 사항:

```bash
# L2 풀이 적용되어 있는지
kubectl get ciliumloadbalancerippools.cilium.io

# L2 정책이 적용되어 있는지
kubectl get ciliuml2announcementpolicies.cilium.io

# cilium-operator 로그에서 IP 할당 오류 확인
kubectl -n kube-system logs -l name=cilium-operator --tail=50
```

### Harbor 접근 불가 (curl timeout)

1. Gateway EXTERNAL-IP 확인: `kubectl get gateway cilium-gateway -n nodevault-system`
2. 호스트 라우팅 확인: `ip route get 10.113.24.96`
3. ARP 응답 확인: `arping -I mpqemubr0 10.113.24.96`
4. HTTPRoute 상태 확인: `kubectl get httproute harbor -n harbor`

```bash
# HTTPRoute가 Gateway에 정상 연결되었는지
kubectl describe httproute harbor -n harbor
# Conditions: Accepted=True, ResolvedRefs=True 이어야 함
```

### cilium Pod CrashLoopBackOff

```bash
kubectl -n kube-system logs -l k8s-app=cilium --tail=100

# 주로 발생하는 원인
# 1. k8sServiceHost 감지 실패 — install.sh의 API 서버 감지 확인
# 2. 기존 CNI 파일 충돌 — /etc/cni/net.d/ 에 10-flannel.* 파일 잔존 여부 확인
# 3. kube-proxy 와 충돌 — kubeProxyReplacement=true 상태에서 kube-proxy Pod가 남아있는지 확인
```

### gRPC 연결 실패

```bash
# h2c 확인
grpcurl -plaintext nodevault.10.113.24.96.nip.io:80 list

# GRPCRoute 상태 확인
kubectl get grpcroute -n nodevault-system
kubectl describe grpcroute nodevault-grpc -n nodevault-system
# ⚠ nodevault-controlplane Service가 없으면 BackendNotFound 상태
```

### Cilium 설치 후 DNS 해석 실패

coredns가 Flannel IP를 가지고 있을 수 있다:

```bash
kubectl -n kube-system rollout restart deployment/coredns
kubectl -n kube-system rollout status deployment/coredns --timeout=120s
```
