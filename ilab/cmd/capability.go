package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
	"github.com/HeaInSeo/infra-lab/ilab/internal/output"
)

var capabilityCmd = &cobra.Command{
	Use:   "capability",
	Short: "Inspect cluster/node capabilities required by external consumers",
}

var capabilityListCmd = &cobra.Command{
	Use:   "list",
	Short: "List known capability checks",
	Args:  cobra.NoArgs,
	RunE:  runCapabilityList,
}

var capabilityCheckCmd = &cobra.Command{
	Use:   "check <capability> [env-name]",
	Short: "Check whether the cluster satisfies a named capability",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runCapabilityCheck,
}

func init() {
	capabilityCmd.AddCommand(capabilityListCmd)
	capabilityCmd.AddCommand(capabilityCheckCmd)
}

// knownCapabilities is the registry of capability checks ilab can run.
// NodeVault's in-Pod Buildah path (issue #36) is the first consumer driving
// this, but each entry describes a cluster/node capability, not a specific
// consumer application, so other consumers or environments can reuse it.
var knownCapabilities = []capabilityDescriptorData{
	{
		Name:      "buildah-userns",
		Consumers: []string{"nodevault"},
		Summary:   "in-Pod Buildah build under a Kubernetes User Namespace (pod.spec.hostUsers: false)",
	},
}

func capabilityDescriptor(name string) capabilityDescriptorData {
	for _, c := range knownCapabilities {
		if c.Name == name {
			return c
		}
	}
	return capabilityDescriptorData{Name: name}
}

func runCapabilityList(_ *cobra.Command, _ []string) error {
	data := capabilityListData{Capabilities: knownCapabilities}
	if wantsJSON() {
		return output.WriteJSON(os.Stdout, output.Success("capability.list", data))
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCONSUMERS\tSUMMARY")
	for _, c := range data.Capabilities {
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.Name, strings.Join(c.Consumers, ","), c.Summary)
	}
	return w.Flush()
}

func runCapabilityCheck(_ *cobra.Command, args []string) error {
	name := args[0]
	envName := ""
	if len(args) > 1 {
		envName = args[1]
	}
	if name != "buildah-userns" {
		err := fmt.Errorf("unknown capability %q; run 'ilab capability list' for known capabilities", name)
		return output.WrapError("CAPABILITY_UNKNOWN", err.Error(), output.ExitUsage, err)
	}

	root, err := lab.FindRoot()
	if err != nil {
		return output.WrapError("ROOT_NOT_FOUND", err.Error(), output.ExitDomain, err)
	}

	data, err := capabilityCheckBuildahUsernsPayload(root, envName)
	if err != nil {
		return err
	}

	if wantsJSON() {
		return output.WriteJSON(os.Stdout, output.Success("capability.check", data))
	}
	printCapabilityCheck(data)
	return nil
}

