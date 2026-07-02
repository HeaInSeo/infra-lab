package server

import (
	"encoding/json"
	"testing"
)

func TestToolCatalogIncludesMutationTools(t *testing.T) {
	handlers := readOnlyTools(bootstrapInfo{
		InfraLabVersion: "dev",
		ContractVersion: supportedContractVersion,
		Capabilities: map[string]bool{
			"version.v1":          true,
			"capabilities.v1":     true,
			"doctor.v1":           true,
			"env.list.v1":         true,
			"env.status.v1":       true,
			"k8s.status.v1":       true,
			"vm.list.v1":          true,
			"vm.version.v1":       true,
			"profile.list.v1":     true,
			"profile.show.v1":     true,
			"profile.validate.v1": true,
		},
	})
	catalog := toolCatalogFromHandlers(handlers)

	if catalog.Summary.ApprovedMutation == 0 {
		t.Fatal("expected approved mutation tools")
	}
	if catalog.Summary.Destructive == 0 {
		t.Fatal("expected destructive tools")
	}
	if !catalogHasTool(catalog, "infra_lab.env_up_prepare") {
		t.Fatal("expected env_up_prepare in catalog")
	}
	if !catalogHasTool(catalog, "infra_lab.env_down_commit") {
		t.Fatal("expected env_down_commit in catalog")
	}
}

func TestWhatCanIDoJSON(t *testing.T) {
	handlers := readOnlyTools(bootstrapInfo{
		InfraLabVersion: "dev",
		ContractVersion: supportedContractVersion,
		Capabilities:    map[string]bool{"env.status.v1": true, "profile.validate.v1": true},
	})
	raw, err := whatCanIDoJSON(handlers)
	if err != nil {
		t.Fatal(err)
	}
	var env whatCanIDoEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK {
		t.Fatal("expected ok")
	}
	if env.Command != "mcp.what_can_i_do" {
		t.Fatalf("command = %q", env.Command)
	}
	if env.Data.Tools.Summary.Total == 0 {
		t.Fatal("expected tool summary")
	}
}

func catalogHasTool(catalog toolCatalog, name string) bool {
	for _, category := range catalog.Categories {
		for _, tool := range category.Tools {
			if tool.Name == name {
				return true
			}
		}
	}
	return false
}
