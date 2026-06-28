package cmd

import (
	"errors"
	"reflect"
	"testing"

	"github.com/HeaInSeo/infra-lab/ilab/internal/output"
)

func TestCurrentCapabilitiesData(t *testing.T) {
	data := currentCapabilitiesData()

	if data.ContractVersion != output.ContractVersion {
		t.Fatalf("contractVersion = %q, want %q", data.ContractVersion, output.ContractVersion)
	}

	want := []string{"version.v1", "capabilities.v1"}
	if !reflect.DeepEqual(data.Capabilities, want) {
		t.Fatalf("capabilities = %#v, want %#v", data.Capabilities, want)
	}
}

func TestContractCommandName(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "root child", args: []string{"version"}, want: "version"},
		{name: "nested child", args: []string{"profile", "validate"}, want: "profile.validate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, err := rootCmd.Find(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if got := contractCommandName(cmd); got != tt.want {
				t.Fatalf("contractCommandName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorInfoForContractError(t *testing.T) {
	err := output.NewError("CAPABILITY_UNSUPPORTED", "not supported", output.ExitDomain)
	info := errorInfoFor(err)

	if info.Code != "CAPABILITY_UNSUPPORTED" {
		t.Fatalf("code = %q, want CAPABILITY_UNSUPPORTED", info.Code)
	}
	if exit := exitCodeFor(err); exit != output.ExitDomain {
		t.Fatalf("exit = %d, want %d", exit, output.ExitDomain)
	}
}

func TestErrorInfoForGenericError(t *testing.T) {
	err := errors.New("boom")
	info := errorInfoFor(err)

	if info.Code != "COMMAND_FAILED" {
		t.Fatalf("code = %q, want COMMAND_FAILED", info.Code)
	}
	if exit := exitCodeFor(err); exit != output.ExitDomain {
		t.Fatalf("exit = %d, want %d", exit, output.ExitDomain)
	}
}