func capabilityCheckBuildahUsernsPayload(root, envName string) (capabilityCheckData, error) {
	descriptor := capabilityDescriptor("buildah-userns")

	kubeconfig, err := lab.ResolveKubeconfig(root, envName)
	if err != nil {
		return capabilityCheckData{}, output.WrapError("KUBECONFIG_NOT_FOUND", err.Error(), output.ExitDomain, err)
	}

	serverVersion, err := kubectlServerVersion(kubeconfig)
	if err != nil {
		return capabilityCheckData{}, output.WrapError("CLUSTER_UNREACHABLE", err.Error(), output.ExitDomain, err)
	}
	nodes, err := kubectlNodeSystemInfo(kubeconfig)
	if err != nil {
		return capabilityCheckData{}, output.WrapError("CLUSTER_UNREACHABLE", err.Error(), output.ExitDomain, err)
	}

	checks := []capabilityCheckItem{}
	findings := []doctorFindingData{}

	// K8S_VERSION
	const minMajor, minMinor = 1, 30
	k8sCheck := capabilityCheckItem{Name: "K8S_VERSION", Stage: "static"}
	major, minor, ok := parseK8sVersion(serverVersion)
	switch {
	case !ok:
		k8sCheck.Result = "UNKNOWN"
		k8sCheck.Detail = fmt.Sprintf("could not parse server version %q", serverVersion)
	case major > minMajor || (major == minMajor && minor >= minMinor):
		k8sCheck.Result = "PASS"
		k8sCheck.Detail = fmt.Sprintf("server %s >= v%d.%d (hostUsers baseline)", serverVersion, minMajor, minMinor)
	default:
		k8sCheck.Result = "FAIL"
		k8sCheck.Detail = fmt.Sprintf("server %s < v%d.%d (hostUsers requires a newer control plane)", serverVersion, minMajor, minMinor)
	}
	checks = append(checks, k8sCheck)

	// NODE_RUNTIME (kernel/containerd via kubectl, OCI runtime via SSH best-effort)
	nodeData := make([]capabilityNodeData, 0, len(nodes))
	var runtimeUnknown, runtimeMismatch, containerdBelowMin []string
	for _, n := range nodes {
		ociName, ociVersion, ok := lab.DiscoverNodeOCIRuntime(root, n.name)
		switch {
		case !ok:
			runtimeUnknown = append(runtimeUnknown, n.name)
		case ociName != "crun":
			runtimeMismatch = append(runtimeMismatch, n.name)
		}
		if !containerdAtLeast(n.containerRuntimeVersion, 2) {
			containerdBelowMin = append(containerdBelowMin, n.name)
		}
		nodeData = append(nodeData, capabilityNodeData{
			Name:                    n.name,
			KernelVersion:           n.kernelVersion,
			ContainerRuntimeVersion: n.containerRuntimeVersion,
			OCIRuntime:              ociName,
			OCIRuntimeVersion:       ociVersion,
		})
	}

	nodeCheck := capabilityCheckItem{Name: "NODE_RUNTIME", Stage: "static"}
	switch {
	case len(containerdBelowMin) > 0:
		nodeCheck.Result = "FAIL"
		nodeCheck.Detail = fmt.Sprintf("containerd < 2.0 on node(s): %s", strings.Join(containerdBelowMin, ", "))
	case len(runtimeMismatch) > 0:
		nodeCheck.Result = "FAIL"
		nodeCheck.Detail = fmt.Sprintf("OCI runtime is not crun on node(s): %s", strings.Join(runtimeMismatch, ", "))
	case len(runtimeUnknown) > 0:
		nodeCheck.Result = "UNKNOWN"
		nodeCheck.Detail = fmt.Sprintf("could not verify OCI runtime over SSH on node(s): %s", strings.Join(runtimeUnknown, ", "))
		findings = append(findings, doctorFindingData{
			Code:    "NODE_OCI_RUNTIME_NOT_VERIFIED",
			Message: fmt.Sprintf("SSH-based OCI runtime check could not reach: %s", strings.Join(runtimeUnknown, ", ")),
		})
	default:
		nodeCheck.Result = "PASS"
		nodeCheck.Detail = "all nodes report containerd >= 2.0 with crun as the OCI runtime"
	}
	checks = append(checks, nodeCheck)

	// HOST_USERS_API
	hostUsersSupported, err := kubectlHostUsersFieldSupported(kubeconfig)
	huCheck := capabilityCheckItem{Name: "HOST_USERS_API", Stage: "static"}
	switch {
	case err != nil:
		huCheck.Result = "UNKNOWN"
		huCheck.Detail = fmt.Sprintf("kubectl explain pod.spec.hostUsers failed: %v", err)
	case hostUsersSupported:
		huCheck.Result = "PASS"
		huCheck.Detail = "pod.spec.hostUsers is present in the API schema"
	default:
		huCheck.Result = "FAIL"
		huCheck.Detail = "pod.spec.hostUsers is not present in the API schema"
	}
	checks = append(checks, huCheck)

	// STORAGECLASS
	defaultSC, allSC, err := kubectlStorageClasses(kubeconfig)
	scCheck := capabilityCheckItem{Name: "STORAGECLASS", Stage: "static"}
	switch {
	case err != nil:
		scCheck.Result = "UNKNOWN"
		scCheck.Detail = err.Error()
	case len(allSC) == 0:
		scCheck.Result = "FAIL"
		scCheck.Detail = "no StorageClass found"
	case defaultSC == "":
		scCheck.Result = "UNKNOWN"
		scCheck.Detail = fmt.Sprintf("no default StorageClass among: %s", strings.Join(allSC, ", "))
	default:
		scCheck.Result = "PASS"
		scCheck.Detail = fmt.Sprintf("default StorageClass %q present", defaultSC)
	}
	checks = append(checks, scCheck)

	// Smoke stage: disposable, self-cleaning Pods in the live cluster.
	usernsSmoke := runUserNamespaceSmokeTest(kubeconfig)
	checks = append(checks, capabilityCheckItem{
		Name: "USERNS_SMOKE_TEST", Stage: "smoke",
		Result: usernsSmoke.Result, Detail: usernsSmoke.Detail,
	})
	if usernsSmoke.Result == "PASS" {
		buildahSmoke := runBuildahSmokeTest(kubeconfig)
		checks = append(checks, capabilityCheckItem{
			Name: "BUILDAH_SMOKE_TEST", Stage: "smoke",
			Result: buildahSmoke.Result, Detail: buildahSmoke.Detail,
		})
		if buildahSmoke.Result == "FAIL" && strings.Contains(buildahSmoke.Detail, "mount namespace") {
			findings = append(findings, doctorFindingData{
				Code:    "BUILDAH_USERNS_MOUNT_INCOMPATIBLE",
				Message: "in-Pod Buildah cannot privatize its own mount namespace under hostUsers:false on this cluster — see BUILDAH_SMOKE_TEST detail for the full explanation",
			})
		}
	} else {
		checks = append(checks, capabilityCheckItem{
			Name: "BUILDAH_SMOKE_TEST", Stage: "smoke", Result: "SKIPPED",
			Detail: "skipped because USERNS_SMOKE_TEST did not pass",
		})
	}

	status, risk, summary := capabilityStatus(checks)

	return capabilityCheckData{
		Capability: descriptor.Name,
		Consumers:  descriptor.Consumers,
		Env:        envName,
		Kubeconfig: kubeconfig,
		Status:     status,
		Health:     doctorHealthData{Risk: risk, Summary: summary},
		Cluster: capabilityClusterData{
			Reachable:               true,
			ServerVersion:           serverVersion,
			HostUsersFieldSupported: hostUsersSupported,
		},
		Nodes:    nodeData,
		Storage:  capabilityStorageData{DefaultStorageClass: defaultSC, StorageClasses: allSC},
		Checks:   checks,
		Findings: findings,
	}, nil
}

