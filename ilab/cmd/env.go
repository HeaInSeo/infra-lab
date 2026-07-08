package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
	"github.com/HeaInSeo/infra-lab/ilab/internal/output"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Environment operations",
}

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all environments in state/",
	RunE:  runEnvList,
}

var envStatusCmd = &cobra.Command{
	Use:   "status [env-name]",
	Short: "Show cluster and VM status for an environment",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runEnvStatus,
}

var envInfoCmd = &cobra.Command{
	Use:   "info <env-name>",
	Short: "Show connection information for an environment",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnvInfo,
}

var envSSHCmd = &cobra.Command{
	Use:   "ssh <env-name>",
	Short: "Open an interactive shell for a single-VM environment",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnvSSH,
}

var envUseCmd = &cobra.Command{
	Use:   "use <profile>",
	Short: "Set the current profile (no VMs affected)",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnvUse,
}

var envPlanCmd = &cobra.Command{
	Use:   "plan <profile>",
	Short: "Run tofu plan for the given profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnvPlan,
}

var envUpCmd = &cobra.Command{
	Use:   "up <profile>",
	Short: "Bring up an environment using k8s-tool.sh",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnvUp,
}

var envDownCmd = &cobra.Command{
	Use:   "down <profile>",
	Short: "Tear down an environment using k8s-tool.sh",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnvDown,
}

var envRebuildApprove bool

var envRebuildCmd = &cobra.Command{
	Use:   "rebuild <profile>",
	Short: "Destroy and recreate an environment (requires --approve)",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnvRebuild,
}

// env up override flags
var (
	envUpCNI     string
	envUpWorkers int
	envUpSaveAs  string
	envUpApprove bool
)

func init() {
	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envStatusCmd)
	envCmd.AddCommand(envInfoCmd)
	envCmd.AddCommand(envSSHCmd)
	envCmd.AddCommand(envUseCmd)
	envCmd.AddCommand(envPlanCmd)

	envUpCmd.Flags().StringVar(&envUpCNI, "cni", "", "override CNI (immutable — requires --save-as)")
	envUpCmd.Flags().IntVar(&envUpWorkers, "workers", 0, "override worker count (scale-in requires --approve)")
	envUpCmd.Flags().StringVar(&envUpSaveAs, "save-as", "", "save overrides as a new profile before running up")
	envUpCmd.Flags().BoolVar(&envUpApprove, "approve", false, "approve scale-in (worker decrease)")
	envCmd.AddCommand(envUpCmd)

	envCmd.AddCommand(envDownCmd)
	envRebuildCmd.Flags().BoolVar(&envRebuildApprove, "approve", false, "actually execute down + up (required)")
	envCmd.AddCommand(envRebuildCmd)
}

// ── existing commands ──────────────────────────────────────────────────────

