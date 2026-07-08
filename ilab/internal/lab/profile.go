package lab

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// osImageURLs maps canonical OS image names to cloud image download URLs.
var osImageURLs = map[string]string{
	"ubuntu-24.04": "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
	"ubuntu-22.04": "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img",
	"ubuntu-20.04": "https://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img",
}

// OSImageURL returns the cloud image URL for the given OS image name,
// or an empty string if it is not in the built-in lookup table.
func OSImageURL(osImage string) string {
	return osImageURLs[osImage]
}

// SupportedOSImages returns the sorted list of known OS image names.
func SupportedOSImages() []string {
	out := make([]string, 0, len(osImageURLs))
	for k := range osImageURLs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ImmutableConflict describes a proposed change to a field that must not change in-place.
type ImmutableConflict struct {
	Field    string // YAML path, e.g. "kubernetes.cni"
	OldValue string
	NewValue string
}

// immutableGetters maps YAML field paths to their current-value getters.
var immutableGetters = map[string]func(*Profile) string{
	"kubernetes.cni": func(p *Profile) string { return p.Kubernetes.CNI },
	"backend":        func(p *Profile) string { return p.Backend },
	"vm.osImage":     func(p *Profile) string { return p.VM.OSImage },
	"vm.masters":     func(p *Profile) string { return fmt.Sprintf("%d", p.VM.Masters) },
}

// CheckImmutableConflicts returns conflicts for any proposed changes to immutable fields.
// proposed maps field paths (e.g. "kubernetes.cni") to their desired new values.
func (p *Profile) CheckImmutableConflicts(proposed map[string]string) []ImmutableConflict {
	var conflicts []ImmutableConflict
	for field, getter := range immutableGetters {
		newVal, ok := proposed[field]
		if !ok || newVal == "" {
			continue
		}
		if oldVal := getter(p); oldVal != newVal {
			conflicts = append(conflicts, ImmutableConflict{
				Field:    field,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}
	return conflicts
}

// Validate checks the profile for required and consistent fields.
// Returns a list of human-readable error strings; empty means the profile is valid.
func (p *Profile) Validate() []string {
	var errs []string

	switch p.KindOrDefault() {
	case "kubernetes":
	case "single-vm":
	default:
		errs = append(errs, fmt.Sprintf("kind %q is not supported", p.Kind))
	}
	if p.Backend == "" {
		errs = append(errs, "backend is required")
	}
	if p.KindOrDefault() == "single-vm" {
		return p.validateSingleVM(errs)
	}
	if p.Kubernetes.CNI == "" {
		errs = append(errs, "kubernetes.cni is required")
	}
	if p.VM.Masters == 0 {
		errs = append(errs, "vm.masters must be > 0")
	}
	if p.VM.Workers == 0 {
		errs = append(errs, "vm.workers must be > 0")
	}

	if p.Backend == "libvirt" {
		if p.Libvirt == nil {
			errs = append(errs, "libvirt section is required for backend=libvirt")
		} else {
			if p.Libvirt.SSHPrivateKeyPath == "" {
				errs = append(errs, "libvirt.sshPrivateKeyPath is required")
			} else {
				expanded := ExpandTilde(p.Libvirt.SSHPrivateKeyPath)
				if _, err := os.Stat(expanded); err != nil {
					errs = append(errs, fmt.Sprintf("libvirt.sshPrivateKeyPath not found: %s", expanded))
				}
			}
			switch {
			case p.Libvirt.SSHPublicKey == "":
				errs = append(errs, "libvirt.sshPublicKey is required")
			case strings.Contains(p.Libvirt.SSHPublicKey, "AAAA..."):
				errs = append(errs, "libvirt.sshPublicKey contains placeholder — replace with the real public key")
			}
			if p.Libvirt.PoolName == "" {
				errs = append(errs, "libvirt.poolName is required")
			}
			if p.Libvirt.PoolPath == "" {
				errs = append(errs, "libvirt.poolPath is required")
			}
		}
		if p.VM.ImageURL == "" {
			errs = append(errs, "vm.imageUrl is required for backend=libvirt")
		}
	}

	return errs
}

func (p *Profile) validateSingleVM(errs []string) []string {
	if p.Backend != "libvirt" {
		errs = append(errs, "backend must be libvirt for kind=single-vm")
	}
	if p.VM.Count != 1 {
		errs = append(errs, "vm.count must be 1 for kind=single-vm")
	}
	if p.VM.CPU <= 0 {
		errs = append(errs, "vm.cpu must be > 0 for kind=single-vm")
	}
	if p.VM.Memory == "" {
		errs = append(errs, "vm.memory is required for kind=single-vm")
	}
	if p.VM.Disk == "" {
		errs = append(errs, "vm.disk is required for kind=single-vm")
	}
	if p.VM.ImageURL == "" {
		errs = append(errs, "vm.imageUrl is required for kind=single-vm")
	}
	if p.SSH.User == "" {
		errs = append(errs, "ssh.user is required for kind=single-vm")
	}
	if p.SSH.PrivateKeyPath == "" {
		errs = append(errs, "ssh.privateKeyPath is required for kind=single-vm")
	} else {
		expanded := ExpandTilde(p.SSH.PrivateKeyPath)
		if _, err := os.Stat(expanded); err != nil {
			errs = append(errs, fmt.Sprintf("ssh.privateKeyPath not found: %s", expanded))
		}
		if _, err := os.Stat(expanded + ".pub"); err != nil {
			errs = append(errs, fmt.Sprintf("ssh public key not found: %s.pub", expanded))
		}
	}
	if p.Workspace.Path == "" {
		errs = append(errs, "workspace.path is required for kind=single-vm")
	}
	if p.Libvirt == nil {
		errs = append(errs, "libvirt section is required for backend=libvirt")
	} else {
		if p.Libvirt.PoolName == "" {
			errs = append(errs, "libvirt.poolName is required")
		}
		if p.Libvirt.PoolPath == "" {
			errs = append(errs, "libvirt.poolPath is required")
		}
	}
	return errs
}

// Profile represents a YAML-based environment profile.
// It is the single source of truth for an environment's desired state.
type Profile struct {
	Kind       string         `yaml:"kind,omitempty"`
	Name       string         `yaml:"name"`
	NamePrefix string         `yaml:"namePrefix,omitempty"`
	Backend    string         `yaml:"backend"`
	VM         VMSpec         `yaml:"vm"`
	Kubernetes KubernetesSpec `yaml:"kubernetes"`
	Addons     AddonsSpec     `yaml:"addons"`
	SSH        SSHSpec        `yaml:"ssh,omitempty"`
	Workspace  WorkspaceSpec  `yaml:"workspace,omitempty"`
	Bootstrap  BootstrapSpec  `yaml:"bootstrap,omitempty"`
	Libvirt    *LibvirtSpec   `yaml:"libvirt,omitempty"`
	State      StateSpec      `yaml:"state"`
	SourcePath string         `yaml:"-"`
}

// VMSpec describes the VM resources for a profile.
type VMSpec struct {
	OSImage  string   `yaml:"osImage"`
	ImageURL string   `yaml:"imageUrl"`
	Count    int      `yaml:"count,omitempty"`
	CPU      int      `yaml:"cpu,omitempty"`
	Memory   string   `yaml:"memory,omitempty"`
	Disk     string   `yaml:"disk,omitempty"`
	Masters  int      `yaml:"masters"`
	Workers  int      `yaml:"workers"`
	Master   NodeSpec `yaml:"master"`
	Worker   NodeSpec `yaml:"worker"`
}

// NodeSpec describes CPU/memory/disk for a node type.
type NodeSpec struct {
	CPU    int    `yaml:"cpu"`
	Memory string `yaml:"memory"`
	Disk   string `yaml:"disk"`
}

// KubernetesSpec describes Kubernetes version and CNI.
type KubernetesSpec struct {
	Version string `yaml:"version"`
	CNI     string `yaml:"cni"`
}

// AddonsSpec lists base and optional addons.
type AddonsSpec struct {
	Base     []string `yaml:"base"`
	Optional []string `yaml:"optional"`
}

// SSHSpec describes guest SSH settings for VM-only profiles.
type SSHSpec struct {
	User           string `yaml:"user,omitempty"`
	PrivateKeyPath string `yaml:"privateKeyPath,omitempty"`
}

// WorkspaceSpec describes the remote workspace created for VM-only profiles.
type WorkspaceSpec struct {
	Path string   `yaml:"path,omitempty"`
	Dirs []string `yaml:"dirs,omitempty"`
}

// BootstrapSpec lists scripts copied into a VM-only profile workspace.
type BootstrapSpec struct {
	Scripts []string `yaml:"scripts,omitempty"`
}

// LibvirtSpec holds libvirt-specific settings (SSH key, pool).
type LibvirtSpec struct {
	SSHPrivateKeyPath string `yaml:"sshPrivateKeyPath"`
	SSHPublicKey      string `yaml:"sshPublicKey"`
	PoolName          string `yaml:"poolName"`
	PoolPath          string `yaml:"poolPath"`
}

// StateSpec defines where state for this environment is stored.
type StateSpec struct {
	Dir string `yaml:"dir"`
}

type ProfileLocation struct {
	Path      string
	Source    string
	IsExample bool
}

func ResolveProfileLocation(path string) (ProfileLocation, error) {
	resolved, isExample, err := resolveProfilePath(path)
	if err != nil {
		return ProfileLocation{}, err
	}
	return ProfileLocation{
		Path:      resolved,
		Source:    profileSource(path, resolved),
		IsExample: isExample,
	}, nil
}

// LoadProfile loads a profile from the given path argument.
//
// Search order (stops at first match):
//  1. path is absolute → use directly
//  2. ~/.config/infra-lab/profiles/<path>.yaml
//  3. <repo>/envs/<path>.yaml
//  4. <repo>/envs/<path>.yaml.example  (prints a warning)
//
// After loading:
//   - profile.Name is set to the filename stem if empty
//   - profile.State.Dir defaults to "state/<name>" if empty
func LoadProfile(path string) (*Profile, error) {
	resolved, isExample, err := resolveProfilePath(path)
	if err != nil {
		return nil, err
	}
	if isExample {
		fmt.Fprintf(os.Stderr, "warning: loading example profile %s — copy to envs/%s.yaml and fill in real values\n",
			resolved, strings.TrimSuffix(filepath.Base(resolved), ".yaml.example"))
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read profile %q: %w", resolved, err)
	}

	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse profile %q: %w", resolved, err)
	}
	p.SourcePath = resolved

	// Fill defaults.
	if p.Name == "" {
		base := filepath.Base(resolved)
		// Strip .yaml or .yaml.example
		base = strings.TrimSuffix(base, ".example")
		base = strings.TrimSuffix(base, ".yaml")
		p.Name = base
	}
	if p.State.Dir == "" {
		p.State.Dir = "state/" + p.Name
	}
	if p.Kind == "" {
		p.Kind = "kubernetes"
	}
	if p.Kind == "single-vm" {
		if p.VM.OSImage == "" {
			p.VM.OSImage = "ubuntu-24.04"
		}
		if p.VM.ImageURL == "" {
			p.VM.ImageURL = OSImageURL(p.VM.OSImage)
		}
		if p.VM.Count == 0 {
			p.VM.Count = 1
		}
	}

	return &p, nil
}

// KindOrDefault returns the profile kind, preserving backwards compatibility
// for older Kubernetes profiles that do not set kind.
func (p *Profile) KindOrDefault() string {
	if p.Kind == "" {
		return "kubernetes"
	}
	return p.Kind
}

func profileSource(input, resolved string) string {
	if filepath.IsAbs(input) || strings.ContainsRune(input, filepath.Separator) {
		return "explicit"
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		userDir := filepath.Join(home, ".config", "infra-lab", "profiles")
		if rel, err := filepath.Rel(userDir, resolved); err == nil && !strings.HasPrefix(rel, "..") {
			return "user"
		}
	}
	if root, err := FindRoot(); err == nil {
		envsDir := filepath.Join(root, "envs")
		if rel, err := filepath.Rel(envsDir, resolved); err == nil && !strings.HasPrefix(rel, "..") {
			return "repo"
		}
	}
	return "explicit"
}

// resolveProfilePath returns the actual file path for a profile name/path,
// a boolean indicating whether it is a .yaml.example fallback, and an error.
func resolveProfilePath(path string) (string, bool, error) {
	// 1. Absolute path — use as-is.
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err != nil {
			return "", false, fmt.Errorf("profile not found: %s", path)
		}
		return path, false, nil
	}

	// Build candidate list.
	home, _ := os.UserHomeDir()

	var candidates []string

	// If path already has a .yaml extension treat it as a direct relative path first.
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yaml.example") {
		candidates = append(candidates, path)
	}

	// Derive stem (strip extensions if present).
	stem := path
	stem = strings.TrimSuffix(stem, ".yaml.example")
	stem = strings.TrimSuffix(stem, ".yaml")

	// ~/.config/infra-lab/profiles/<stem>.yaml
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".config", "infra-lab", "profiles", stem+".yaml"))
	}

	// <repo>/envs/<stem>.yaml  — find repo root from cwd
	root, rootErr := FindRoot()
	if rootErr == nil {
		candidates = append(candidates, filepath.Join(root, "envs", stem+".yaml"))
	}

	// Check non-example candidates first.
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, false, nil
		}
	}

	// Example fallback: <repo>/envs/<stem>.yaml.example
	if rootErr == nil {
		example := filepath.Join(root, "envs", stem+".yaml.example")
		if _, err := os.Stat(example); err == nil {
			return example, true, nil
		}
	}

	return "", false, fmt.Errorf("profile %q not found (searched ~/.config/infra-lab/profiles/, envs/)", path)
}