// capabilityStatus derives the issue-#36 four-state verdict from the
// individual checks. NOT_IMPLEMENTED/SKIPPED both mean "couldn't reach a
// verdict for this check" — the former because this ilab build doesn't run
// that check yet, the latter because a prerequisite check didn't PASS — so
// both cap the result below SUPPORTED the same way UNKNOWN does.
func capabilityStatus(checks []capabilityCheckItem) (status, risk, summary string) {
	var hasFail, hasUnknown bool
	for _, c := range checks {
		switch c.Result {
		case "FAIL":
			hasFail = true
		case "UNKNOWN", "NOT_IMPLEMENTED", "SKIPPED":
			hasUnknown = true
		}
	}
	switch {
	case hasFail:
		return "UNSUPPORTED", "HIGH", "one or more required checks failed"
	case hasUnknown:
		return "NOT_VERIFIED", "MEDIUM", "one or more checks could not be verified"
	default:
		return "SUPPORTED", "LOW", "all static and smoke checks passed"
	}
}

func printCapabilityCheck(data capabilityCheckData) {
	fmt.Printf("Capability: %s (consumers: %s)\n", data.Capability, strings.Join(data.Consumers, ", "))
	fmt.Printf("Kubeconfig: %s\n", data.Kubeconfig)
	fmt.Printf("Status: %s (risk: %s)\n", data.Status, data.Health.Risk)
	fmt.Println(data.Health.Summary)
	fmt.Println()

	fmt.Println("Checks:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  NAME\tSTAGE\tRESULT\tDETAIL")
	for _, c := range data.Checks {
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", c.Name, c.Stage, c.Result, c.Detail)
	}
	_ = w.Flush()
	fmt.Println()

	if len(data.Nodes) > 0 {
		fmt.Println("Nodes:")
		nw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(nw, "  NAME\tKERNEL\tCONTAINER RUNTIME\tOCI RUNTIME")
		for _, n := range data.Nodes {
			oci := "unknown"
			switch {
			case n.OCIRuntime != "" && n.OCIRuntimeVersion != "":
				oci = fmt.Sprintf("%s %s", n.OCIRuntime, n.OCIRuntimeVersion)
			case n.OCIRuntime != "":
				oci = n.OCIRuntime
			}
			fmt.Fprintf(nw, "  %s\t%s\t%s\t%s\n", n.Name, n.KernelVersion, n.ContainerRuntimeVersion, oci)
		}
		_ = nw.Flush()
		fmt.Println()
	}

	if len(data.Findings) > 0 {
		fmt.Println("Findings:")
		for _, f := range data.Findings {
			fmt.Printf("  - [%s] %s\n", f.Code, f.Message)
		}
	}
}

