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
	tools := readOnlyTools(map[string]bool{
		"version.v1":      true,
		"capabilities.v1": true,
	})

	if _, ok := tools["infra_lab.version"]; !ok {
		t.Fatal("expected infra_lab.version")
	}
	if _, ok := tools["infra_lab.capabilities"]; !ok {
		t.Fatal("expected infra_lab.capabilities")
	}
	if _, ok := tools["infra_lab.doctor"]; ok {
		t.Fatal("did not expect infra_lab.doctor without doctor.v1")
	}
}
