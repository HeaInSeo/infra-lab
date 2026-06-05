// Package lab provides the core logic for reading infra-lab state:
// environment metadata, VM information, build provenance, and kubectl access.
// It does not own any state — source of truth is tofu state, VMs, and K8s API.
package lab

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

// FindRoot locates the infra-lab repository root by walking up from cwd.
// Checks INFRA_LAB_ROOT env var first, then looks for scripts/k8s-tool.sh.
func FindRoot() (string, error) {
	if r := os.Getenv("INFRA_LAB_ROOT"); r != "" {
		return r, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "k8s-tool.sh")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("not inside an infra-lab repository; set INFRA_LAB_ROOT to override")
}

// Env represents a managed infra-lab environment backed by state/<name>/.
type Env struct {
	Name       string
	Root       string
	Backend    string
	CNI        string
	NamePrefix string
	GitCommit  string
	GitBranch  string
	CreatedAt  string
	Kubeconfig string
	StateFile  string
}

// VMInfo holds status information for a single VM.
type VMInfo struct {
	Name    string
	State   string
	IPv4    string
	Managed bool   // true if the VM belongs to a known infra-lab env
	EnvName string // env name if Managed, otherwise empty
}

// BuildInfo mirrors /etc/infra-lab/build.json written by write-build-json.sh.
type BuildInfo struct {
	SchemaVersion     string `json:"schemaVersion"`
	InfraLabGitCommit string `json:"infraLabGitCommit"`
	InfraLabGitBranch string `json:"infraLabGitBranch"`
	EnvName           string `json:"envName"`
	Backend           string `json:"backend"`
	CNI               string `json:"cni"`
	Role              string `json:"role"`
	NodeName          string `json:"nodeName"`
	KubernetesVersion string `json:"kubernetesVersion"`
	CreatedAt         string `json:"createdAt"`
}

// ListEnvs returns all environments found in <root>/state/.
func ListEnvs(root string) ([]*Env, error) {
	stateDir := filepath.Join(root, "state")
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var envs []*Env
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		env, err := LoadEnv(root, e.Name())
		if err != nil {
			continue // skip dirs without a valid meta file
		}
		envs = append(envs, env)
	}
	return envs, nil
}

// LoadEnv reads a single environment from state/<name>/meta.
func LoadEnv(root, name string) (*Env, error) {
	stateDir := filepath.Join(root, "state", name)
	m, err := readMeta(filepath.Join(stateDir, "meta"))
	if err != nil {
		return nil, fmt.Errorf("env %q: %w", name, err)
	}
	prefix := m["name_prefix"]
	if prefix == "" {
		prefix = "lab"
	}
	return &Env{
		Name:       name,
		Root:       root,
		Backend:    m["backend"],
		CNI:        m["cni"],
		NamePrefix: prefix,
		GitCommit:  m["infra_lab_git_commit"],
		GitBranch:  m["infra_lab_git_branch"],
		CreatedAt:  m["created_at"],
		Kubeconfig: filepath.Join(stateDir, "kubeconfig"),
		StateFile:  filepath.Join(stateDir, "terraform.tfstate"),
	}, nil
}

// FindEnvForVM finds which environment a VM belongs to by matching its name prefix.
// Falls back to a minimal multipass env so basic operations still work without state/.
func FindEnvForVM(root, vmName string) (*Env, error) {
	envs, err := ListEnvs(root)
	if err != nil {
		return nil, err
	}
	for _, e := range envs {
		if strings.HasPrefix(vmName, e.NamePrefix+"-") {
			return e, nil
		}
	}
	// No matching env: return a minimal env for direct multipass access.
	return &Env{Root: root, Backend: "multipass", NamePrefix: "lab"}, nil
}

// ListVMs returns VMs belonging to this environment.
func (e *Env) ListVMs() ([]VMInfo, error) {
	switch e.Backend {
	case "multipass":
		return listMultipassVMs(e.NamePrefix)
	default:
		return nil, fmt.Errorf("vm list not implemented for backend %q", e.Backend)
	}
}

// ReadBuildJSON reads /etc/infra-lab/build.json from a VM.
func (e *Env) ReadBuildJSON(vmName string) (*BuildInfo, error) {
	switch e.Backend {
	case "multipass":
		return readBuildJSONMultipass(vmName)
	default:
		return nil, fmt.Errorf("vm version not implemented for backend %q", e.Backend)
	}
}

// SSH opens an interactive shell on a VM.
func (e *Env) SSH(vmName string) error {
	switch e.Backend {
	case "multipass":
		cmd := exec.Command("multipass", "shell", vmName)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("vm ssh not implemented for backend %q", e.Backend)
	}
}