func runEnvList(_ *cobra.Command, _ []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	envs, err := lab.ListEnvs(root)
	if err != nil {
		return err
	}
	if wantsJSON() {
		return output.WriteJSON(os.Stdout, output.Success("env.list", envListPayload(envs)))
	}
	if len(envs) == 0 {
		fmt.Println("No managed environments found under state/")
		fmt.Println()
		if legacy := lab.DetectLegacyFiles(root); len(legacy) > 0 {
			fmt.Println("Legacy files detected (pre-Phase-2 environment):")
			for _, f := range legacy {
				fmt.Println("  " + f)
			}
			fmt.Println()
			fmt.Println("These will not be modified automatically.")
			fmt.Println("Create a new managed environment with:")
			fmt.Println("  HOST_PROFILE=envs/<name>.env make env-up")
		} else {
			fmt.Println("hint: run 'HOST_PROFILE=envs/<name>.env make env-up' to create one")
		}
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ENV\tKIND\tBACKEND\tCNI\tCREATED\tCOMMIT\tSTALE")
	for _, e := range envs {
		stale := ""
		if count, err := e.TerraformResourceCount(); err == nil && count == 0 {
			stale = "yes"
		}
		kind := e.Kind
		if kind == "" {
			kind = "kubernetes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			e.Name, kind, e.Backend, e.CNI, e.CreatedAt, shortHash(e.GitCommit), stale)
	}
	return w.Flush()
}

func runEnvStatus(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	envName := ""
	if len(args) > 0 {
		envName = args[0]
	}
	if wantsJSON() {
		if envName != "" {
			env, err := lab.LoadEnv(root, envName)
			if err != nil {
				return output.WrapError("ENV_NOT_FOUND", err.Error(), output.ExitDomain, err)
			}
			return output.WriteJSON(os.Stdout, output.Success("env.status", envStatusPayload(root, env)))
		}
		envs, err := lab.ListEnvs(root)
		if err != nil {
			return err
		}
		payloads := make([]envStatusData, 0, len(envs))
		for _, env := range envs {
			payloads = append(payloads, envStatusPayload(root, env))
		}
		return output.WriteJSON(os.Stdout, output.Success("env.status", map[string]any{"envs": payloads}))
	}
	return lab.PrintEnvStatus(root, envName)
}

// ── new profile-driven commands ────────────────────────────────────────────

// runEnvUse validates the profile and stores its name as the current env
// in state/.current-env.  No VMs or state directories are modified.
func runEnvUse(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	p, err := lab.LoadProfile(args[0])
	if err != nil {
		return err
	}

	currentFile := filepath.Join(root, "state", ".current-env")
	if err := os.MkdirAll(filepath.Dir(currentFile), 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	if err := os.WriteFile(currentFile, []byte(p.Name+"\n"), 0644); err != nil {
		return fmt.Errorf("write .current-env: %w", err)
	}

	fmt.Printf("Switched to profile: %s (no VMs affected)\n", p.Name)
	return nil
}

// runEnvPlan runs tofu plan with the profile's environment variables.
func runEnvPlan(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	p, err := lab.LoadProfile(args[0])
	if err != nil {
		return err
	}

	backendDir, err := resolveBackendDir(root, p)
	if err != nil {
		return err
	}

	stateFile := filepath.Join(root, p.State.Dir, "terraform.tfstate")
	tofuArgs := []string{
		"plan",
		"-state", stateFile,
	}
	if p.KindOrDefault() == "single-vm" {
		vars, err := singleVMTFVars(p)
		if err != nil {
			return err
		}
		for _, v := range vars {
			tofuArgs = append(tofuArgs, "-var", v)
		}
	}

	return runWithEnv(root, "tofu", tofuArgs, p.ToEnvVars(), backendDir)
}

// runEnvUp brings up an environment by delegating to k8s-tool.sh up,
// then writes a resolved-profile.yaml on success.
//
// Flag overrides are validated before execution:
//   - Immutable fields (cni, backend, osImage, masters) require --save-as.
//   - Worker scale-in requires --approve.
func runEnvUp(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	p, err := lab.LoadProfile(args[0])
	if err != nil {
		return err
	}

	// Build proposed immutable-field overrides from flags.
	proposed := map[string]string{}
	if envUpCNI != "" {
		proposed["kubernetes.cni"] = envUpCNI
	}

	// Guard: immutable field changes require --save-as.
	if len(proposed) > 0 {
		if conflicts := p.CheckImmutableConflicts(proposed); len(conflicts) > 0 && envUpSaveAs == "" {
			fmt.Fprintln(os.Stderr, "error: this command would change immutable profile fields:")
			for _, c := range conflicts {
				fmt.Fprintf(os.Stderr, "  - %s: %s -> %s\n", c.Field, c.OldValue, c.NewValue)
			}
			fmt.Fprintln(os.Stderr, "\nUse one of:")
			fmt.Fprintf(os.Stderr, "  ilab profile clone %s <new-name>\n", p.Name)
			fmt.Fprintf(os.Stderr, "  ilab env up %s --cni %s --save-as <new-name>\n", p.Name, envUpCNI)
			return fmt.Errorf("immutable field change blocked — add --save-as <new-profile> to create a new profile")
		}
	}

	// Guard: scale-in requires --approve.
	if envUpWorkers > 0 && envUpWorkers < p.VM.Workers && !envUpApprove {
		return fmt.Errorf("scale-in (%d → %d workers) is destructive — re-run with --approve", p.VM.Workers, envUpWorkers)
	}

	// --save-as: apply all overrides to a new profile, then run up with it.
	if envUpSaveAs != "" {
		p, err = saveProfileWithOverrides(p, proposed, envUpWorkers, envUpSaveAs)
		if err != nil {
			return err
		}
	} else {
		// Ephemeral overrides (mutable fields only at this point).
		if envUpWorkers > 0 {
			cp := *p
			cp.VM.Workers = envUpWorkers
			p = &cp
		}
	}

	if p.KindOrDefault() == "single-vm" {
		return runSingleVMUp(root, p)
	}
	if err := runKToolWithProfile(root, p, "up"); err != nil {
		return err
	}
	return writeResolvedProfile(root, p)
}

// saveProfileWithOverrides copies base with proposed+worker overrides, saves to
// ~/.config/infra-lab/profiles/<newName>.yaml, and returns the new profile.
func saveProfileWithOverrides(base *lab.Profile, proposed map[string]string, workers int, newName string) (*lab.Profile, error) {
	cp := *base
	cp.Name = newName
	cp.State.Dir = "state/" + newName

	if cni, ok := proposed["kubernetes.cni"]; ok {
		cp.Kubernetes.CNI = cni
	}
	if workers > 0 {
		cp.VM.Workers = workers
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "infra-lab", "profiles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create profiles dir: %w", err)
	}

	data, err := yaml.Marshal(&cp)
	if err != nil {
		return nil, fmt.Errorf("marshal profile: %w", err)
	}
	outPath := filepath.Join(dir, newName+".yaml")
	if err := os.WriteFile(outPath, data, 0600); err != nil {
		return nil, fmt.Errorf("write profile: %w", err)
	}
	fmt.Printf("Profile saved: %s\n", outPath)
	return &cp, nil
}

// runEnvDown tears down an environment by delegating to k8s-tool.sh down.
func runEnvDown(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	p, err := lab.LoadProfile(args[0])
	if err != nil {
		return err
	}

	stateDir := filepath.Join(root, p.State.Dir)
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: state dir %s does not exist — environment may already be down\n", stateDir)
	}

	if p.KindOrDefault() == "single-vm" {
		return runSingleVMDown(root, p)
	}
	return runKToolWithProfile(root, p, "down")
}

// runEnvRebuild runs down → state-dir removal → up, but only with --approve.
func runEnvRebuild(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	p, err := lab.LoadProfile(args[0])
	if err != nil {
		return err
	}

	if !envRebuildApprove {
		fmt.Printf("Rebuild plan for profile: %s\n\n", p.Name)
		fmt.Printf("  1. env down  (%s)\n", p.EnvName())
		fmt.Printf("  2. rm -rf    %s\n", filepath.Join(root, p.State.Dir))
		fmt.Printf("  3. env up    (%s)\n", p.EnvName())
		fmt.Println()
		fmt.Println("Re-run with --approve to execute.")
		return nil
	}

	// Step 1: down.
	fmt.Println("==> Step 1/3: env down")
	if p.KindOrDefault() == "single-vm" {
		if err := runSingleVMDown(root, p); err != nil {
			return fmt.Errorf("down failed: %w", err)
		}
	} else {
		if err := runKToolWithProfile(root, p, "down"); err != nil {
			return fmt.Errorf("down failed: %w", err)
		}
	}

	// Step 2: remove state dir (save recovery files first).
	stateDir := filepath.Join(root, p.State.Dir)
	fmt.Printf("==> Step 2/3: removing state dir %s\n", stateDir)
	recovery := readRebuildRecoveryFiles(stateDir)
	if err := os.RemoveAll(stateDir); err != nil {
		return fmt.Errorf("remove state dir: %w", err)
	}

	// Step 3: up.
	fmt.Println("==> Step 3/3: env up")
	if p.KindOrDefault() == "single-vm" {
		if err := runSingleVMUp(root, p); err != nil {
			restoreRebuildRecoveryFiles(stateDir, recovery)
			return fmt.Errorf("up failed: %w", err)
		}
	} else {
		if err := runKToolWithProfile(root, p, "up"); err != nil {
			restoreRebuildRecoveryFiles(stateDir, recovery)
			return fmt.Errorf("up failed: %w", err)
		}
	}

	return writeResolvedProfile(root, p)
}

type envInfoPayload struct {
	Env       string           `json:"env"`
	Kind      string           `json:"kind"`
	Backend   string           `json:"backend"`
	SSH       envInfoSSH       `json:"ssh"`
	Workspace envInfoWorkspace `json:"workspace"`
	VM        envInfoVM        `json:"vm"`
}

type envInfoSSH struct {
	Host         string `json:"host"`
	User         string `json:"user"`
	Port         int    `json:"port"`
	IdentityFile string `json:"identityFile"`
}

type envInfoWorkspace struct {
	Path string `json:"path"`
}

type envInfoVM struct {
	Name string `json:"name"`
}

func runEnvInfo(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	p, err := loadResolvedProfile(root, args[0])
	if err != nil {
		return err
	}
	if p.KindOrDefault() != "single-vm" {
		return fmt.Errorf("env info currently supports kind=single-vm only")
	}
	info, err := singleVMInfo(root, p)
	if err != nil {
		return err
	}
	if wantsJSON() {
		return output.WriteJSON(os.Stdout, output.Success("env.info", info))
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Env:\t%s\n", info.Env)
	fmt.Fprintf(w, "VM:\t%s\n", info.VM.Name)
	fmt.Fprintf(w, "SSH:\tssh -i %s -p %d %s@%s\n", info.SSH.IdentityFile, info.SSH.Port, info.SSH.User, info.SSH.Host)
	fmt.Fprintf(w, "Workspace:\t%s\n", info.Workspace.Path)
	return w.Flush()
}

func runEnvSSH(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	p, err := loadResolvedProfile(root, args[0])
	if err != nil {
		return err
	}
	if p.KindOrDefault() != "single-vm" {
		return fmt.Errorf("env ssh currently supports kind=single-vm only")
	}
	info, err := singleVMInfo(root, p)
	if err != nil {
		return err
	}
	cmd := exec.Command("ssh",
		"-i", lab.ExpandTilde(info.SSH.IdentityFile),
		"-p", fmt.Sprintf("%d", info.SSH.Port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		info.SSH.User+"@"+info.SSH.Host,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runSingleVMUp(root string, p *lab.Profile) error {
	if errs := p.Validate(); len(errs) > 0 {
		return fmt.Errorf("profile validation failed: %s", strings.Join(errs, "; "))
	}
	stateDir := filepath.Join(root, p.State.Dir)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	backendDir := filepath.Join(root, "backends", "single-vm")
	stateFile := filepath.Join(stateDir, "terraform.tfstate")
	vars, err := singleVMTFVars(p)
	if err != nil {
		return err
	}
	if err := runWithEnv(root, "tofu", []string{"init", "-input=false"}, nil, backendDir); err != nil {
		return err
	}
	importExistingLibvirtPool(root, backendDir, stateFile, p, vars)
	args := []string{"apply", "-auto-approve", "-input=false", "-state", stateFile}
	for _, v := range vars {
		args = append(args, "-var", v)
	}
	if err := runWithEnv(root, "tofu", args, nil, backendDir); err != nil {
		return err
	}
	if err := writeSingleVMMeta(root, p); err != nil {
		return err
	}
	if err := writeResolvedProfile(root, p); err != nil {
		return err
	}
	info, err := singleVMInfo(root, p)
	if err != nil {
		return err
	}
	if err := waitForSSH(info); err != nil {
		return err
	}
	if err := copyBootstrapScripts(p, info); err != nil {
		return err
	}
	fmt.Printf("single-vm environment %q is ready\n", p.EnvName())
	return nil
}

func runSingleVMDown(root string, p *lab.Profile) error {
	backendDir := filepath.Join(root, "backends", "single-vm")
	stateFile := filepath.Join(root, p.State.Dir, "terraform.tfstate")
	vars, err := singleVMTFVars(p)
	if err != nil {
		return err
	}
	if err := runWithEnv(root, "tofu", []string{"init", "-input=false"}, nil, backendDir); err != nil {
		return err
	}
	args := []string{"destroy", "-auto-approve", "-input=false", "-state", stateFile}
	for _, v := range vars {
		args = append(args, "-var", v)
	}
	return runWithEnv(root, "tofu", args, nil, backendDir)
}

func importExistingLibvirtPool(root, backendDir, stateFile string, p *lab.Profile, vars []string) {
	if p.Libvirt == nil || p.Libvirt.PoolName == "" {
		return
	}
	args := []string{"import", "-input=false", "-state", stateFile}
	for _, v := range vars {
		args = append(args, "-var", v)
	}
	args = append(args, "libvirt_pool.lab", p.Libvirt.PoolName)
	if err := runWithEnv(root, "tofu", args, nil, backendDir); err != nil {
		fmt.Fprintf(os.Stderr, "[INFO] libvirt pool %q was not imported; apply will create it if needed\n", p.Libvirt.PoolName)
	}
}

func singleVMTFVars(p *lab.Profile) ([]string, error) {
	pub, err := os.ReadFile(lab.ExpandTilde(p.SSH.PrivateKeyPath) + ".pub")
	if err != nil {
		return nil, fmt.Errorf("read ssh public key: %w", err)
	}
	dirs, err := json.Marshal(p.Workspace.Dirs)
	if err != nil {
		return nil, err
	}
	vars := []string{
		"env_name=" + p.EnvName(),
		"vm_user=" + p.SSH.User,
		fmt.Sprintf("vm_cpus=%d", p.VM.CPU),
		"vm_memory=" + p.VM.Memory,
		"vm_disk=" + p.VM.Disk,
		"workspace_path=" + p.Workspace.Path,
		"workspace_dirs=" + string(dirs),
		"libvirt_base_image_url=" + p.VM.ImageURL,
		"libvirt_base_image_name=" + strings.ReplaceAll(p.VM.OSImage, ".", "-") + "-base.qcow2",
		"ssh_public_key=" + strings.TrimSpace(string(pub)),
	}
	if p.Libvirt != nil {
		vars = append(vars,
			"libvirt_pool_name="+p.Libvirt.PoolName,
			"libvirt_pool_path="+p.Libvirt.PoolPath,
		)
	}
	return vars, nil
}

func writeSingleVMMeta(root string, p *lab.Profile) error {
	stateDir := filepath.Join(root, p.State.Dir)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return err
	}
	content := strings.Join([]string{
		"env_name=" + p.EnvName(),
		"kind=" + p.KindOrDefault(),
		"backend=" + p.Backend,
		"cni=",
		"name_prefix=" + p.EnvName(),
		"infra_lab_git_commit=" + gitHead(root),
		"infra_lab_git_branch=" + gitBranch(root),
		"created_at=" + time.Now().UTC().Format(time.RFC3339),
		"",
	}, "\n")
	return os.WriteFile(filepath.Join(stateDir, "meta"), []byte(content), 0644)
}

func loadResolvedProfile(root, envName string) (*lab.Profile, error) {
	path := filepath.Join(root, "state", envName, "resolved-profile.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read resolved profile for %q: %w", envName, err)
	}
	var record resolvedProfileOnDisk
	if err := yaml.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("parse resolved profile for %q: %w", envName, err)
	}
	if record.Profile == nil {
		return nil, fmt.Errorf("resolved profile for %q is empty", envName)
	}
	return record.Profile, nil
}

func singleVMInfo(root string, p *lab.Profile) (envInfoPayload, error) {
	host := singleVMOutput(root, p, "ipv4")
	if host == "" {
		host = libvirtDomainIP(p.EnvName())
	}
	if host == "" {
		return envInfoPayload{}, fmt.Errorf("could not determine IP for %q", p.EnvName())
	}
	return envInfoPayload{
		Env:     p.EnvName(),
		Kind:    p.KindOrDefault(),
		Backend: p.Backend,
		SSH: envInfoSSH{
			Host:         host,
			User:         p.SSH.User,
			Port:         22,
			IdentityFile: p.SSH.PrivateKeyPath,
		},
		Workspace: envInfoWorkspace{Path: p.Workspace.Path},
		VM:        envInfoVM{Name: p.EnvName()},
	}, nil
}

func singleVMOutput(root string, p *lab.Profile, name string) string {
	stateFile := filepath.Join(root, p.State.Dir, "terraform.tfstate")
	backendDir := filepath.Join(root, "backends", "single-vm")
	out, err := exec.Command("tofu", "-chdir="+backendDir, "output", "-raw", "-state", stateFile, name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func libvirtDomainIP(name string) string {
	out, err := exec.Command("virsh", "-c", "qemu:///system", "domifaddr", name).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "ipv4") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		addr, _, _ := strings.Cut(fields[3], "/")
		if addr != "" {
			return addr
		}
	}
	return ""
}

func waitForSSH(info envInfoPayload) error {
	addr := info.SSH.User + "@" + info.SSH.Host
	for i := 0; i < 60; i++ {
		cmd := exec.Command("ssh",
			"-i", lab.ExpandTilde(info.SSH.IdentityFile),
			"-p", fmt.Sprintf("%d", info.SSH.Port),
			"-o", "BatchMode=yes",
			"-o", "ConnectTimeout=5",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			addr,
			"true",
		)
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("timed out waiting for SSH on %s", addr)
}

func copyBootstrapScripts(p *lab.Profile, info envInfoPayload) error {
	if len(p.Bootstrap.Scripts) == 0 {
		return nil
	}
	scriptsDir := strings.TrimRight(info.Workspace.Path, "/") + "/scripts"
	if err := runSSH(info, "mkdir -p "+shellQuote(scriptsDir)); err != nil {
		return fmt.Errorf("create remote scripts dir: %w", err)
	}
	for _, script := range p.Bootstrap.Scripts {
		local, err := resolveProfileRelativePath(p, script)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(local)
		if err != nil {
			return fmt.Errorf("read bootstrap script %s: %w", local, err)
		}
		remote := scriptsDir + "/" + filepath.Base(script)
		if err := runSSHWithStdin(info, "cat > "+shellQuote(remote), bytes.NewReader(data)); err != nil {
			return fmt.Errorf("copy bootstrap script %s: %w", script, err)
		}
		if err := runSSH(info, "chmod +x "+shellQuote(remote)); err != nil {
			return fmt.Errorf("chmod bootstrap script %s: %w", script, err)
		}
	}
	return nil
}

func resolveProfileRelativePath(p *lab.Profile, path string) (string, error) {
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("bootstrap script not found: %s", path)
	}
	candidates := []string{path}
	if p.SourcePath != "" {
		dir := filepath.Dir(p.SourcePath)
		for {
			candidates = append(candidates, filepath.Join(dir, path))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("bootstrap script %q not found", path)
}

func runSSH(info envInfoPayload, remoteCmd string) error {
	return runSSHWithStdin(info, remoteCmd, nil)
}

func runSSHWithStdin(info envInfoPayload, remoteCmd string, stdin io.Reader) error {
	cmd := exec.Command("ssh",
		"-i", lab.ExpandTilde(info.SSH.IdentityFile),
		"-p", fmt.Sprintf("%d", info.SSH.Port),
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		info.SSH.User+"@"+info.SSH.Host,
		remoteCmd,
	)
	cmd.Stdin = stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

type rebuildRecovery struct {
	meta            []byte
	resolvedProfile []byte
}

func readRebuildRecoveryFiles(stateDir string) rebuildRecovery {
	var r rebuildRecovery
	r.meta, _ = os.ReadFile(filepath.Join(stateDir, "meta"))
	r.resolvedProfile, _ = os.ReadFile(filepath.Join(stateDir, "resolved-profile.yaml"))
	return r
}

func restoreRebuildRecoveryFiles(stateDir string, r rebuildRecovery) {
	if len(r.meta) == 0 && len(r.resolvedProfile) == 0 {
		return
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] rebuild recovery: could not create state dir: %v\n", err)
		return
	}
	if len(r.meta) > 0 {
		if err := os.WriteFile(filepath.Join(stateDir, "meta"), r.meta, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] rebuild recovery: could not restore meta: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "[INFO] rebuild recovery: restored meta")
		}
	}
	if len(r.resolvedProfile) > 0 {
		if err := os.WriteFile(filepath.Join(stateDir, "resolved-profile.yaml"), r.resolvedProfile, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] rebuild recovery: could not restore resolved-profile.yaml: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "[INFO] rebuild recovery: restored resolved-profile.yaml")
		}
	}
	fmt.Fprintln(os.Stderr, "[INFO] rebuild recovery: state files restored — env is visible to MCP; retry rebuild or run env down to clean up")
}

