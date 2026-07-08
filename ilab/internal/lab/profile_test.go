package lab

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── helpers ────────────────────────────────────────────────────────────────

// writeProfile writes a YAML profile to path.
func writeProfile(t *testing.T, path, content string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0755))
	must(t, os.WriteFile(path, []byte(content), 0644))
}

// ── LoadProfile ────────────────────────────────────────────────────────────

const libvirtProfileYAML = `
name: libvirt-flannel
backend: libvirt
vm:
  osImage: ubuntu-24.04
  imageUrl: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
  masters: 1
  workers: 2
  master:
    cpu: 2
    memory: 4G
    disk: 40G
  worker:
    cpu: 2
    memory: 4G
    disk: 50G
kubernetes:
  version: "1.32"
  cni: flannel
addons:
  base:
    - metrics-server
  optional: []
libvirt:
  sshPrivateKeyPath: ~/.ssh/id_ed25519
  sshPublicKey: "ssh-ed25519 AAAA..."
  poolName: lab-pool
  poolPath: /var/lib/libvirt/images
state:
  dir: state/libvirt-flannel
`

const multipassProfileYAML = `
name: multipass-flannel
backend: multipass
vm:
  osImage: ubuntu-24.04
  masters: 1
  workers: 2
  master:
    cpu: 2
    memory: 4G
    disk: 40G
  worker:
    cpu: 2
    memory: 4G
    disk: 50G
kubernetes:
  version: "1.32"
  cni: flannel
addons:
  base:
    - metrics-server
  optional: []
state:
  dir: state/multipass-flannel
`

func singleVMProfileYAML(keyPath string) string {
	return `
kind: single-vm
name: ebpf-dev
backend: libvirt
vm:
  osImage: ubuntu-24.04
  imageUrl: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
  count: 1
  cpu: 4
  memory: 8G
  disk: 80G
ssh:
  user: ubuntu
  privateKeyPath: ` + keyPath + `
workspace:
  path: /home/ubuntu/workspace/ebpf-lab
  dirs:
    - c-libbpf
    - rust-aya
    - notes
    - scripts
bootstrap:
  scripts:
    - lab/infra-lab/bootstrap/install-core.sh
libvirt:
  poolName: lab-pool
  poolPath: /var/lib/libvirt/images
state:
  dir: state/ebpf-dev
`
}

func TestLoadProfile_AbsolutePath(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "libvirt-flannel.yaml")
	writeProfile(t, path, libvirtProfileYAML)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "libvirt-flannel" {
		t.Errorf("p.Name = %q, want libvirt-flannel", p.Name)
	}
	if p.Backend != "libvirt" {
		t.Errorf("p.Backend = %q, want libvirt", p.Backend)
	}
	if p.Kubernetes.CNI != "flannel" {
		t.Errorf("p.Kubernetes.CNI = %q, want flannel", p.Kubernetes.CNI)
	}
}

func TestLoadProfile_DefaultKindKubernetes(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "multipass-flannel.yaml")
	writeProfile(t, path, multipassProfileYAML)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.KindOrDefault(); got != "kubernetes" {
		t.Errorf("KindOrDefault() = %q, want kubernetes", got)
	}
}

func TestLoadProfile_DefaultName(t *testing.T) {
	// Profile with empty name field — should derive from filename.
	tmp := t.TempDir()
	path := filepath.Join(tmp, "my-profile.yaml")
	writeProfile(t, path, "backend: multipass\n")

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "my-profile" {
		t.Errorf("p.Name = %q, want my-profile", p.Name)
	}
}

func TestLoadProfile_DefaultStateDir(t *testing.T) {
	// Profile with no state.dir — should default to "state/<name>".
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test-env.yaml")
	writeProfile(t, path, "name: test-env\nbackend: multipass\n")

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.State.Dir != "state/test-env" {
		t.Errorf("p.State.Dir = %q, want state/test-env", p.State.Dir)
	}
}

func TestLoadProfile_ExplicitStateDir(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "custom.yaml")
	writeProfile(t, path, "name: custom\nbackend: multipass\nstate:\n  dir: state/custom-env\n")

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.State.Dir != "state/custom-env" {
		t.Errorf("p.State.Dir = %q, want state/custom-env", p.State.Dir)
	}
}

func TestLoadProfile_InvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.yaml")
	writeProfile(t, path, "this: is: not: valid: yaml: [")

	_, err := LoadProfile(path)
	if err == nil {
		t.Error("LoadProfile() expected error for invalid YAML, got nil")
	}
}

func TestLoadProfile_NotFound(t *testing.T) {
	_, err := LoadProfile("/nonexistent/path/profile.yaml")
	if err == nil {
		t.Error("LoadProfile() expected error for missing file, got nil")
	}
}

