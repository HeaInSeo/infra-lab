package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/HeaInSeo/infra-lab/ilab/internal/output"
)

var jsonOutput bool

var jsonCapableCommands = map[string]bool{
	"version":          true,
	"capabilities":     true,
	"doctor":           true,
	"env.list":         true,
	"env.status":       true,
	"profile.list":     true,
	"profile.show":     true,
	"profile.validate": true,
	"k8s.status":       true,
	"vm.list":          true,
	"vm.version":       true,
}

var rootCmd = &cobra.Command{
	Use:   "ilab",
	Short: "infra-lab operator CLI",
	Long: `ilab is a thin operator interface for infra-lab environments.

It reads OpenTofu state, kubeconfig, and /etc/infra-lab/build.json from
VMs — it does not manage state itself. Source of truth remains the tofu
state, VM runtime, and Kubernetes API.`,
	SilenceErrors: true, // we print the error ourselves in Execute
	SilenceUsage:  true, // don't dump usage on every error
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if !wantsJSON() {
			return nil
		}
		command := contractCommandName(cmd)
		if jsonCapableCommands[command] {
			return nil
		}
		return output.NewError(
			"CAPABILITY_UNSUPPORTED",
			fmt.Sprintf("%s does not support --json yet", command),
			output.ExitDomain,
		)
	},
}

func Execute() {
	cmd, err := rootCmd.ExecuteC()
	if err != nil {
		exitCode := exitCodeFor(err)
		if wantsJSON() {
			command := contractCommandName(cmd)
			env := output.Failure(command, errorInfosFor(err))
			if writeErr := output.WriteJSON(os.Stdout, env); writeErr != nil {
				fmt.Fprintf(os.Stderr, "Error: write JSON response: %v\n", writeErr)
				os.Exit(output.ExitRuntime)
			}
			os.Exit(exitCode)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(exitCode)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "print machine-readable JSON")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(capabilitiesCmd)
	rootCmd.AddCommand(envCmd)
	rootCmd.AddCommand(vmCmd)
	rootCmd.AddCommand(k8sCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(profileCmd)
}

func wantsJSON() bool {
	return jsonOutput
}

func contractCommandName(cmd *cobra.Command) string {
	if cmd == nil {
		return "unknown"
	}
	path := cmd.CommandPath()
	if path == "" {
		return "unknown"
	}
	parts := strings.Fields(path)
	if len(parts) == 0 {
		return "unknown"
	}
	if parts[0] == "ilab" {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return "root"
	}
	return strings.Join(parts, ".")
}

func errorInfoFor(err error) output.ErrorInfo {
	infos := errorInfosFor(err)
	if len(infos) == 0 {
		return output.ErrorInfo{Code: "COMMAND_FAILED", Message: "command failed"}
	}
	return infos[0]
}

func errorInfosFor(err error) []output.ErrorInfo {
	var contractErr *output.ContractError
	if errors.As(err, &contractErr) {
		return contractErr.ErrorInfos()
	}
	return []output.ErrorInfo{{
		Code:    "COMMAND_FAILED",
		Message: err.Error(),
	}}
}

func exitCodeFor(err error) int {
	var contractErr *output.ContractError
	if errors.As(err, &contractErr) {
		return contractErr.ExitCode()
	}
	return output.ExitDomain
}
