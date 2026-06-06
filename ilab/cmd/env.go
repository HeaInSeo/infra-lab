package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
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

func init() {
	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envStatusCmd)
	envCmd.AddCommand(envUseCmd)
	envCmd.AddCommand(envPlanCmd)
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
	fmt.Fprintln(w, "ENV\tBACKEND\tCNI\tCREATED\tCOMMIT")
	for _, e := range envs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			e.Name, e.Backend, e.CNI, e.CreatedAt, shortHash(e.GitCommit))
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

	return runWithEnv(root, "tofu", tofuArgs, p.ToEnvVars(), backendDir)
}

// runEnvUp brings up an environment by delegating to k8s-tool.sh up,
// then writes a resolved-profile.yaml on success.
func runEnvUp(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	p, err := lab.LoadProfile(args[0])
	if err != nil {
		return err
	}

	if err := runKToolWithProfile(root, p, "up"); err != nil {
		return err
	}

	// Write resolved-profile.yaml on success.
	return writeResolvedProfile(root, p)
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
	if err := runKToolWithProfile(root, p, "down"); err != nil {
		return fmt.Errorf("down failed: %w", err)
	}

	// Step 2: remove state dir.
	stateDir := filepath.Join(root, p.State.Dir)
	fmt.Printf("==> Step 2/3: removing state dir %s\n", stateDir)
	if err := os.RemoveAll(stateDir); err != nil {
		return fmt.Errorf("remove state dir: %w", err)
	}

	// Step 3: up.
	fmt.Println("==> Step 3/3: env up")
	if err := runKToolWithProfile(root, p, "up"); err != nil {
		return fmt.Errorf("up failed: %w", err)
	}

	return writeResolvedProfile(root, p)
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
	dir := filepath.Join(root, "backends", p.Backend)
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
