package lab

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ── helpers ────────────────────────────────────────────────────────────────

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// makeEnv creates a minimal state/<name>/meta in root for testing.
func makeEnv(t *testing.T, root, name, backend, cni, prefix string) {
	t.Helper()
	dir := filepath.Join(root, "state", name)
	must(t, os.MkdirAll(dir, 0755))
	content := fmt.Sprintf(
		"backend=%s\ncni=%s\nname_prefix=%s\n"+
			"infra_lab_git_commit=abc1234\ninfra_lab_git_branch=main\n"+
			"created_at=2026-06-05T00:00:00Z\n",
		backend, cni, prefix,
	)
	must(t, os.WriteFile(filepath.Join(dir, "meta"), []byte(content), 0644))
}

// ── FindRoot ───────────────────────────────────────────────────────────────

func TestFindRoot_EnvOverride(t *testing.T) {
	t.Setenv("INFRA_LAB_ROOT", "/override/path")
	got, err := FindRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/override/path" {
		t.Errorf("FindRoot() = %q, want /override/path", got)
	}
}

func TestFindRoot_WalkUp(t *testing.T) {
	t.Setenv("INFRA_LAB_ROOT", "") // prevent env var from interfering

	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, "scripts"), 0755))
	must(t, os.WriteFile(filepath.Join(root, "scripts", "k8s-tool.sh"), nil, 0644))

	subdir := filepath.Join(root, "ilab", "cmd")
	must(t, os.MkdirAll(subdir, 0755))

	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	must(t, os.Chdir(subdir))

	got, err := FindRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("FindRoot() = %q, want %q", got, root)
	}
}

func TestFindRoot_NotFound(t *testing.T) {
	t.Setenv("INFRA_LAB_ROOT", "")

	tmp := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	must(t, os.Chdir(tmp))

	_, err := FindRoot()
	if err == nil {
		t.Error("FindRoot() expected error outside repo, got nil")
	}
}

// ── DetectLegacyFiles ──────────────────────────────────────────────────────

func TestDetectLegacyFiles(t *testing.T) {
	tests := []struct {
		name      string
		files     []string
		wantCount int
	}{
		{"none", nil, 0},
		{"kubeconfig only", []string{"kubeconfig"}, 1},
		{"terraform.tfstate only", []string{"terraform.tfstate"}, 1},
		{"tofu.tfstate only", []string{"tofu.tfstate"}, 1},
		{"kubeconfig + tfstate", []string{"kubeconfig", "terraform.tfstate"}, 2},
		{"libvirt kubeconfig", []string{"kubeconfig.libvirt"}, 1},
		{"all legacy files", []string{"kubeconfig", "kubeconfig.libvirt", "terraform.tfstate", "tofu.tfstate"}, 4},
		{"unrelated files ignored", []string{"some-other-file.txt"}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			for _, f := range tc.files {
				must(t, os.WriteFile(filepath.Join(tmp, f), nil, 0644))
			}
			got := DetectLegacyFiles(tmp)
			if len(got) != tc.wantCount {
				t.Errorf("DetectLegacyFiles() = %v (%d), want %d files",
					got, len(got), tc.wantCount)
			}
		})
	}
}

// ── readMeta ──────────────────────────────────────────────────────────────

func TestReadMeta(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]string
	}{
		{
			name:    "simple key=value",
			content: "backend=multipass\ncni=flannel\n",
			want:    map[string]string{"backend": "multipass", "cni": "flannel"},
		},
		{
			name:    "whitespace trimmed",
			content: " backend = multipass \n cni = flannel \n",
			want:    map[string]string{"backend": "multipass", "cni": "flannel"},
		},
		{
			name:    "empty lines skipped",
			content: "a=1\n\n\nb=2\n",
			want:    map[string]string{"a": "1", "b": "2"},
		},
		{
			name:    "no trailing newline",
			content: "key=val",
			want:    map[string]string{"key": "val"},
		},
		{
			name:    "line without = skipped",
			content: "key=val\njunk\nother=x\n",
			want:    map[string]string{"key": "val", "other": "x"},
		},
		{
			name:    "value with equals sign",
			content: "ssh_pub=ssh-ed25519 AAAA=rest\n",
			want:    map[string]string{"ssh_pub": "ssh-ed25519 AAAA=rest"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			path := filepath.Join(tmp, "meta")
			must(t, os.WriteFile(path, []byte(tc.content), 0644))

			got, err := readMeta(path)
			if err != nil {
				t.Fatal(err)
			}
			for k, want := range tc.want {
				if got[k] != want {
					t.Errorf("meta[%q] = %q, want %q", k, got[k], want)
				}
			}
		})
	}
}

