package server

import (
	"encoding/json"
	"testing"
)

func TestToolCatalogRegisteredWithBootstrapCapabilities(t *testing.T) {
	handlers := readOnlyTools(map[string]bool{
		"version.v1":      true,
		"capabilities.v1": true,
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
	handlers := readOnlyTools(map[string]bool{
		"version.v1":          true,
		"capabilities.v1":     true,
		"env.status.v1":       true,
		"profile.validate.v1": true,
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
	handlers := readOnlyTools(map[string]bool{
		"version.v1":      true,
		"capabilities.v1": true,
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

func catalogEntryByName(catalog toolCatalogData, name string) *toolCatalogEntry {
	for i := range catalog.Tools {
		if catalog.Tools[i].Name == name {
			return &catalog.Tools[i]
		}
	}
	return nil
}