// ── helpers ────────────────────────────────────────────────────────────────

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

// runKToolWithProfile executes scripts/k8s-tool.sh <action> with the profile's env vars.
func runKToolWithProfile(root string, p *lab.Profile, action string) error {
	script := filepath.Join(root, "scripts", "k8s-tool.sh")
	return runWithEnv(root, "bash", []string{script, action}, p.ToEnvVars(), root)
}

// runWithEnv runs a command in dir, inheriting the current environment and
// overlaying the provided vars.  stdin/stdout/stderr are all streamed.
func runWithEnv(root, name string, args []string, extraEnv map[string]string, dir string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Inherit current environment, then overlay profile vars.
	env := os.Environ()
	for k, v := range extraEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	_ = root // root kept for context; dir is already set
	return cmd.Run()
}

// resolveBackendDir returns the path to the backend Terraform directory.
// Mirrors k8s-tool.sh backend_dir logic.
func resolveBackendDir(root string, p *lab.Profile) (string, error) {
	backend := p.Backend
	if p.KindOrDefault() == "single-vm" {
		backend = "single-vm"
	}
	dir := filepath.Join(root, "backends", backend)
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("backend dir not found: %s", dir)
	}
	return dir, nil
}

// resolvedProfileOnDisk is what we write to state/<env>/resolved-profile.yaml.
type resolvedProfileOnDisk struct {
	*lab.Profile      `yaml:",inline"`
	InfraLabGitCommit string `yaml:"infraLabGitCommit"`
	CreatedAt         string `yaml:"createdAt"`
}

// writeResolvedProfile writes the profile + metadata to state/<env>/resolved-profile.yaml.
func writeResolvedProfile(root string, p *lab.Profile) error {
	stateDir := filepath.Join(root, p.State.Dir)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	commit := gitHead(root)
	record := resolvedProfileOnDisk{
		Profile:           p,
		InfraLabGitCommit: commit,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339),
	}

	data, err := yaml.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal resolved profile: %w", err)
	}

	out := filepath.Join(stateDir, "resolved-profile.yaml")
	if err := os.WriteFile(out, data, 0644); err != nil {
		return fmt.Errorf("write resolved-profile.yaml: %w", err)
	}
	fmt.Printf("resolved-profile.yaml written to %s\n", out)
	return nil
}

// gitHead returns the current HEAD commit hash, or "unknown" on error.
func gitHead(root string) string {
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	h := string(out)
	if len(h) > 0 && h[len(h)-1] == '\n' {
		h = h[:len(h)-1]
	}
	return h
}

func gitBranch(root string) string {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