// PrintEnvStatus shows status for one or all environments.
func PrintEnvStatus(root, envName string) error {
	var targets []*Env
	if envName != "" {
		e, err := LoadEnv(root, envName)
		if err != nil {
			return err
		}
		targets = []*Env{e}
	} else {
		var err error
		targets, err = ListEnvs(root)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			fmt.Println("no environments found in state/")
			fmt.Println("hint: run 'HOST_PROFILE=envs/<name>.env make env-up' to create one")
			return nil
		}
	}
	for _, e := range targets {
		fmt.Printf("=== %s  backend=%s  cni=%s ===\n", e.Name, e.Backend, e.CNI)
		if _, err := os.Stat(e.Kubeconfig); err == nil {
			_ = runKubectl(e.Kubeconfig, "get", "nodes", "-o", "wide")
		} else if e.Backend == "multipass" {
			vms, err := listMultipassVMs(e.NamePrefix)
			if err == nil {
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tSTATE\tIPv4")
				for _, v := range vms {
					fmt.Fprintf(w, "%s\t%s\t%s\n", v.Name, v.State, v.IPv4)
				}
				_ = w.Flush()
			}
		}
		fmt.Println()
	}
	return nil
}

// PrintK8sStatus shows kubectl node and pod status.
func PrintK8sStatus(root, envName string) error {
	kubeconfig, err := resolveKubeconfig(root, envName)
	if err != nil {
		return err
	}
	fmt.Println("== Nodes ==")
	if err := runKubectl(kubeconfig, "get", "nodes", "-o", "wide"); err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("== Pods ==")
	return runKubectl(kubeconfig, "get", "pods", "-A", "-o", "wide")
}

// DetectLegacyFiles returns paths of pre-Phase-2 state files found in root.
func DetectLegacyFiles(root string) []string {
	candidates := []string{
		"kubeconfig", "kubeconfig.libvirt",
		"terraform.tfstate", "tofu.tfstate",
	}
	var found []string
	for _, f := range candidates {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			found = append(found, "./"+f)
		}
	}
	return found
}

// ListAllVMs returns every VM from the backend, annotated with managed status.
// Currently supports multipass only; other backends return an empty slice.
func ListAllVMs(root string) ([]VMInfo, error) {
	envs, _ := ListEnvs(root) // ignore error — treat as no envs

	vms, err := listMultipassVMs("") // empty prefix = no filtering
	if err != nil {
		return nil, err
	}
	for i, vm := range vms {
		for _, e := range envs {
			if strings.HasPrefix(vm.Name, e.NamePrefix+"-") {
				vms[i].Managed = true
				vms[i].EnvName = e.Name
				break
			}
		}
	}
	return vms, nil
}

// Print writes formatted build info as a key-value table.
func (b *BuildInfo) Print(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Node:\t%s\n", b.NodeName)
	fmt.Fprintf(tw, "Role:\t%s\n", b.Role)
	fmt.Fprintf(tw, "Kubernetes:\t%s\n", b.KubernetesVersion)
	fmt.Fprintf(tw, "CNI:\t%s\n", b.CNI)
	fmt.Fprintf(tw, "Backend:\t%s\n", b.Backend)
	fmt.Fprintf(tw, "Env:\t%s\n", b.EnvName)
	fmt.Fprintf(tw, "infra-lab branch:\t%s\n", b.InfraLabGitBranch)
	fmt.Fprintf(tw, "infra-lab commit:\t%s\n", b.InfraLabGitCommit)
	fmt.Fprintf(tw, "Created:\t%s\n", b.CreatedAt)
	_ = tw.Flush()
}

// --- internal helpers ---

func readMeta(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		m[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return m, sc.Err()
}

type multipassListOutput struct {
	List []struct {
		Name  string   `json:"name"`
		State string   `json:"state"`
		IPv4  []string `json:"ipv4"`
	} `json:"list"`
}

func listMultipassVMs(prefix string) ([]VMInfo, error) {
	out, err := exec.Command("multipass", "list", "--format", "json").Output()
	if err != nil {
		return nil, fmt.Errorf("multipass list: %w", err)
	}
	var raw multipassListOutput
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse multipass list output: %w", err)
	}
	var vms []VMInfo
	for _, v := range raw.List {
		if prefix != "" && !strings.HasPrefix(v.Name, prefix+"-") {
			continue
		}
		ip := ""
		if len(v.IPv4) > 0 {
			ip = v.IPv4[0]
		}
		vms = append(vms, VMInfo{Name: v.Name, State: v.State, IPv4: ip})
	}
	return vms, nil
}

func readBuildJSONMultipass(vmName string) (*BuildInfo, error) {
	out, err := exec.Command(
		"multipass", "exec", vmName, "--", "cat", "/etc/infra-lab/build.json",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("VM %q: cannot read build.json — was it created with infra-lab env-up?", vmName)
	}
	var info BuildInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("parse build.json from %q: %w", vmName, err)
	}
	return &info, nil
}

func runKubectl(kubeconfig string, args ...string) error {
	a := append([]string{"--kubeconfig", kubeconfig}, args...)
	cmd := exec.Command("kubectl", a...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func resolveKubeconfig(root, envName string) (string, error) {
	if envName != "" {
		e, err := LoadEnv(root, envName)
		if err != nil {
			return "", err
		}
		return e.Kubeconfig, nil
	}
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return kc, nil
	}
	envs, err := ListEnvs(root)
	if err != nil {
		return "", err
	}
	for _, e := range envs {
		if _, err := os.Stat(e.Kubeconfig); err == nil {
			return e.Kubeconfig, nil
		}
	}
	return "", fmt.Errorf("no kubeconfig found; specify env with 'ilab k8s status <env-name>'")
}