// EnvName returns the environment name derived from the profile.
// It uses the basename of state.dir if set, otherwise falls back to name.
func (p *Profile) EnvName() string {
	if p.State.Dir != "" {
		return filepath.Base(p.State.Dir)
	}
	return p.Name
}

// ToEnvVars converts the profile into a map of environment variables
// understood by k8s-tool.sh (ENV_NAME, BACKEND, CNI, TF_VAR_* etc.).
func (p *Profile) ToEnvVars() map[string]string {
	vars := make(map[string]string)

	vars["ENV_NAME"] = p.EnvName()
	vars["BACKEND"] = p.Backend
	vars["CNI"] = p.Kubernetes.CNI

	if p.NamePrefix != "" {
		vars["TF_VAR_name_prefix"] = p.NamePrefix
		vars["NAME_PREFIX"] = p.NamePrefix
	}

	if p.VM.Masters > 0 {
		vars["TF_VAR_masters"] = fmt.Sprintf("%d", p.VM.Masters)
	}
	if p.VM.Workers > 0 {
		vars["TF_VAR_workers"] = fmt.Sprintf("%d", p.VM.Workers)
	}

	// Master node specs.
	if p.VM.Master.CPU > 0 {
		vars["TF_VAR_master_cpus"] = fmt.Sprintf("%d", p.VM.Master.CPU)
	}
	if p.VM.Master.Memory != "" {
		vars["TF_VAR_master_memory"] = p.VM.Master.Memory
	}
	if p.VM.Master.Disk != "" {
		vars["TF_VAR_master_disk"] = p.VM.Master.Disk
	}

	// Worker node specs.
	if p.VM.Worker.CPU > 0 {
		vars["TF_VAR_worker_cpus"] = fmt.Sprintf("%d", p.VM.Worker.CPU)
	}
	if p.VM.Worker.Memory != "" {
		vars["TF_VAR_worker_memory"] = p.VM.Worker.Memory
	}
	if p.VM.Worker.Disk != "" {
		vars["TF_VAR_worker_disk"] = p.VM.Worker.Disk
	}

	// libvirt-specific vars.
	if p.Backend == "libvirt" && p.Libvirt != nil {
		if p.Libvirt.SSHPrivateKeyPath != "" {
			vars["TF_VAR_ssh_private_key_path"] = ExpandTilde(p.Libvirt.SSHPrivateKeyPath)
		}
		if p.Libvirt.SSHPublicKey != "" {
			vars["TF_VAR_ssh_public_key"] = p.Libvirt.SSHPublicKey
		}
		if p.Libvirt.PoolName != "" {
			vars["TF_VAR_libvirt_pool_name"] = p.Libvirt.PoolName
		}
		if p.Libvirt.PoolPath != "" {
			vars["TF_VAR_libvirt_pool_path"] = p.Libvirt.PoolPath
		}
		if p.VM.ImageURL != "" {
			vars["TF_VAR_libvirt_base_image_url"] = p.VM.ImageURL
		}
	}

	return vars
}

// ExpandTilde replaces a leading "~" with the user's home directory.
func ExpandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