type kubectlVersionInfo struct {
	ServerVersion struct {
		GitVersion string `json:"gitVersion"`
	} `json:"serverVersion"`
}

func kubectlServerVersion(kubeconfig string) (string, error) {
	raw, err := runKubectlJSON(kubeconfig, "version", "-o", "json")
	if err != nil {
		return "", err
	}
	var v kubectlVersionInfo
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	return v.ServerVersion.GitVersion, nil
}

var k8sVersionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)`)

func parseK8sVersion(gitVersion string) (major, minor int, ok bool) {
	m := k8sVersionPattern.FindStringSubmatch(gitVersion)
	if m == nil {
		return 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	return major, minor, true
}

type capabilityNodeInfo struct {
	name                    string
	kernelVersion           string
	containerRuntimeVersion string
}

type kubectlNodeSystemInfoList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Status struct {
			NodeInfo struct {
				KernelVersion           string `json:"kernelVersion"`
				ContainerRuntimeVersion string `json:"containerRuntimeVersion"`
			} `json:"nodeInfo"`
		} `json:"status"`
	} `json:"items"`
}

func kubectlNodeSystemInfo(kubeconfig string) ([]capabilityNodeInfo, error) {
	raw, err := runKubectlJSON(kubeconfig, "get", "nodes", "-o", "json")
	if err != nil {
		return nil, err
	}
	var list kubectlNodeSystemInfoList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	nodes := make([]capabilityNodeInfo, 0, len(list.Items))
	for _, item := range list.Items {
		nodes = append(nodes, capabilityNodeInfo{
			name:                    item.Metadata.Name,
			kernelVersion:           item.Status.NodeInfo.KernelVersion,
			containerRuntimeVersion: item.Status.NodeInfo.ContainerRuntimeVersion,
		})
	}
	return nodes, nil
}

var containerdVersionPattern = regexp.MustCompile(`^containerd://v?(\d+)`)

// containerdAtLeast reports whether containerRuntimeVersion (e.g.
// "containerd://2.0.1") is at least minMajor. Runtimes it doesn't recognize
// (including non-containerd runtimes) pass through untouched — NODE_RUNTIME
// degrades via the OCI-runtime-mismatch/unknown path for those instead.
func containerdAtLeast(containerRuntimeVersion string, minMajor int) bool {
	m := containerdVersionPattern.FindStringSubmatch(containerRuntimeVersion)
	if m == nil {
		return true
	}
	major, _ := strconv.Atoi(m[1])
	return major >= minMajor
}

func kubectlHostUsersFieldSupported(kubeconfig string) (bool, error) {
	raw, err := runKubectlJSON(kubeconfig, "explain", "pod.spec.hostUsers")
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(string(raw)), "hostusers"), nil
}

type kubectlStorageClassList struct {
	Items []struct {
		Metadata struct {
			Name        string            `json:"name"`
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
	} `json:"items"`
}

func kubectlStorageClasses(kubeconfig string) (defaultClass string, all []string, err error) {
	raw, err := runKubectlJSON(kubeconfig, "get", "storageclass", "-o", "json")
	if err != nil {
		return "", nil, err
	}
	var list kubectlStorageClassList
	if err := json.Unmarshal(raw, &list); err != nil {
		return "", nil, err
	}
	all = make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		all = append(all, item.Metadata.Name)
		if item.Metadata.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			defaultClass = item.Metadata.Name
		}
	}
	sort.Strings(all)
	return defaultClass, all, nil
}

