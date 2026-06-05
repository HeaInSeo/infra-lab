package cmd

import (
	"github.com/spf13/cobra"

	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
)

var k8sCmd = &cobra.Command{
	Use:   "k8s",
	Short: "Kubernetes cluster operations",
}

var k8sStatusCmd = &cobra.Command{
	Use:   "status [env-name]",
	Short: "Show Kubernetes node and pod status",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runK8sStatus,
}

func init() {
	k8sCmd.AddCommand(k8sStatusCmd)
}

func runK8sStatus(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	envName := ""
	if len(args) > 0 {
		envName = args[0]
	}
	return lab.PrintK8sStatus(root, envName)
}
