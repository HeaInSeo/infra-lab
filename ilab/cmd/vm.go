package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
)

var vmListAll bool

var vmCmd = &cobra.Command{
	Use:   "vm",
	Short: "VM operations",
}

var vmListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all VMs across environments",
	RunE:  runVMList,
}

var vmVersionCmd = &cobra.Command{
	Use:   "version <vm-name>",
	Short: "Read /etc/infra-lab/build.json from a VM",
	Args:  cobra.ExactArgs(1),
	RunE:  runVMVersion,
}

var vmSSHCmd = &cobra.Command{
	Use:   "ssh <vm-name>",
	Short: "Open an interactive shell on a VM",
	Args:  cobra.ExactArgs(1),
	RunE:  runVMSSH,
}

func init() {
	vmListCmd.Flags().BoolVar(&vmListAll, "all", false, "include unmanaged VMs from all backends")
	vmCmd.AddCommand(vmListCmd)
	vmCmd.AddCommand(vmVersionCmd)
	vmCmd.AddCommand(vmSSHCmd)
}

func runVMList(_ *cobra.Command, _ []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}

	if vmListAll {
		return runVMListAll(root)
	}

	// Default: managed VMs only, grouped by environment.
	envs, err := lab.ListEnvs(root)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VM\tENV\tBACKEND\tSTATE\tIPv4")
	found := false
	for _, e := range envs {
		vms, err := e.ListVMs()
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: %s: %v\n", e.Name, err)
			continue
		}
		for _, vm := range vms {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				vm.Name, e.Name, e.Backend, vm.State, vm.IPv4)
			found = true
		}
	}
	if !found {
		fmt.Fprintln(w, "(no managed VMs found; use --all to see unmanaged VMs)")
	}
	return w.Flush()
}

func runVMListAll(root string) error {
	vms, err := lab.ListAllVMs(root)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VM\tMANAGED\tENV\tSTATE\tIPv4")
	if len(vms) == 0 {
		fmt.Fprintln(w, "(no VMs found)")
	}
	for _, vm := range vms {
		managed := "no"
		envName := "-"
		if vm.Managed {
			managed = "yes"
			envName = vm.EnvName
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			vm.Name, managed, envName, vm.State, vm.IPv4)
	}
	return w.Flush()
}

func runVMVersion(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	vmName := args[0]
	env, err := lab.FindEnvForVM(root, vmName)
	if err != nil {
		return err
	}
	info, err := env.ReadBuildJSON(vmName)
	if err != nil {
		return err
	}
	info.Print(os.Stdout)
	return nil
}

func runVMSSH(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	vmName := args[0]
	env, err := lab.FindEnvForVM(root, vmName)
	if err != nil {
		return err
	}
	return env.SSH(vmName)
}