// smokeResult is the outcome of a single disposable-resource smoke test.
type smokeResult struct {
	Result string // PASS | FAIL | UNKNOWN
	Detail string
}

const smokeTimeout = 90 * time.Second

// runUserNamespaceSmokeTest creates a disposable hostUsers:false Pod, checks
// that it starts, runs as a remapped (non-host) UID/GID, and can write to an
// emptyDir volume — then removes the namespace it created, success or
// failure, so no smoke-test state survives the check.
func runUserNamespaceSmokeTest(kubeconfig string) smokeResult {
	ns := smokeNamespaceName("userns")
	const pod = "userns-smoke"
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
  labels:
    app.kubernetes.io/managed-by: ilab-capability-check
---
apiVersion: v1
kind: Pod
metadata:
  name: %[2]s
  namespace: %[1]s
  labels:
    app.kubernetes.io/managed-by: ilab-capability-check
spec:
  hostUsers: false
  restartPolicy: Never
  containers:
  - name: smoke
    image: busybox:1.36
    command: ["sh", "-c", "id && echo ilab-smoke-marker > /work/test.txt && cat /work/test.txt"]
    volumeMounts:
    - name: work
      mountPath: /work
  volumes:
  - name: work
    emptyDir: {}
`, ns, pod)

	defer kubectlDeleteNamespaceAsync(kubeconfig, ns)

	if err := kubectlApplyManifest(kubeconfig, manifest); err != nil {
		return classifySmokeSetupError(err, "hostUsers:false Pod")
	}

	phase, err := waitForPodTerminal(kubeconfig, ns, pod, smokeTimeout)
	if err != nil {
		return smokeResult{Result: "UNKNOWN", Detail: err.Error()}
	}
	logs, _ := kubectlPodLogs(kubeconfig, ns, pod)
	if phase != "Succeeded" {
		return smokeResult{Result: "FAIL", Detail: fmt.Sprintf("Pod ended in phase %q (want Succeeded); logs: %s", phase, truncateOutput(logs))}
	}
	if !strings.Contains(logs, "ilab-smoke-marker") {
		return smokeResult{Result: "FAIL", Detail: "Pod succeeded but did not write/read back the expected emptyDir marker"}
	}
	return smokeResult{Result: "PASS", Detail: fmt.Sprintf("disposable hostUsers:false Pod ran, wrote to emptyDir, and exited 0 (id: %s)", firstLine(logs))}
}

// runBuildahSmokeTest creates a disposable hostUsers:false Pod that runs a
// minimal in-Pod Buildah build (vfs storage driver) and cleans up its
// namespace regardless of outcome. It intentionally requests no extra
// capabilities or seccomp changes: adding CAP_SYS_ADMIN + an unconfined
// seccomp profile was tried against the live seoy-libvirt-cilium cluster and
// made no difference (see classifyBuildahFailure) — the failure this
// reproduces is a mount-namespace-ownership restriction the kernel enforces
// regardless of the container's own capability set, so granting more
// capabilities here would just be privilege escalation with no payoff.
func runBuildahSmokeTest(kubeconfig string) smokeResult {
	ns := smokeNamespaceName("buildah")
	const pod = "buildah-smoke"
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
  labels:
    app.kubernetes.io/managed-by: ilab-capability-check
---
apiVersion: v1
kind: Pod
metadata:
  name: %[2]s
  namespace: %[1]s
  labels:
    app.kubernetes.io/managed-by: ilab-capability-check
spec:
  hostUsers: false
  restartPolicy: Never
  containers:
  - name: smoke
    image: quay.io/buildah/stable:latest
    command:
    - sh
    - -c
    - |
      set -e
      mkdir -p /tmp/build
      printf 'FROM busybox:1.36\nRUN echo ilab-smoke-build\n' > /tmp/build/Containerfile
      buildah --storage-driver vfs bud -f /tmp/build/Containerfile -t ilab-smoke:test /tmp/build
`, ns, pod)

	defer kubectlDeleteNamespaceAsync(kubeconfig, ns)

	if err := kubectlApplyManifest(kubeconfig, manifest); err != nil {
		return classifySmokeSetupError(err, "Buildah smoke Pod")
	}

	phase, err := waitForPodTerminal(kubeconfig, ns, pod, smokeTimeout)
	if err != nil {
		return smokeResult{Result: "UNKNOWN", Detail: err.Error()}
	}
	logs, _ := kubectlPodLogs(kubeconfig, ns, pod)
	if phase != "Succeeded" {
		return smokeResult{Result: "FAIL", Detail: classifyBuildahFailure(logs)}
	}
	return smokeResult{Result: "PASS", Detail: "disposable in-Pod Buildah build (FROM busybox, vfs storage driver) completed successfully"}
}

