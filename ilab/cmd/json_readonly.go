package cmd

import (
	"os"
	"path/filepath"

	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
	"github.com/HeaInSeo/infra-lab/ilab/internal/output"
)

type conditionData struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message,omitempty"`
}

type envListItemData struct {
	Name      string `json:"name"`
	Source    string `json:"source"`
	StateDir  string `json:"stateDir"`
	Backend   string `json:"backend"`
	CNI       string `json:"cni"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt,omitempty"`
	GitCommit string `json:"gitCommit,omitempty"`
	// Stale is true when state/<name>/ exists but terraform reports zero
	// resources for it — the env was likely destroyed without cleanup.
	Stale bool `json:"stale"`
}

type envListData struct {
	Envs []envListItemData `json:"envs"`
}

type envStatusData struct {
	Env        string          `json:"env"`
	StateDir   string          `json:"stateDir"`
	Profile    *profileRefData `json:"profile,omitempty"`
	Backend    string          `json:"backend"`
	CNI        string          `json:"cni"`
	Status     string          `json:"status"`
	Conditions []conditionData `json:"conditions"`
}

type profileRefData struct {
	Name   string `json:"name"`
	Source string `json:"source,omitempty"`
	Path   string `json:"path,omitempty"`
}

type profileListItemData struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

type profileListData struct {
	Profiles []profileListItemData `json:"profiles"`
}

type profileValidateData struct {
	Profile    profileRefData        `json:"profile"`
	Valid      bool                  `json:"valid"`
	Normalized profileNormalizedData `json:"normalized"`
	Conditions []conditionData       `json:"conditions"`
}

type profileShowData struct {
	Profile profileRefData        `json:"profile"`
	Spec    profileNormalizedData `json:"spec"`
}

type profileNormalizedData struct {
	Backend  string `json:"backend"`
	CNI      string `json:"cni"`
	Masters  int    `json:"masters"`
	Workers  int    `json:"workers"`
	OSImage  string `json:"osImage"`
	StateDir string `json:"stateDir"`
}

type doctorData struct {
	Root          string              `json:"root"`
	Prerequisites []doctorPrereqData  `json:"prerequisites"`
	Envs          []envListItemData   `json:"envs"`
	LegacyFiles   []string            `json:"legacyFiles"`
	VMs           []doctorVMData      `json:"vms"`
	Health        doctorHealthData    `json:"health"`
	Findings      []doctorFindingData `json:"findings"`
}

type doctorPrereqData struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Scope    string `json:"scope"`
	Required bool   `json:"required"`
	Found    bool   `json:"found"`
	Path     string `json:"path,omitempty"`
}

type doctorVMData struct {
	Name    string `json:"name"`
	Managed bool   `json:"managed"`
	Env     string `json:"env,omitempty"`
	State   string `json:"state"`
	IPv4    string `json:"ipv4,omitempty"`
}

type doctorHealthData struct {
	Risk    string `json:"risk"`
	Summary string `json:"summary"`
}

type doctorFindingData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type k8sStatusData struct {
	Env        string              `json:"env,omitempty"`
	Kubeconfig string              `json:"kubeconfig"`
	Cluster    k8sClusterData      `json:"cluster"`
	Nodes      []k8sNodeData       `json:"nodes"`
	Pods       []k8sPodData        `json:"pods"`
	Health     doctorHealthData    `json:"health"`
	Findings   []doctorFindingData `json:"findings"`
}

type k8sClusterData struct {
	Reachable    bool     `json:"reachable"`
	NodesReady   int      `json:"nodesReady"`
	PodsNotReady []string `json:"podsNotReady"`
}

type k8sNodeData struct {
	Name              string   `json:"name"`
	Ready             bool     `json:"ready"`
	Roles             []string `json:"roles"`
	KubernetesVersion string   `json:"kubernetesVersion,omitempty"`
}

type k8sPodData struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Phase     string `json:"phase"`
	Ready     bool   `json:"ready"`
	NodeName  string `json:"nodeName,omitempty"`
}

type vmListData struct {
	VMs []vmData `json:"vms"`
}

type vmData struct {
	Name    string `json:"name"`
	Managed bool   `json:"managed"`
	Env     string `json:"env,omitempty"`
	Backend string `json:"backend,omitempty"`
	State   string `json:"state"`
	IPv4    string `json:"ipv4,omitempty"`
}

type vmVersionData struct {
	VM    string         `json:"vm"`
	Build *lab.BuildInfo `json:"build"`
}

func envListPayload(envs []*lab.Env) envListData {
	items := make([]envListItemData, 0, len(envs))
	for _, env := range envs {
		count, err := env.TerraformResourceCount()
		items = append(items, envListItemData{
			Name:      env.Name,
			Source:    "state",
			StateDir:  filepath.Join("state", env.Name),
			Backend:   env.Backend,
			CNI:       env.CNI,
			Status:    "present",
			CreatedAt: env.CreatedAt,
			GitCommit: env.GitCommit,
			Stale:     err == nil && count == 0,
		})
	}
	return envListData{Envs: items}
}

func envStatusPayload(root string, env *lab.Env) envStatusData {
	stateDir := filepath.Join(root, "state", env.Name)
	resolvedProfilePath := filepath.Join(stateDir, "resolved-profile.yaml")
	conditions := []conditionData{
		fileCondition("StateDirPresent", stateDir),
		fileCondition("KubeconfigPresent", env.Kubeconfig),
		fileCondition("StateFilePresent", env.StateFile),
		fileCondition("ResolvedProfilePresent", resolvedProfilePath),
	}
	status := "ok"
	for _, condition := range conditions {
		if condition.Status != "True" {
			status = "degraded"
			break
		}
	}

	return envStatusData{
		Env:      env.Name,
		StateDir: filepath.Join("state", env.Name),
		Profile: &profileRefData{
			Name: env.Name,
			Path: resolvedProfilePath,
		},
		Backend:    env.Backend,
		CNI:        env.CNI,
		Status:     status,
		Conditions: conditions,
	}
}

func fileCondition(conditionType, path string) conditionData {
	if _, err := os.Stat(path); err == nil {
		return conditionData{Type: conditionType, Status: "True", Reason: "Found"}
	}
	return conditionData{Type: conditionType, Status: "False", Reason: "NotFound"}
}

func profileRef(arg string, p *lab.Profile) profileRefData {
	location, err := lab.ResolveProfileLocation(arg)
	if err == nil {
		return profileRefData{
			Name:   p.Name,
			Source: location.Source,
			Path:   location.Path,
		}
	}
	return profileRefData{
		Name:   p.Name,
		Source: profileSource(arg),
		Path:   arg,
	}
}

func profileSource(path string) string {
	switch {
	case filepath.IsAbs(path):
		return "explicit"
	case filepath.Dir(path) != ".":
		return "explicit"
	default:
		return "unknown"
	}
}

func profileNormalized(p *lab.Profile) profileNormalizedData {
	return profileNormalizedData{
		Backend:  p.Backend,
		CNI:      p.Kubernetes.CNI,
		Masters:  p.VM.Masters,
		Workers:  p.VM.Workers,
		OSImage:  p.VM.OSImage,
		StateDir: p.State.Dir,
	}
}

func contractErrors(code string, messages []string) []output.ErrorInfo {
	errors := make([]output.ErrorInfo, 0, len(messages))
	for _, message := range messages {
		errors = append(errors, output.ErrorInfo{Code: code, Message: message})
	}
	return errors
}
