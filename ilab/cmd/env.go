package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

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

func init() {
	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envStatusCmd)
}

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
		fmt.Println("no environments found in state/")
		fmt.Println("hint: run 'HOST_PROFILE=envs/<name>.env make env-up' to create one")
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

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}
