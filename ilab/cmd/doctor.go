package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose infra-lab environment state",
	RunE:  runDoctor,
}

func runDoctor(_ *cobra.Command, _ []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ infra-lab root: %v\n", err)
		fmt.Fprintln(os.Stderr, "  set INFRA_LAB_ROOT or run from inside the repository")
		return err
	}
	fmt.Printf("✓ infra-lab root: %s\n\n", root)

	// ── Managed environments ────────────────────────────────────────────────
	fmt.Println("Managed environments (state/):")
	envs, err := lab.ListEnvs(root)
	if err != nil {
		fmt.Println("  ✗ could not read state/:", err)
	} else if len(envs) == 0 {
		fmt.Println("  (none)")
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  ENV\tBACKEND\tCNI\tCREATED")
		for _, e := range envs {
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", e.Name, e.Backend, e.CNI, e.CreatedAt)
		}
		_ = w.Flush()
	}
	fmt.Println()

	// ── Legacy files ────────────────────────────────────────────────────────
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

	// ── VMs ─────────────────────────────────────────────────────────────────
	fmt.Println("VMs (all backends):")
	vms, err := lab.ListAllVMs(root)
	if err != nil {
		fmt.Println("  ✗", err)
	} else if len(vms) == 0 {
		fmt.Println("  (none found)")
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  NAME\tMANAGED\tENV\tSTATE\tIPv4")
		for _, vm := range vms {
			managed := "no"
			envName := "-"
			if vm.Managed {
				managed = "yes"
				envName = vm.EnvName
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n",
				vm.Name, managed, envName, vm.State, vm.IPv4)
		}
		_ = w.Flush()
	}
	fmt.Println()

	// ── Recommendation ──────────────────────────────────────────────────────
	fmt.Println("Recommendation:")
	if len(envs) == 0 && len(legacy) > 0 {
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