func TestReadMeta_NotFound(t *testing.T) {
	_, err := readMeta("/nonexistent/path/meta")
	if err == nil {
		t.Error("readMeta() expected error for missing file, got nil")
	}
}

// ── LoadEnv ───────────────────────────────────────────────────────────────

func TestLoadEnv(t *testing.T) {
	tmp := t.TempDir()
	makeEnv(t, tmp, "multipass-flannel", "multipass", "flannel", "lab")

	env, err := LoadEnv(tmp, "multipass-flannel")
	if err != nil {
		t.Fatal(err)
	}

	checks := []struct{ got, want, field string }{
		{env.Name, "multipass-flannel", "Name"},
		{env.Backend, "multipass", "Backend"},
		{env.CNI, "flannel", "CNI"},
		{env.NamePrefix, "lab", "NamePrefix"},
		{env.GitCommit, "abc1234", "GitCommit"},
		{env.GitBranch, "main", "GitBranch"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("env.%s = %q, want %q", c.field, c.got, c.want)
		}
	}

	if env.Root != tmp {
		t.Errorf("env.Root = %q, want %q", env.Root, tmp)
	}
	wantKubeconfig := filepath.Join(tmp, "state", "multipass-flannel", "kubeconfig")
	if env.Kubeconfig != wantKubeconfig {
		t.Errorf("env.Kubeconfig = %q, want %q", env.Kubeconfig, wantKubeconfig)
	}
}

func TestLoadEnv_DefaultPrefix(t *testing.T) {
	// name_prefix absent → defaults to "lab"
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "state", "no-prefix")
	must(t, os.MkdirAll(dir, 0755))
	must(t, os.WriteFile(filepath.Join(dir, "meta"), []byte("backend=multipass\ncni=flannel\n"), 0644))

	env, err := LoadEnv(tmp, "no-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if env.NamePrefix != "lab" {
		t.Errorf("NamePrefix = %q, want lab", env.NamePrefix)
	}
}

func TestLoadEnv_MissingMeta(t *testing.T) {
	tmp := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(tmp, "state", "empty"), 0755))
	// no meta file
	_, err := LoadEnv(tmp, "empty")
	if err == nil {
		t.Error("LoadEnv() expected error for missing meta, got nil")
	}
}

// ── TerraformResourceCount ───────────────────────────────────────────────

func TestTerraformResourceCount_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	makeEnv(t, tmp, "no-state-file", "libvirt", "flannel", "lab")
	env, err := LoadEnv(tmp, "no-state-file")
	must(t, err)

	count, err := env.TerraformResourceCount()
	if err != nil {
		t.Fatalf("TerraformResourceCount() error = %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 for missing state file", count)
	}
}

func TestTerraformResourceCount_EmptyResources(t *testing.T) {
	tmp := t.TempDir()
	makeEnv(t, tmp, "stale-env", "libvirt", "flannel", "lab")
	env, err := LoadEnv(tmp, "stale-env")
	must(t, err)
	must(t, os.WriteFile(env.StateFile, []byte(`{"version":4,"resources":[],"outputs":{}}`), 0644))

	count, err := env.TerraformResourceCount()
	if err != nil {
		t.Fatalf("TerraformResourceCount() error = %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 for empty resources array", count)
	}
}

