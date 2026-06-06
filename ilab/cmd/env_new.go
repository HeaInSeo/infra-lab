package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
)

var envNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Interactive wizard to create and launch a new environment",
	Args:  cobra.NoArgs,
	RunE:  runEnvNew,
}

func init() {
	envCmd.AddCommand(envNewCmd)
}

func runEnvNew(_ *cobra.Command, _ []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}

	// Require an interactive terminal.
	if fi, err := os.Stdin.Stat(); err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return fmt.Errorf("ilab env new requires an interactive terminal\nhint: use 'ilab profile new <name> --flags' for non-interactive profile creation")
	}

	w := &wizard{r: bufio.NewReader(os.Stdin)}

	fmt.Println()
	fmt.Println("ilab env new — interactive wizard")
	fmt.Println(strings.Repeat("─", 34))
	fmt.Println()

	// ── name ──────────────────────────────────────────────────────────────────

	name := w.ask("Environment name", "my-k8s")

	// ── backend ───────────────────────────────────────────────────────────────

	backend := w.pick("Backend", []string{"libvirt", "multipass"}, 0)

	// ── CNI ───────────────────────────────────────────────────────────────────

	cni := w.pick("CNI plugin", []string{"flannel", "cilium", "calico"}, 0)

	// ── OS image ──────────────────────────────────────────────────────────────

	osList := lab.SupportedOSImages()
	osDefault := 0
	for i, o := range osList {
		if o == "ubuntu-24.04" {
			osDefault = i
			break
		}
	}
	osImage := w.pick("OS image", osList, osDefault)

	// ── node counts ───────────────────────────────────────────────────────────

	workers := w.askInt("Worker nodes", 2)
	masters := w.askInt("Master nodes", 1)

	// ── libvirt settings ──────────────────────────────────────────────────────

	var libvirtSpec *lab.LibvirtSpec
	if backend == "libvirt" {
		fmt.Println()
		fmt.Println("libvirt settings")

		sshKey := w.ask("SSH key path", "~/.ssh/id_ed25519")
		poolName := w.ask("Pool name", "lab-pool")
		poolPath := w.ask("Pool path", "/var/lib/libvirt/images")

		sshPub, err := readSSHPublicKey(sshKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not read %s.pub: %v\n", sshKey, err)
			sshPub = "ssh-ed25519 AAAA... # TODO: replace with your public key"
		}

		libvirtSpec = &lab.LibvirtSpec{
			SSHPrivateKeyPath: sshKey,
			SSHPublicKey:      sshPub,
			PoolName:          poolName,
			PoolPath:          poolPath,
		}
	}

	// ── build profile ─────────────────────────────────────────────────────────

	imageURL := lab.OSImageURL(osImage)
	p := &lab.Profile{
		Name:    name,
		Backend: backend,
		VM: lab.VMSpec{
			OSImage:  osImage,
			ImageURL: imageURL,
			Masters:  masters,
			Workers:  workers,
			Master:   lab.NodeSpec{CPU: 2, Memory: "4G", Disk: "40G"},
			Worker:   lab.NodeSpec{CPU: 2, Memory: "4G", Disk: "50G"},
		},
		Kubernetes: lab.KubernetesSpec{
			Version: "1.32",
			CNI:     cni,
		},
		Addons: lab.AddonsSpec{
			Base:     []string{"metrics-server"},
			Optional: []string{},
		},
		Libvirt: libvirtSpec,
		State:   lab.StateSpec{Dir: "state/" + name},
	}

	// ── summary ───────────────────────────────────────────────────────────────

	fmt.Println()
	fmt.Println("Profile summary")
	fmt.Printf("  name:    %s\n", p.Name)
	fmt.Printf("  backend: %s\n", p.Backend)
	fmt.Printf("  cni:     %s\n", p.Kubernetes.CNI)
	fmt.Printf("  os:      %s\n", p.VM.OSImage)
	fmt.Printf("  masters: %d\n", p.VM.Masters)
	fmt.Printf("  workers: %d\n", p.VM.Workers)
	if p.Libvirt != nil {
		fmt.Printf("  ssh-key: %s\n", p.Libvirt.SSHPrivateKeyPath)
		fmt.Printf("  pool:    %s (%s)\n", p.Libvirt.PoolName, p.Libvirt.PoolPath)
	}

	if errs := p.Validate(); len(errs) > 0 {
		fmt.Println()
		fmt.Fprintln(os.Stderr, "Validation warnings:")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
	}

	// ── action ────────────────────────────────────────────────────────────────

	fmt.Println()
	const (
		actionUpAndSave = "save profile and run env up"
		actionSaveOnly  = "save profile only"
		actionCancel    = "cancel"
	)
	action := w.pick("What next?", []string{actionUpAndSave, actionSaveOnly, actionCancel}, 0)

	if action == actionCancel {
		fmt.Println("Cancelled.")
		return nil
	}

	// ── save profile ──────────────────────────────────────────────────────────

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "infra-lab", "profiles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create profiles dir: %w", err)
	}
	outPath := filepath.Join(dir, name+".yaml")

	if _, err := os.Stat(outPath); err == nil {
		if !w.confirm(fmt.Sprintf("Profile already exists at %s — overwrite?", outPath)) {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0600); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}
	fmt.Printf("\nProfile saved: %s\n", outPath)

	if action == actionSaveOnly {
		fmt.Printf("\nTo run:  ilab env up %s\n", name)
		return nil
	}

	// ── run env up ────────────────────────────────────────────────────────────

	fmt.Printf("\n==> ilab env up %s\n", name)
	if err := runKToolWithProfile(root, p, "up"); err != nil {
		return err
	}
	return writeResolvedProfile(root, p)
}

// ── wizard helpers ────────────────────────────────────────────────────────────

type wizard struct {
	r *bufio.Reader
}

// ask prints a prompt with a default value and returns the user's input (or the default).
func (w *wizard) ask(question, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", question, defaultVal)
	} else {
		fmt.Printf("  %s: ", question)
	}
	line, _ := w.r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}

// askInt asks for an integer value with a default.
func (w *wizard) askInt(question string, defaultVal int) int {
	raw := w.ask(question, strconv.Itoa(defaultVal))
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}

// pick shows a numbered list and returns the selected value.
// Accepts the item number, the item value itself, or Enter for the default.
func (w *wizard) pick(question string, options []string, defaultIdx int) string {
	fmt.Printf("\n  %s\n", question)
	for i, opt := range options {
		if i == defaultIdx {
			fmt.Printf("    %d) %s  (default)\n", i+1, opt)
		} else {
			fmt.Printf("    %d) %s\n", i+1, opt)
		}
	}
	fmt.Printf("  > ")

	line, _ := w.r.ReadString('\n')
	line = strings.TrimSpace(line)

	if line == "" {
		return options[defaultIdx]
	}
	// numeric input
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(options) {
		return options[n-1]
	}
	// exact value input
	for _, opt := range options {
		if opt == line {
			return line
		}
	}
	fmt.Printf("  invalid input — using default: %s\n", options[defaultIdx])
	return options[defaultIdx]
}

// confirm asks a yes/no question; Enter or "y" returns true.
func (w *wizard) confirm(question string) bool {
	fmt.Printf("  %s [Y/n]: ", question)
	line, _ := w.r.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "" || line == "y" || line == "yes"
}
