# K8sGPT CLI 원격 진단

## 결론

`infra-lab`에서는 K8sGPT를 먼저 Operator가 아니라 CLI 방식으로 붙인다.

CLI 방식은 노트북, CI Runner, `nightly-agent` 같은 외부 실행 환경에서 kubeconfig로 Kubernetes API Server에 접속해 클러스터 상태를 분석하는 방식이다. 클러스터 안에 Operator, CRD, Secret, 추가 Pod를 설치하지 않아도 되므로 초기 PoC와 실패 리포트 자동화에 적합하다.

현재 `infra-lab` 검증 결과는 다음과 같다.

```text
검증일: 2026-05-30
클러스터: infra-lab kubeconfig
Kubernetes: v1.32.13
K8sGPT: v0.4.33
결과: --explain 없이 AI backend 없이 analyze 성공
```

중요한 정정:

```text
K8sGPT v0.3.24에서는 backend 설정 없이 `k8sgpt analyze`가 실패했다.
K8sGPT v0.4.33에서는 `AI Provider: AI not used; --explain not set`으로 정상 실행됐다.
따라서 설치 문서에는 오래된 v0.3.24 고정 URL을 쓰지 말고 최신 릴리스를 확인하도록 적는다.
```

## CLI 방식과 Operator 방식

Kubernetes에 K8sGPT를 붙이는 방식은 크게 두 가지다.

```text
1. CLI 방식
   외부 실행 환경에서 kubeconfig로 Kubernetes API Server에 접속해 진단한다.

2. Operator 방식
   Kubernetes 클러스터 안에 k8sgpt-operator를 설치하고
   K8sGPT Custom Resource와 Result CR 기반으로 진단 결과를 관리한다.
```

현재 단계에서는 CLI 방식을 우선한다.

- 클러스터 내부에 Operator, CRD, Secret, 추가 Pod를 만들지 않는다.
- VM 안의 Kubernetes도 kubeconfig만 맞으면 외부에서 분석할 수 있다.
- `kubectl` 접속이 되는 환경이면 K8sGPT CLI도 같은 접근 방식을 사용할 수 있다.
- 실패했을 때만 실행할 수 있어 가볍다.

Operator 방식은 나중에 아래 요구가 생기면 검토한다.

- K8sGPT 결과를 클러스터 안의 `Result` CR로 관리하고 싶다.
- Slack, Grafana, Prometheus 같은 운영 도구와 연동하고 싶다.
- 여러 namespace를 지속적으로 진단하고 싶다.
- 운영팀이 상시 진단 결과를 보고 싶다.

## K8sGPT가 하는 일

K8sGPT는 Kubernetes 리소스와 Event 등을 읽고 문제를 진단한다.

```text
kubectl
= 사람이 직접 Kubernetes 상태를 보는 도구

K8sGPT
= Kubernetes 상태를 읽고
  어떤 리소스에 문제가 있는지,
  어떤 원인 가능성이 있는지,
  어떤 방향으로 확인하면 되는지 보여주는 진단 도구
```

예를 들어 다음 문제를 감지할 수 있다.

- Pod `Pending`
- Pod `CrashLoopBackOff`
- 이미지 Pull 실패
- Service Endpoint 없음
- Ingress 연결 문제
- Deployment replica 미달
- Job 실패
- PVC `Bound` 실패
- Node 상태 이상
- Event 에러

K8sGPT가 모든 데이터를 LLM에 던지는 도구라고 보면 안 된다. 기본 분석은 Analyzer가 Kubernetes API에서 읽은 리소스 상태와 Event를 기반으로 수행한다. `--explain`을 붙이면 설정된 AI backend를 사용해 설명을 보강한다.

## 로그 관련 주의

K8sGPT가 CrashLoopBackOff를 진단할 수는 있지만, 애플리케이션 로그를 자세히 읽어 원인을 확정한다고 표현하면 안 된다.

정확한 표현:

```text
K8sGPT는 Pod 상태, Container status message, Event message 등을 바탕으로
CrashLoopBackOff, ImagePullBackOff, Pending 같은 문제를 감지할 수 있다.

하지만 애플리케이션 내부 stacktrace나 자세한 로그는
여전히 kubectl logs로 직접 확인해야 한다.
```

## infra-lab 연결 모델

`infra-lab`은 `multipass`와 `libvirt` backend를 모두 수용한다. K8sGPT CLI 입장에서는 backend 차이가 중요하지 않다. 중요한 것은 실행 환경에서 접근 가능한 kubeconfig다.

```text
노트북 / CI / nightly-agent
  |
  | kubeconfig
  v
VM Kubernetes API Server
  |
  | Kubernetes 상태 조회
  v
K8sGPT CLI
```

기본 확인 순서:

```bash
KUBECONFIG=./kubeconfig kubectl get nodes
KUBECONFIG=./kubeconfig k8sgpt analyze
```

`kubectl get nodes`가 실패하면 K8sGPT 문제가 아니라 kubeconfig, API Server 주소, 인증서 SAN, 방화벽, SSH 터널, VM 네트워크 문제일 가능성이 높다.

## VM 연결 방식

### 직접 연결

