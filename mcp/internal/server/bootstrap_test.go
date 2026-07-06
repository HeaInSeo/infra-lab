package server

import "testing"

func TestValidateEnvelope(t *testing.T) {
	raw := `{"ok":true,"command":"version","contractVersion":"infra-lab.contract/v1","data":{},"warnings":[],"errors":[]}`
	if err := validateEnvelope(raw, "version"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateEnvelopeRejectsWrongCommand(t *testing.T) {
	raw := `{"ok":true,"command":"doctor","contractVersion":"infra-lab.contract/v1","data":{},"warnings":[],"errors":[]}`
	if err := validateEnvelope(raw, "version"); err == nil {
		t.Fatal("expected wrong command error")
	}
}

func TestReadOnlyToolsFilteredByCapability(t *testing.T) {
	tools := readOnlyTools(bootstrapInfo{
		InfraLabVersion: "dev",
		ContractVersion: supportedContractVersion,
		Capabilities: map[string]bool{
			"version.v1":      true,
			"capabilities.v1": true,
		},
	})

	if _, ok := tools["setup_check"]; !ok {
		t.Fatal("expected setup_check")
	}
	if _, ok := tools["what_can_i_do"]; !ok {
		t.Fatal("expected what_can_i_do")
	}
	if _, ok := tools["version"]; !ok {
		t.Fatal("expected version")
	}
	if _, ok := tools["capabilities"]; !ok {
		t.Fatal("expected capabilities")
	}
	if _, ok := tools["doctor"]; ok {
		t.Fatal("did not expect doctor without doctor.v1")
	}
}
