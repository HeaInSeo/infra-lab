package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

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

	// Smoke stage: not implemented this iteration (see #36 scope note).
	checks = append(checks,
		capabilityCheckItem{
			Name: "USERNS_SMOKE_TEST", Stage: "smoke", Result: "NOT_IMPLEMENTED",
			Detail: "disposable hostUsers:false Pod smoke test scheduled for a later iteration",
		},
		capabilityCheckItem{
			Name: "BUILDAH_SMOKE_TEST", Stage: "smoke", Result: "NOT_IMPLEMENTED",
			Detail: "disposable Buildah build smoke test scheduled for a later iteration",
		},
	)

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
// individual checks. Smoke-stage checks are NOT_IMPLEMENTED this iteration,
// so SUPPORTED can never be reached yet — the best a clean cluster gets is
// DEGRADED, which is intentional: full support requires the smoke tests.
func capabilityStatus(checks []capabilityCheckItem) (status, risk, summary string) {
	var hasFail, hasUnknown, smokeIncomplete bool
	for _, c := range checks {
		switch c.Result {
		case "FAIL":
			hasFail = true
		case "UNKNOWN":
			hasUnknown = true
		case "NOT_IMPLEMENTED":
			smokeIncomplete = true
		}
	}
	switch {
	case hasFail:
		return "UNSUPPORTED", "HIGH", "one or more required static checks failed"
	case hasUnknown:
		return "NOT_VERIFIED", "MEDIUM", "one or more checks could not be verified"
	case smokeIncomplete:
		return "DEGRADED", "MEDIUM", "static discovery passed; smoke tests are not yet implemented"
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