func TestLoadProfile_SearchRepoEnvs(t *testing.T) {
	// Set up a fake repo with envs/<name>.yaml, then set INFRA_LAB_ROOT.
	tmp := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(tmp, "scripts"), 0755))
	must(t, os.WriteFile(filepath.Join(tmp, "scripts", "k8s-tool.sh"), nil, 0644))
	must(t, os.MkdirAll(filepath.Join(tmp, "envs"), 0755))
	writeProfile(t, filepath.Join(tmp, "envs", "my-env.yaml"), multipassProfileYAML)

	t.Setenv("INFRA_LAB_ROOT", tmp)

	p, err := LoadProfile("my-env")
	if err != nil {
		t.Fatal(err)
	}
	if p.Backend != "multipass" {
		t.Errorf("p.Backend = %q, want multipass", p.Backend)
	}
}

func TestLoadProfile_ExampleFallback(t *testing.T) {
	// Only .yaml.example exists — should load with warning (we just check no error).
	tmp := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(tmp, "scripts"), 0755))
	must(t, os.WriteFile(filepath.Join(tmp, "scripts", "k8s-tool.sh"), nil, 0644))
	must(t, os.MkdirAll(filepath.Join(tmp, "envs"), 0755))
	writeProfile(t, filepath.Join(tmp, "envs", "example-env.yaml.example"),
		"name: example-env\nbackend: libvirt\n")

	t.Setenv("INFRA_LAB_ROOT", tmp)

	p, err := LoadProfile("example-env")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "example-env" {
		t.Errorf("p.Name = %q, want example-env", p.Name)
	}
}

// ── EnvName ────────────────────────────────────────────────────────────────

func TestEnvName_FromStateDir(t *testing.T) {
	p := &Profile{Name: "libvirt-flannel", State: StateSpec{Dir: "state/libvirt-flannel"}}
	if got := p.EnvName(); got != "libvirt-flannel" {
		t.Errorf("EnvName() = %q, want libvirt-flannel", got)
	}
}

func TestEnvName_FromName(t *testing.T) {
	p := &Profile{Name: "my-env"}
	if got := p.EnvName(); got != "my-env" {
		t.Errorf("EnvName() = %q, want my-env", got)
	}
}

func TestEnvName_StateDirBasename(t *testing.T) {
	p := &Profile{Name: "ignored", State: StateSpec{Dir: "state/actual-name"}}
	if got := p.EnvName(); got != "actual-name" {
		t.Errorf("EnvName() = %q, want actual-name", got)
	}
}

func TestResolveEnvForVMName_SingleVMExactName(t *testing.T) {
	envs := []*Env{
		{Name: "ebpf-dev", Kind: "single-vm", NamePrefix: "ebpf-dev"},
	}
	env, err := resolveEnvForVMName(envs, "ebpf-dev")
	if err != nil {
		t.Fatal(err)
	}
	if env == nil || env.Name != "ebpf-dev" {
		t.Fatalf("resolveEnvForVMName() = %#v, want ebpf-dev", env)
	}
}

func TestResolveEnvForVMName_SingleVMDoesNotRequireDashSuffix(t *testing.T) {
	envs := []*Env{
		{Name: "ebpf-dev", Kind: "single-vm", NamePrefix: "ebpf-dev"},
	}
	env, err := resolveEnvForVMName(envs, "ebpf-dev-master-0")
	if err != nil {
		t.Fatal(err)
	}
	if env == nil {
		t.Fatal("resolveEnvForVMName() = nil, want prefix match for legacy-style name")
	}
}

// ── ToEnvVars ──────────────────────────────────────────────────────────────

func TestToEnvVars_Libvirt(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "libvirt-flannel.yaml")
	writeProfile(t, path, libvirtProfileYAML)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}

	vars := p.ToEnvVars()

	checks := []struct{ key, want string }{
		{"ENV_NAME", "libvirt-flannel"},
		{"BACKEND", "libvirt"},
		{"CNI", "flannel"},
		{"TF_VAR_masters", "1"},
		{"TF_VAR_workers", "2"},
		{"TF_VAR_master_cpus", "2"},
		{"TF_VAR_master_memory", "4G"},
		{"TF_VAR_master_disk", "40G"},
		{"TF_VAR_worker_cpus", "2"},
		{"TF_VAR_worker_memory", "4G"},
		{"TF_VAR_worker_disk", "50G"},
		{"TF_VAR_libvirt_pool_name", "lab-pool"},
		{"TF_VAR_libvirt_pool_path", "/var/lib/libvirt/images"},
		{"TF_VAR_libvirt_base_image_url", "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"},
	}
	for _, c := range checks {
		if got := vars[c.key]; got != c.want {
			t.Errorf("vars[%q] = %q, want %q", c.key, got, c.want)
		}
	}

	// SSH public key must be present.
	if sshKey := vars["TF_VAR_ssh_public_key"]; !strings.HasPrefix(sshKey, "ssh-ed25519") {
		t.Errorf("vars[TF_VAR_ssh_public_key] = %q, want ssh-ed25519 prefix", sshKey)
	}

	// SSH private key path must exist (tilde expanded to absolute).
	sshPriv := vars["TF_VAR_ssh_private_key_path"]
	if strings.HasPrefix(sshPriv, "~") {
		t.Errorf("vars[TF_VAR_ssh_private_key_path] = %q, tilde should be expanded", sshPriv)
	}
}