외부 실행 환경에서 VM의 API Server로 직접 붙는 방식이다.

```text
노트북 / CI
  |
  | https://<VM_IP>:6443
  v
VM 안의 kube-apiserver
```

kubeconfig의 `server` 주소가 외부 기준으로 접근 가능해야 한다.

```yaml
server: https://<VM_IP>:6443
```

주의:

- VM의 `6443` 포트가 외부에서 접근 가능해야 한다.
- 방화벽이 막고 있으면 실패한다.
- kube-apiserver 인증서 SAN에 해당 IP가 없으면 `x509` 에러가 날 수 있다.
- `insecure-skip-tls-verify: true`는 테스트 환경에서만 임시로 사용한다.

### SSH 터널

VM 안의 API Server가 VM 기준 localhost에만 열려 있으면 SSH 터널을 사용할 수 있다.

```bash
ssh -L 6443:127.0.0.1:6443 ubuntu@<VM_IP>
```

kind처럼 VM 안 kubeconfig가 임의 localhost 포트를 가리키는 경우도 같은 방식으로 맞춘다.

```yaml
server: https://127.0.0.1:37123
```

```bash
ssh -L 37123:127.0.0.1:37123 ubuntu@<VM_IP>
```

이 문서는 검증 목적상 연결이 실패하면 임의 우회하지 않고 실패 원인을 그대로 기록한다.

## 설치

공식 docs의 오래된 예시에는 `v0.3.24`가 남아 있을 수 있다. `v0.3.24`는 최신이 아니며, 이 버전에서는 backend 설정 없이 `k8sgpt analyze`가 실패했다.

설치 전 최신 릴리스를 확인한다.

- GitHub Releases: <https://github.com/k8sgpt-ai/k8sgpt/releases>
- 설치 문서: <https://docs.k8sgpt.ai/getting-started/installation/>

Rocky/RHEL 계열의 현재 검증 명령:

```bash
sudo rpm -Uvh https://github.com/k8sgpt-ai/k8sgpt/releases/download/v0.4.33/k8sgpt_amd64.rpm
k8sgpt version
```

검증된 출력:

```text
k8sgpt: 0.4.33 (fb24679), built at: unknown
```

Ubuntu/Debian 계열은 GitHub Releases에서 최신 `.deb` URL을 확인한 뒤 설치한다.

```bash
curl -LO https://github.com/k8sgpt-ai/k8sgpt/releases/download/<VERSION>/k8sgpt_amd64.deb
sudo dpkg -i k8sgpt_amd64.deb
k8sgpt version
```

## 기본 사용법

### 1. Kubernetes 연결 확인

```bash
KUBECONFIG=./kubeconfig kubectl get nodes
```

`infra-lab` 검증 출력:

```text
NAME           STATUS   ROLES           AGE   VERSION
lab-master-0   Ready    control-plane   48d   v1.32.13
lab-worker-0   Ready    <none>          48d   v1.32.13
lab-worker-1   Ready    <none>          48d   v1.32.13
```

### 2. AI 없이 기본 분석

K8sGPT `v0.4.33` 기준으로 `--explain` 없이 실행하면 AI backend를 사용하지 않는다.

```bash
KUBECONFIG=./kubeconfig k8sgpt analyze
```

검증 출력 첫 줄:

```text
AI Provider: AI not used; --explain not set
```

이후 감지된 문제들이 출력된다.

### 3. 특정 namespace만 분석

```bash
KUBECONFIG=./kubeconfig k8sgpt analyze \
  --namespace=default
```

### 4. 특정 리소스만 분석

```bash
KUBECONFIG=./kubeconfig k8sgpt analyze \
  --filter=Pod
```

```bash
KUBECONFIG=./kubeconfig k8sgpt analyze \
  --filter=Service
```

```bash
KUBECONFIG=./kubeconfig k8sgpt analyze \
  --filter=Ingress
```

### 5. JSON 출력

자동화에서는 JSON 출력이 중요하다.

```bash
KUBECONFIG=./kubeconfig k8sgpt analyze \
  --output=json > ./artifacts/k8sgpt-summary.json
```

`v0.4.33` 검증 결과는 아래 구조였다.

```json
{
  "provider": "",
  "errors": null,
  "status": "ProblemDetected",
  "problems": 29,
  "results": []
}
```

실제 `results` 항목은 analyzer와 클러스터 상태에 따라 달라진다. 파서는 깊은 구조에 강하게 의존하지 말고 다음 필드를 우선 사용한다.

- `status`
- `problems`
- `results[].kind`
- `results[].name`
- `results[].error[].Text`
- `results[].details`

## AI 설명 붙이기

AI 설명을 붙일 때만 `--explain`을 사용한다.

```bash
KUBECONFIG=./kubeconfig k8sgpt analyze \
  --explain
```

JSON으로 저장:

```bash
KUBECONFIG=./kubeconfig k8sgpt analyze \
  --explain \
  --output=json > ./artifacts/k8sgpt-explain.json
```

식별 정보 마스킹 시도:

```bash
KUBECONFIG=./kubeconfig k8sgpt analyze \
  --explain \
  --output=json \
  --anonymize > ./artifacts/k8sgpt-explain.json
```