// classifyBuildahFailure recognizes the specific "remount /" mount-namespace-
// ownership error observed on the live seoy-libvirt-cilium cluster (crun
// 1.28 + containerd 2.2.1 + kernel 6.8): Buildah tries to privatize its own
// mount namespace to manage image layers, but the Pod's root filesystem
// mount is owned by an ancestor (host) user namespace under hostUsers:false,
// so the kernel denies the remount no matter what capabilities the
// container holds. Granting CAP_SYS_ADMIN + an unconfined seccomp profile
// was tested against this exact cluster and made no difference.
func classifyBuildahFailure(logs string) string {
	if strings.Contains(logs, "remount /") && strings.Contains(logs, "permission denied") {
		return fmt.Sprintf(
			"in-Pod Buildah cannot privatize its own mount namespace under hostUsers:false — "+
				"the Pod's root filesystem is mounted in a mount namespace owned by an ancestor "+
				"(host) user namespace, so the kernel denies Buildah's own \"remount / (MS_PRIVATE)\" "+
				"regardless of granted capabilities (CAP_SYS_ADMIN + unconfined seccomp were tried and "+
				"did not help). This looks like a fundamental incompatibility between Kubernetes User "+
				"Namespaces and Buildah's default (containers/storage) layer-application path on this "+
				"stack, not a fixable RBAC/capability gap. Raw error: %s",
			truncateOutput(logs),
		)
	}
	return fmt.Sprintf("Pod did not complete successfully; logs: %s", truncateOutput(logs))
}

func smokeNamespaceName(kind string) string {
	return fmt.Sprintf("ilab-capability-check-%s-%d", kind, time.Now().UnixNano())
}

func kubectlApplyManifest(kubeconfig, manifest string) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// kubectlDeleteNamespaceAsync fires a non-blocking delete so a slow or
// stuck cluster-side GC never makes a capability check hang — it's a
// best-effort cleanup, not a guarantee the namespace disappears instantly.
func kubectlDeleteNamespaceAsync(kubeconfig, ns string) {
	_ = exec.Command("kubectl", "--kubeconfig", kubeconfig, "delete", "namespace", ns, "--ignore-not-found", "--wait=false").Run()
}

func waitForPodTerminal(kubeconfig, ns, pod string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		out, err := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", ns, "get", "pod", pod, "-o", "jsonpath={.status.phase}").Output()
		if err == nil {
			phase := strings.TrimSpace(string(out))
			if phase == "Succeeded" || phase == "Failed" {
				return phase, nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out after %s waiting for pod %s/%s to finish", timeout, ns, pod)
		}
		time.Sleep(2 * time.Second)
	}
}

func kubectlPodLogs(kubeconfig, ns, pod string) (string, error) {
	out, err := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", ns, "logs", pod).Output()
	return string(out), err
}

// classifySmokeSetupError distinguishes "we're not allowed to test this"
// (RBAC — NOT_VERIFIED via UNKNOWN) from "the cluster rejected the thing
// we're testing" (admission/spec — FAIL) so the verdict matches the cause.
func classifySmokeSetupError(err error, what string) smokeResult {
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "forbidden") {
		return smokeResult{Result: "UNKNOWN", Detail: fmt.Sprintf("insufficient RBAC permissions to run the %s: %s", what, msg)}
	}
	return smokeResult{Result: "FAIL", Detail: fmt.Sprintf("%s was rejected: %s", what, msg)}
}

func truncateOutput(s string) string {
	s = strings.TrimSpace(s)
	const max = 400
	if len(s) > max {
		return s[:max] + "...(truncated)"
	}
	return s
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}
