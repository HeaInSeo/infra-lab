package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose infra-lab environment state and prerequisites",
	RunE:  runDoctor,
}

// prereq describes a host tool that infra-lab depends on.
type prereq struct {
	name     string
	cmd      string
	scope    string // what this tool is needed for
	required bool   // false = optional but noted
}

var prereqs = []prereq{
	// Always required
	{"git", "git", "all — root detection, build metadata", true},
	{"tofu", "tofu", "all — OpenTofu (>= 1.6)", true},
	{"kubectl", "kubectl", "cluster access, addon management", true},
	// Installed by host-setup on Rocky/RHEL
	{"python3", "python3", "lint-yaml (make check)", false},
	{"helm", "helm", "Cilium addon installation", false},
	// Backend-specific
	{"multipass", "multipass", "multipass backend", false},
	{"virsh", "virsh", "libvirt backend", false},
	{"qemu-img", "qemu-img", "libvirt backend", false},
	// Optional helpers
	{"jq", "jq", "multipass state reconcile", false},
	// CLI build
	{"go", "go", "ilab CLI build (>= 1.22)", false},
}

func runDoctor(_ *cobra.Command, _ []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ infra-lab root: %v\n", err)
		fmt.Fprintln(os.Stderr, "  set INFRA_LAB_ROOT or run from inside the repository")
		return err
	}
	fmt.Printf("✓ infra-lab root: %s\n\n", root)

	// ── Prerequisites ────────────────────────────────────────────────────────
	fmt.Println("Prerequisites:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	missing := 0
	for _, p := range prereqs {
		path, err := exec.LookPath(p.cmd)
		if err != nil {
			if p.required {
				fmt.Fprintf(w, "  ✗ %-10s\tnot found\t%s  [REQUIRED]\n", p.name, p.scope)
				missing++
			} else {
				fmt.Fprintf(w, "  - %-10s\tnot found\t%s\n", p.name, p.scope)
			}
		} else {
			fmt.Fprintf(w, "  ✓ %-10s\t%s\t%s\n", p.name, path, p.scope)
		}
	}
	_ = w.Flush()
	if missing > 0 {
		fmt.Printf("\n  %d required tool(s) missing. Run 'host-setup' or install manually.\n", missing)
	}
	fmt.Println()

	// ── Managed environments ─────────────────────────────────────────────────
	fmt.Println("Managed environments (state/):")
	envs, err := lab.ListEnvs(root)
	if err != nil {
		fmt.Println("  ✗ could not read state/:", err)
	} else if len(envs) == 0 {
		fmt.Println("  (none)")
	} else {
		ew := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(ew, "  ENV\tBACKEND\tCNI\tCREATED")
		for _, e := range envs {
			fmt.Fprintf(ew, "  %s\t%s\t%s\t%s\n", e.Name, e.Backend, e.CNI, e.CreatedAt)
		}
		_ = ew.Flush()
	}
	fmt.Println()

	// ── Legacy files ─────────────────────────────────────────────────────────
	fmt.Println("Legacy files (pre-Phase-2):")
	legacy := lab.DetectLegacyFiles(root)
	if len(legacy) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, f := range legacy {
			fmt.Println(" ", f)
		}
		fmt.Println()
		fmt.Println("  These files are from a pre-Phase-2 environment.")
		fmt.Println("  They will not be modified automatically.")
	}
	fmt.Println()

	// ── VMs ──────────────────────────────────────────────────────────────────
	fmt.Println("VMs (all backends):")
	vms, err := lab.ListAllVMs(root)
	if err != nil {
		fmt.Println("  ✗", err)
	} else if len(vms) == 0 {
		fmt.Println("  (none found)")
	} else {
		vw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(vw, "  NAME\tMANAGED\tENV\tSTATE\tIPv4")
		for _, vm := range vms {
			managed := "no"
			envName := "-"
			if vm.Managed {
				managed = "yes"
				envName = vm.EnvName
			}
			fmt.Fprintf(vw, "  %s\t%s\t%s\t%s\t%s\n",
				vm.Name, managed, envName, vm.State, vm.IPv4)
		}
		_ = vw.Flush()
	}
	fmt.Println()

	// ── Recommendation ────────────────────────────────────────────────────────
	fmt.Println("Recommendation:")
	if missing > 0 {
		fmt.Println("  Install missing required tools, then rerun 'ilab doctor'.")
	} else if len(envs) == 0 && len(legacy) > 0 {
		fmt.Println("  Legacy environment detected. Keep it as-is.")
		fmt.Println("  Create a new managed environment:")
		fmt.Println("    cp envs/multipass-flannel.env.example envs/multipass-flannel.env")
		fmt.Println("    HOST_PROFILE=envs/multipass-flannel.env make env-up")
	} else if len(envs) == 0 {
		fmt.Println("  No environments yet. Create one:")
		fmt.Println("    cp envs/multipass-flannel.env.example envs/multipass-flannel.env")
		fmt.Println("    HOST_PROFILE=envs/multipass-flannel.env make env-up")
	} else {
		fmt.Printf("  %d managed environment(s) found. Run 'ilab env status' to inspect.\n", len(envs))
	}

	return nil
}