func TestToEnvVars_Multipass(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "multipass-flannel.yaml")
	writeProfile(t, path, multipassProfileYAML)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}

	vars := p.ToEnvVars()

	if vars["BACKEND"] != "multipass" {
		t.Errorf("vars[BACKEND] = %q, want multipass", vars["BACKEND"])
	}
	if vars["ENV_NAME"] != "multipass-flannel" {
		t.Errorf("vars[ENV_NAME] = %q, want multipass-flannel", vars["ENV_NAME"])
	}

	// Libvirt-specific vars must NOT be present for multipass.
	for _, key := range []string{
		"TF_VAR_ssh_private_key_path", "TF_VAR_ssh_public_key",
		"TF_VAR_libvirt_pool_name", "TF_VAR_libvirt_pool_path",
		"TF_VAR_libvirt_base_image_url",
	} {
		if _, ok := vars[key]; ok {
			t.Errorf("vars[%q] should not be set for multipass backend", key)
		}
	}
}

func TestToEnvVars_NoLibvirtSection(t *testing.T) {
	// libvirt backend but no libvirt section — should not panic, skip libvirt vars.
	p := &Profile{
		Name:    "no-libvirt",
		Backend: "libvirt",
		State:   StateSpec{Dir: "state/no-libvirt"},
	}
	vars := p.ToEnvVars()
	if vars["BACKEND"] != "libvirt" {
		t.Errorf("vars[BACKEND] = %q, want libvirt", vars["BACKEND"])
	}
	if _, ok := vars["TF_VAR_ssh_public_key"]; ok {
		t.Error("TF_VAR_ssh_public_key should not be set when Libvirt is nil")
	}
}

func TestValidate_SingleVMAllowsNoKubernetesFields(t *testing.T) {
	tmp := t.TempDir()
	keyPath := filepath.Join(tmp, "id_ed25519")
	must(t, os.WriteFile(keyPath, []byte("private key placeholder"), 0600))
	must(t, os.WriteFile(keyPath+".pub", []byte("ssh-ed25519 AAAArealkey test@example\n"), 0644))
	path := filepath.Join(tmp, "ebpf-dev.yaml")
	writeProfile(t, path, singleVMProfileYAML(keyPath))

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if errs := p.Validate(); len(errs) > 0 {
		t.Fatalf("Validate() errors = %#v, want none", errs)
	}
	if p.Kubernetes.CNI != "" {
		t.Fatalf("Kubernetes.CNI = %q, want empty for single-vm", p.Kubernetes.CNI)
	}
}

func TestValidate_SingleVMRequiresCountOne(t *testing.T) {
	tmp := t.TempDir()
	keyPath := filepath.Join(tmp, "id_ed25519")
	must(t, os.WriteFile(keyPath, []byte("private key placeholder"), 0600))
	must(t, os.WriteFile(keyPath+".pub", []byte("ssh-ed25519 AAAArealkey test@example\n"), 0644))
	path := filepath.Join(tmp, "ebpf-dev.yaml")
	writeProfile(t, path, strings.Replace(singleVMProfileYAML(keyPath), "count: 1", "count: 2", 1))

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	errs := p.Validate()
	if len(errs) == 0 {
		t.Fatal("Validate() got no errors, want vm.count error")
	}
	found := false
	for _, err := range errs {
		if err == "vm.count must be 1 for kind=single-vm" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Validate() errors = %#v, want vm.count error", errs)
	}
}

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	tests := []struct {
		in   string
		want string
	}{
		{"~/.ssh/id_ed25519", filepath.Join(home, ".ssh/id_ed25519")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~", home},
	}
	for _, tc := range tests {
		got := ExpandTilde(tc.in)
		if got != tc.want {
			t.Errorf("expandTilde(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
