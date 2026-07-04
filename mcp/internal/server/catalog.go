package server

import (
	"encoding/json"
	"sort"
)

type toolCatalogData struct {
	Tools []toolCatalogEntry `json:"tools"`
}

type toolCatalogEntry struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Category             string   `json:"category"`
	Risk                 string   `json:"risk"`
	Destructive          bool     `json:"destructive"`
	RequiresApproval     bool     `json:"requiresApproval"`
	Source               string   `json:"source"`
	Stage                string   `json:"stage"`
	RequiredCapabilities []string `json:"requiredCapabilities"`
}

func addToolCatalog(handlers map[string]toolHandler) {
	const name = "infra_lab.tool_catalog"
	handlers[name] = toolHandler{
		tool: tool{
			Name:        name,
			Description: "List currently registered infra-lab MCP tools with their capability gates and risk metadata.",
			InputSchema: emptySchema(),
		},
		metadata: toolMetadata{
			RequiredCapabilities: []string{"version.v1", "capabilities.v1"},
			Category:             "introspection",
			Risk:                 "LOW",
			Destructive:          false,
			RequiresApproval:     false,
			Source:               "mcp-internal",
			Stage:                "Stage 1",
		},
		call: func(_ json.RawMessage) (toolResult, error) {
			out, err := encodeSuccessEnvelope("mcp.tool_catalog", buildToolCatalog(handlers))
			if err != nil {
				return toolResult{}, err
			}
			return toolResult{Content: []toolContent{{Type: "text", Text: out}}}, nil
		},
	}
}

func buildToolCatalog(handlers map[string]toolHandler) toolCatalogData {
	entries := make([]toolCatalogEntry, 0, len(handlers))
	for _, handler := range handlers {
		required := append([]string(nil), handler.metadata.RequiredCapabilities...)
		sort.Strings(required)
		entries = append(entries, toolCatalogEntry{
			Name:                 handler.tool.Name,
			Description:          handler.tool.Description,
			Category:             handler.metadata.Category,
			Risk:                 handler.metadata.Risk,
			Destructive:          handler.metadata.Destructive,
			RequiresApproval:     handler.metadata.RequiresApproval,
			Source:               handler.metadata.Source,
			Stage:                handler.metadata.Stage,
			RequiredCapabilities: required,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return toolCatalogData{Tools: entries}
}
