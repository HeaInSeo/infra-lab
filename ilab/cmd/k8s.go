package cmd

import (
	"encoding/json"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
	"github.com/HeaInSeo/infra-lab/ilab/internal/output"
)

var k8sCmd = &cobra.Command{
	Use:   "k8s",
	Short: "Kubernetes cluster operations",
}

var k8sStatusCmd = &cobra.Command{
	Use:   "status [env-name]",
	Short: "Show Kubernetes node and pod status",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runK8sStatus,
}

func init() {
	k8sCmd.AddCommand(k8sStatusCmd)
}

func runK8sStatus(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	envName := ""
	if len(args) > 0 {
		envName = args[0]
	}
	if wantsJSON() {
		data, err := k8sStatusPayload(root, envName)
		if err != nil {
			return err
		}
		return output.WriteJSON(os.Stdout, output.Success("k8s.status", data))
	}
	return lab.PrintK8sStatus(root, envName)
}

type kubectlNodeList struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Status struct {
			NodeInfo struct {
				KubeletVersion string `json:"kubeletVersion"`
			} `json:"nodeInfo"`
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	} `json:"items"`
}

type kubectlPodList struct {
	Items []struct {
		Metadata struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			NodeName string `json:"nodeName"`
		} `json:"spec"`
		Status struct {
			Phase             string `json:"phase"`
			ContainerStatuses []struct {
				Ready bool `json:"ready"`
			} `json:"containerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

func k8sStatusPayload(root, envName string) (k8sStatusData, error) {
	kubeconfig, err := lab.ResolveKubeconfig(root, envName)
	if err != nil {
		return k8sStatusData{}, output.WrapError("KUBECONFIG_NOT_FOUND", err.Error(), output.ExitDomain, err)
	}

	nodesRaw, err := runKubectlJSON(kubeconfig, "get", "nodes", "-o", "json")
	if err != nil {
		return k8sStatusData{}, output.WrapError("CLUSTER_UNREACHABLE", err.Error(), output.ExitDomain, err)
	}
	podsRaw, err := runKubectlJSON(kubeconfig, "get", "pods", "-A", "-o", "json")
	if err != nil {
		return k8sStatusData{}, output.WrapError("CLUSTER_UNREACHABLE", err.Error(), output.ExitDomain, err)
	}

	var nodeList kubectlNodeList
	if err := json.Unmarshal(nodesRaw, &nodeList); err != nil {
		return k8sStatusData{}, output.WrapError("COMMAND_FAILED", err.Error(), output.ExitRuntime, err)
	}
	var podList kubectlPodList
	if err := json.Unmarshal(podsRaw, &podList); err != nil {
		return k8sStatusData{}, output.WrapError("COMMAND_FAILED", err.Error(), output.ExitRuntime, err)
	}

	nodes := make([]k8sNodeData, 0, len(nodeList.Items))
	nodesReady := 0
	for _, item := range nodeList.Items {
		ready := false
		for _, condition := range item.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "True" {
				ready = true
				break
			}
		}
		if ready {
			nodesReady++
		}
		nodes = append(nodes, k8sNodeData{
			Name:              item.Metadata.Name,
			Ready:             ready,
			Roles:             nodeRoles(item.Metadata.Labels),
			KubernetesVersion: item.Status.NodeInfo.KubeletVersion,
		})
	}

	pods := make([]k8sPodData, 0, len(podList.Items))
	podsNotReady := []string{}
	for _, item := range podList.Items {
		ready := podReady(item.Status.Phase, item.Status.ContainerStatuses)
		key := item.Metadata.Namespace + "/" + item.Metadata.Name
		if !ready {
			podsNotReady = append(podsNotReady, key)
		}
		pods = append(pods, k8sPodData{
			Namespace: item.Metadata.Namespace,
			Name:      item.Metadata.Name,
			Phase:     item.Status.Phase,
			Ready:     ready,
			NodeName:  item.Spec.NodeName,
		})
	}

	risk := "LOW"
	summary := "Cluster is reachable"
	findings := []doctorFindingData{}
	if nodesReady != len(nodes) || len(podsNotReady) > 0 {
		risk = "MEDIUM"
		summary = "Cluster has non-ready resources"
		if nodesReady != len(nodes) {
			findings = append(findings, doctorFindingData{
				Code:    "NODES_NOT_READY",
				Message: "one or more nodes are not Ready",
			})
		}
		if len(podsNotReady) > 0 {
			findings = append(findings, doctorFindingData{
				Code:    "PODS_NOT_READY",
				Message: "one or more pods are not Ready",
			})
		}
	}

	return k8sStatusData{
		Env:        envName,
		Kubeconfig: kubeconfig,
		Cluster: k8sClusterData{
			Reachable:    true,
			NodesReady:   nodesReady,
			PodsNotReady: podsNotReady,
		},
		Nodes:    nodes,
		Pods:     pods,
		Health:   doctorHealthData{Risk: risk, Summary: summary},
		Findings: findings,
	}, nil
}

func runKubectlJSON(kubeconfig string, args ...string) ([]byte, error) {
	allArgs := append([]string{"--kubeconfig", kubeconfig}, args...)
	cmd := exec.Command("kubectl", allArgs...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func nodeRoles(labels map[string]string) []string {
	roles := []string{}
	for key := range labels {
		const prefix = "node-role.kubernetes.io/"
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			roles = append(roles, key[len(prefix):])
		}
	}
	if len(roles) == 0 {
		roles = append(roles, "worker")
	}
	return roles
}

func podReady(phase string, statuses []struct {
	Ready bool `json:"ready"`
}) bool {
	if phase == "Succeeded" {
		return true
	}
	if phase != "Running" {
		return false
	}
	if len(statuses) == 0 {
		return true
	}
	for _, status := range statuses {
		if !status.Ready {
			return false
		}
	}
	return true
}
