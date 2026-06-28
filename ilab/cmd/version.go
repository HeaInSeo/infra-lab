package cmd

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/HeaInSeo/infra-lab/ilab/internal/output"
)

var (
	infraLabVersion = "dev"
	gitCommit       = "unknown"
	buildDate       = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show infra-lab CLI version",
	Args:  cobra.NoArgs,
	RunE:  runVersion,
}

var capabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "Show supported JSON contract capabilities",
	Args:  cobra.NoArgs,
	RunE:  runCapabilities,
}

type versionData struct {
	InfraLabVersion string `json:"infraLabVersion"`
	GitCommit       string `json:"gitCommit"`
	BuildDate       string `json:"buildDate"`
}

type capabilitiesData struct {
	InfraLabVersion string   `json:"infraLabVersion"`
	ContractVersion string   `json:"contractVersion"`
	Capabilities    []string `json:"capabilities"`
}

func runVersion(_ *cobra.Command, _ []string) error {
	data := currentVersionData()
	if wantsJSON() {
		return output.WriteJSON(os.Stdout, output.Success("version", data))
	}

	fmt.Printf("infra-lab version: %s\n", data.InfraLabVersion)
	fmt.Printf("git commit: %s\n", data.GitCommit)
	fmt.Printf("build date: %s\n", data.BuildDate)
	return nil
}

func runCapabilities(_ *cobra.Command, _ []string) error {
	data := currentCapabilitiesData()
	if wantsJSON() {
		return output.WriteJSON(os.Stdout, output.Success("capabilities", data))
	}

	fmt.Printf("contract version: %s\n", data.ContractVersion)
	fmt.Println("capabilities:")
	for _, capability := range data.Capabilities {
		fmt.Printf("  - %s\n", capability)
	}
	return nil
}

func currentVersionData() versionData {
	data := versionData{
		InfraLabVersion: infraLabVersion,
		GitCommit:       gitCommit,
		BuildDate:       buildDate,
	}
	if data.GitCommit == "unknown" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" && setting.Value != "" {
					data.GitCommit = setting.Value
					break
				}
			}
		}
	}
	return data
}

func currentCapabilitiesData() capabilitiesData {
	return capabilitiesData{
		InfraLabVersion: currentVersionData().InfraLabVersion,
		ContractVersion: output.ContractVersion,
		Capabilities: []string{
			"version.v1",
			"capabilities.v1",
			"doctor.v1",
			"env.list.v1",
			"env.status.v1",
			"profile.list.v1",
			"profile.show.v1",
			"profile.validate.v1",
			"k8s.status.v1",
			"vm.list.v1",
			"vm.version.v1",
		},
	}
}
