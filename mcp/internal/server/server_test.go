package server

import (
	"encoding/json"
	"testing"
)

func TestHandleInitialize(t *testing.T) {
	s := &Server{
		tools: map[string]toolHandler{},
		bootstrap: bootstrapInfo{
			InfraLabVersion: "dev",
			ContractVersion: supportedContractVersion,
			Capabilities:    map[string]bool{"version.v1": true, "capabilities.v1": true},
		},
		serverName: "infra-lab-mcp",
	}

	resp, ok := s.handle(request{JSONRPC: "2.0", ID: float64(1), Method: "initialize"})
	if !ok {
		t.Fatal("expected response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %#v", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("expected initialize result")
	}
}

func TestToolsListSorted(t *testing.T) {
	s := &Server{
		tools: map[string]toolHandler{
			"z": {tool: tool{Name: "z"}},
			"a": {tool: tool{Name: "a"}},
		},
	}

	resp, ok := s.handle(request{JSONRPC: "2.0", ID: float64(1), Method: "tools/list"})
	if !ok {
		t.Fatal("expected response")
	}

	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Tools []tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Tools) != 2 {
		t.Fatalf("tools len = %d, want 2", len(decoded.Tools))
	}
	if decoded.Tools[0].Name != "a" || decoded.Tools[1].Name != "z" {
		t.Fatalf("tools not sorted: %#v", decoded.Tools)
	}
}

func TestUnknownTool(t *testing.T) {
	s := &Server{tools: map[string]toolHandler{}}
	params := []byte(`{"name":"infra_lab.missing","arguments":{}}`)
	resp := s.callTool(request{JSONRPC: "2.0", ID: float64(1), Params: params})
	if resp.Error == nil {
		t.Fatal("expected JSON-RPC error")
	}
}
