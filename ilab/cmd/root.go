package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ilab",
	Short: "infra-lab operator CLI",
	Long: `ilab is a thin operator interface for infra-lab environments.

It reads OpenTofu state, kubeconfig, and /etc/infra-lab/build.json from
VMs — it does not manage state itself. Source of truth remains the tofu
state, VM runtime, and Kubernetes API.`,
	SilenceErrors: true, // we print the error ourselves in Execute
	SilenceUsage:  true, // don't dump usage on every error
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(envCmd)
	rootCmd.AddCommand(vmCmd)
	rootCmd.AddCommand(k8sCmd)
	rootCmd.AddCommand(doctorCmd)
}
