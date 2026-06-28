package cmd

import (
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