주의:

```text
--anonymize는 도움이 되지만 모든 민감 정보가 완벽히 제거된다고 가정하면 안 된다.
```

운영 환경에서는 먼저 AI 없이 `analyze`만 사용하고, 외부 LLM으로 어떤 데이터가 전송되는지 별도로 검토한다.

## 실험용 깨진 Pod

K8sGPT가 문제를 감지하는지 확인하려면 일부러 실패하는 Pod를 만든다.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: broken-pod
  namespace: default
spec:
  containers:
    - name: broken
      image: not-exist-registry/not-exist-image:latest
```

적용:

```bash
kubectl apply -f broken-pod.yaml
```

확인:

```bash
kubectl get pod broken-pod
kubectl describe pod broken-pod
```

K8sGPT 실행:

```bash
KUBECONFIG=./kubeconfig k8sgpt analyze \
  --filter=Pod \
  --namespace=default
```

기대 방향:

- Pod가 정상적으로 시작되지 않음
- 이미지 Pull 실패 또는 관련 Event 확인
- 문제가 발생한 리소스 요약

## 보안 기준

`k8sgpt analyze`는 kubeconfig 권한으로 Kubernetes API를 조회한다. admin kubeconfig를 쓰면 K8sGPT도 admin 권한 범위에서 볼 수 있다.

권장 방향:

```text
PoC:
- admin.conf로 빠르게 실험 가능

반복 실험:
- namespace 범위를 줄인 kubeconfig 사용

운영 근접:
- read-only ClusterRole 또는 Role 기반 kubeconfig 사용
- 필요한 리소스만 get/list/watch 허용
```

`--explain`을 사용할 때 특히 주의해야 할 데이터:

- Pod 이름
- Namespace 이름
- Service 이름
- Event message
- Container status message
- 이미지 이름
- 내부 Registry 주소
- 에러 메시지에 포함된 경로와 환경 정보

## 권한 예시

실제 필요한 리소스는 K8sGPT analyzer와 사용 filter에 맞춰 조정한다.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: k8sgpt-readonly
rules:
  - apiGroups: [""]
    resources: ["pods", "services", "endpoints", "events", "persistentvolumeclaims", "nodes", "configmaps"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["jobs", "cronjobs"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["ingresses", "networkpolicies"]
    verbs: ["get", "list", "watch"]
```

## 도입 계획

### Phase 1: CLI 연결 확인

```bash
KUBECONFIG=./kubeconfig kubectl get nodes
KUBECONFIG=./kubeconfig k8sgpt analyze
```

완료 기준:

- `kubectl get nodes` 성공
- `k8sgpt analyze` 성공
- `AI Provider: AI not used; --explain not set` 확인

### Phase 2: 깨진 Pod로 진단 품질 확인

```bash
kubectl apply -f broken-pod.yaml

KUBECONFIG=./kubeconfig k8sgpt analyze \
  --filter=Pod \
  --namespace=default
```

완료 기준:

- `broken-pod` 관련 문제가 감지됨
- Event와 Pod 상태 기반 원인 후보가 출력됨

### Phase 3: JSON 아티팩트 저장

```bash
KUBECONFIG=./kubeconfig k8sgpt analyze \
  --output=json > ./artifacts/k8sgpt-summary.json
```

완료 기준:

- JSON 파일 생성
- 실패 리포트에 첨부 가능
- 사람이 열어봤을 때 문제 요약 확인 가능

### Phase 4: kube-slint/nightly-agent 연결

```text
테스트 실패
  ↓
k8sgpt analyze --output=json
  ↓
k8sgpt-summary.json 저장
  ↓
morning-summary에 첨부
```

### Phase 5: AI explain 실험

테스트 클러스터에서만 품질을 확인한다.

```bash
KUBECONFIG=./kubeconfig k8sgpt analyze \
  --explain \
  --anonymize \
  --output=json > ./artifacts/k8sgpt-explain.json
```

완료 기준:

- 설명이 실제 디버깅에 도움이 되는지 확인
- 외부로 나갈 수 있는 데이터 검토
- 운영 적용 여부는 별도 판단

### Phase 6: Operator 검토

Operator는 바로 하지 않는다. 지속 진단과 운영 도구 연동이 필요할 때 검토한다.

## 최종 권장안

현재는 아래 순서로 진행한다.

```text
1. 최신 K8sGPT CLI 설치
2. VM kubeconfig 확보
3. kubectl get nodes로 연결 확인
4. k8sgpt analyze 실행
5. 깨진 Pod로 진단 결과 확인
6. --output=json으로 결과 저장
7. kube-slint 실패 시 k8sgpt-summary.json 생성
8. nightly-agent morning-summary에 첨부
9. --explain은 테스트 환경에서만 검토
10. Operator는 지속 진단이 필요할 때 검토
```

한 줄 정리:

```text
K8sGPT는 infra-lab Kubernetes에 CLI로 붙일 수 있다.
현재는 Operator보다 CLI로 노트북/CI/nightly-agent에서 VM 클러스터를 진단하는 방식이 가장 안전하고 현실적이다.
```
