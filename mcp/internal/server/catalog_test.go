package server

import (
	"encoding/json"
	"testing"
)

func TestToolCatalogIncludesMutationTools(t *testing.T) {
	handlers := readOnlyTools(fullCapabilityBootstrap())
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
		Capabilities:    map[string]bool{"version.v1": true, "capabilities.v1": true, "env.status.v1": true, "profile.validate.v1": true},
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

func TestToolCatalogRegisteredWithBootstrapCapabilities(t *testing.T) {
	handlers := readOnlyTools(bootstrapInfo{
		InfraLabVersion: "dev",
		ContractVersion: supportedContractVersion,
		Capabilities: map[string]bool{
			"version.v1":      true,
			"capabilities.v1": true,
		},
	})

	handler, ok := handlers["infra_lab.tool_catalog"]
	if !ok {
		t.Fatal("expected infra_lab.tool_catalog")
	}
	if handler.metadata.Category != "introspection" {
		t.Fatalf("category = %q, want introspection", handler.metadata.Category)
	}
}

func TestBuildToolCatalogIncludesCapabilityGates(t *testing.T) {
	handlers := readOnlyTools(bootstrapInfo{
		InfraLabVersion: "dev",
		ContractVersion: supportedContractVersion,
		Capabilities: map[string]bool{
			"version.v1":          true,
			"capabilities.v1":     true,
			"env.status.v1":       true,
			"profile.validate.v1": true,
		},
	})

	catalog := buildToolCatalog(handlers)
	version := catalogEntryByName(catalog, "infra_lab.version")
	if version == nil {
		t.Fatal("missing infra_lab.version")
	}
	if len(version.RequiredCapabilities) != 1 || version.RequiredCapabilities[0] != "version.v1" {
		t.Fatalf("version required capabilities = %#v", version.RequiredCapabilities)
	}

	envUp := catalogEntryByName(catalog, "infra_lab.env_up_commit")
	if envUp == nil {
		t.Fatal("missing infra_lab.env_up_commit")
	}
	if envUp.Category != "approved_env_up" || envUp.Risk != "HIGH" || envUp.Destructive {
		t.Fatalf("unexpected env_up metadata: %#v", envUp)
	}
	if !envUp.RequiresApproval {
		t.Fatal("env_up_commit should require approval")
	}
	if len(envUp.RequiredCapabilities) != 2 ||
		envUp.RequiredCapabilities[0] != "env.status.v1" ||
		envUp.RequiredCapabilities[1] != "profile.validate.v1" {
		t.Fatalf("env_up required capabilities = %#v", envUp.RequiredCapabilities)
	}

	if got := catalogEntryByName(catalog, "infra_lab.doctor"); got != nil {
		t.Fatalf("doctor should not be cataloged without doctor.v1: %#v", got)
	}
}

func TestToolCatalogCallReturnsEnvelope(t *testing.T) {
	handlers := readOnlyTools(bootstrapInfo{
		InfraLabVersion: "dev",
		ContractVersion: supportedContractVersion,
		Capabilities: map[string]bool{
			"version.v1":      true,
			"capabilities.v1": true,
		},
	})

	result, err := handlers["infra_lab.tool_catalog"].call(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content len = %d, want 1", len(result.Content))
	}

	var env struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
		Data    struct {
			Tools []toolCatalogEntry `json:"tools"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK || env.Command != "mcp.tool_catalog" {
		t.Fatalf("unexpected envelope: %#v", env)
	}
	if len(env.Data.Tools) == 0 {
		t.Fatal("expected tools")
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

func catalogEntryByName(catalog toolCatalogData, name string) *toolCatalogEntry {
	for i := range catalog.Tools {
		if catalog.Tools[i].Name == name {
			return &catalog.Tools[i]
		}
	}
	return nil
}

func fullCapabilityBootstrap() bootstrapInfo {
	return bootstrapInfo{
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
	}
}