func TestTerraformResourceCount_HasResources(t *testing.T) {
	tmp := t.TempDir()
	makeEnv(t, tmp, "live-env", "libvirt", "flannel", "lab")
	env, err := LoadEnv(tmp, "live-env")
	must(t, err)
	must(t, os.WriteFile(env.StateFile, []byte(`{"version":4,"resources":[{"type":"libvirt_domain"},{"type":"libvirt_volume"}],"outputs":{}}`), 0644))

	count, err := env.TerraformResourceCount()
	if err != nil {
		t.Fatalf("TerraformResourceCount() error = %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestTerraformResourceCount_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	makeEnv(t, tmp, "broken-state", "libvirt", "flannel", "lab")
	env, err := LoadEnv(tmp, "broken-state")
	must(t, err)
	must(t, os.WriteFile(env.StateFile, []byte("not json"), 0644))

	if _, err := env.TerraformResourceCount(); err == nil {
		t.Error("TerraformResourceCount() expected error for invalid JSON, got nil")
	}
}

// ── ListEnvs ──────────────────────────────────────────────────────────────

func TestListEnvs_NoStateDir(t *testing.T) {
	tmp := t.TempDir()
	envs, err := ListEnvs(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 0 {
		t.Errorf("ListEnvs() = %d envs, want 0", len(envs))
	}
}

func TestListEnvs_Empty(t *testing.T) {
	tmp := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(tmp, "state"), 0755))

	envs, err := ListEnvs(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 0 {
		t.Errorf("ListEnvs() = %d envs, want 0", len(envs))
	}
}

func TestListEnvs_Multiple(t *testing.T) {
	tmp := t.TempDir()
	makeEnv(t, tmp, "multipass-flannel", "multipass", "flannel", "lab")
	makeEnv(t, tmp, "libvirt-cilium", "libvirt", "cilium", "lab")

	envs, err := ListEnvs(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 2 {
		t.Errorf("ListEnvs() = %d envs, want 2", len(envs))
	}
}

func TestListEnvs_SkipsDirsWithoutMeta(t *testing.T) {
	tmp := t.TempDir()
	makeEnv(t, tmp, "valid-env", "multipass", "flannel", "lab")
	// dir without meta file — should be silently skipped
	must(t, os.MkdirAll(filepath.Join(tmp, "state", "no-meta"), 0755))

	envs, err := ListEnvs(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 {
		t.Errorf("ListEnvs() = %d envs, want 1 (invalid dir should be skipped)", len(envs))
	}
	if envs[0].Name != "valid-env" {
		t.Errorf("envs[0].Name = %q, want valid-env", envs[0].Name)
	}
}

// ── FindEnvForVM ──────────────────────────────────────────────────────────

func TestFindEnvForVM_Found(t *testing.T) {
	tmp := t.TempDir()
	makeEnv(t, tmp, "multipass-flannel", "multipass", "flannel", "lab")

	for _, vmName := range []string{"lab-master-0", "lab-worker-0", "lab-worker-1"} {
		t.Run(vmName, func(t *testing.T) {
			env, err := FindEnvForVM(tmp, vmName)
			if err != nil {
				t.Fatal(err)
			}
			if env.Name != "multipass-flannel" {
				t.Errorf("env.Name = %q, want multipass-flannel", env.Name)
			}
		})
	}
}

func TestFindEnvForVM_PicksCorrectEnv(t *testing.T) {
	tmp := t.TempDir()
	makeEnv(t, tmp, "env-a", "multipass", "flannel", "clusterA")
	makeEnv(t, tmp, "env-b", "libvirt", "cilium", "clusterB")

	env, err := FindEnvForVM(tmp, "clusterB-worker-0")
	if err != nil {
		t.Fatal(err)
	}
	if env.Name != "env-b" {
		t.Errorf("env.Name = %q, want env-b", env.Name)
	}
}

func TestFindEnvForVM_FallbackWhenNoMatch(t *testing.T) {
	// No state dir → falls back to minimal multipass env.
	tmp := t.TempDir()
	env, err := FindEnvForVM(tmp, "some-unmanaged-vm")
	if err != nil {
		t.Fatal(err)
	}
	if env.Backend != "multipass" {
		t.Errorf("fallback env.Backend = %q, want multipass", env.Backend)
	}
	if env.Name != "" {
		t.Errorf("fallback env.Name = %q, want empty", env.Name)
	}
}

func TestFindEnvForVM_PrefixMustMatch(t *testing.T) {
	// "label-worker-0" should not match prefix "lab"
	tmp := t.TempDir()
	makeEnv(t, tmp, "myenv", "multipass", "flannel", "lab")

	env, err := FindEnvForVM(tmp, "label-worker-0")
	if err != nil {
		t.Fatal(err)
	}
	// "label-worker-0" doesn't start with "lab-", so no env match → fallback
	if env.Name == "myenv" {
		t.Error("FindEnvForVM() incorrectly matched 'label-' to prefix 'lab'")
	}
}

func writeState(t *testing.T, env *Env, resourceCount int) {
	t.Helper()
	resources := make([]string, resourceCount)
	for i := range resources {
		resources[i] = `{"type":"libvirt_domain"}`
	}
	body := fmt.Sprintf(`{"version":4,"resources":[%s],"outputs":{}}`, strings.Join(resources, ","))
	must(t, os.WriteFile(env.StateFile, []byte(body), 0644))
}

func TestFindEnvForVM_DuplicatePrefix_OneLive(t *testing.T) {
	// Two envs share name_prefix "lab"; only one has live terraform resources.
	tmp := t.TempDir()
	makeEnv(t, tmp, "stale-env", "libvirt", "flannel", "lab")
	makeEnv(t, tmp, "live-env", "libvirt", "cilium", "lab")

	staleEnv, err := LoadEnv(tmp, "stale-env")
	must(t, err)
	writeState(t, staleEnv, 0)

	liveEnv, err := LoadEnv(tmp, "live-env")
	must(t, err)
	writeState(t, liveEnv, 3)

	env, err := FindEnvForVM(tmp, "lab-master-0")
	if err != nil {
		t.Fatalf("FindEnvForVM() error = %v", err)
	}
	if env.Name != "live-env" {
		t.Errorf("env.Name = %q, want live-env (the one with live terraform state)", env.Name)
	}
}

func TestFindEnvForVM_DuplicatePrefix_NoneLive(t *testing.T) {
	// Two envs share name_prefix "lab"; neither has live terraform resources.
	// No real infra is at stake, so this must not error — just pick one.
	tmp := t.TempDir()
	makeEnv(t, tmp, "env-a", "libvirt", "flannel", "lab")
	makeEnv(t, tmp, "env-b", "libvirt", "cilium", "lab")

	env, err := FindEnvForVM(tmp, "lab-master-0")
	if err != nil {
		t.Fatalf("FindEnvForVM() error = %v, want no error when no matching env is live", err)
	}
	if env.Name != "env-a" && env.Name != "env-b" {
		t.Errorf("env.Name = %q, want env-a or env-b", env.Name)
	}
}

func TestFindEnvForVM_DuplicatePrefix_BothLive(t *testing.T) {
	// Two envs share name_prefix "lab" and both genuinely have live
	// terraform resources — this is real, unresolvable ambiguity.
	tmp := t.TempDir()
	makeEnv(t, tmp, "env-a", "libvirt", "flannel", "lab")
	makeEnv(t, tmp, "env-b", "libvirt", "cilium", "lab")

	envA, err := LoadEnv(tmp, "env-a")
	must(t, err)
	writeState(t, envA, 2)

	envB, err := LoadEnv(tmp, "env-b")
	must(t, err)
	writeState(t, envB, 5)

	_, err = FindEnvForVM(tmp, "lab-master-0")
	if err == nil {
		t.Fatal("FindEnvForVM() expected error for genuine ambiguity, got nil")
	}
}

// ── BuildInfo.Print ───────────────────────────────────────────────────────

func TestBuildInfoPrint(t *testing.T) {
	b := &BuildInfo{
		NodeName:          "lab-master-0",
		Role:              "control-plane",
		KubernetesVersion: "v1.32.5",
		CNI:               "flannel",
		Backend:           "multipass",
		EnvName:           "multipass-flannel",
		InfraLabGitBranch: "main",
		InfraLabGitCommit: "abc1234",
		CreatedAt:         "2026-06-05T00:00:00Z",
	}

	var buf bytes.Buffer
	b.Print(&buf)
	out := buf.String()

	for _, want := range []string{
		"lab-master-0", "control-plane", "v1.32.5",
		"flannel", "multipass", "multipass-flannel",
		"main", "abc1234", "2026-06-05",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Print() output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestBuildInfoPrint_Empty(t *testing.T) {
	// Should not panic on zero-value struct.
	var b BuildInfo
	var buf bytes.Buffer
	b.Print(&buf)
}

// ── ListAllVMs ─────────────────────────────────────────────────────────────

func TestListAllVMs_ManagedAnnotation(t *testing.T) {
	if _, err := exec.LookPath("multipass"); err != nil {
		t.Skip("multipass not available")
	}

	tmp := t.TempDir()
	makeEnv(t, tmp, "test-env", "multipass", "flannel", "lab")

	// With multipass available, just verify the function runs without error
	// and returns VMInfo values with Managed field populated correctly.
	vms, err := ListAllVMs(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, vm := range vms {
		wantManaged := strings.HasPrefix(vm.Name, "lab-")
		if vm.Managed != wantManaged {
			t.Errorf("VM %q: Managed=%v, want %v", vm.Name, vm.Managed, wantManaged)
		}
	}
}
