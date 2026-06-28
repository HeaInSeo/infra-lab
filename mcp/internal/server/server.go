package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

type Server struct {
	tools map[string]toolHandler
}

func New() *Server {
	return &Server{tools: readOnlyTools()}
}

func (s *Server) Serve(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	encoder := json.NewEncoder(w)

	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			if err := encoder.Encode(errorResponse(nil, -32700, "parse error")); err != nil {
				return err
			}
			continue
		}

		resp, ok := s.handle(req)
		if !ok {
			continue
		}
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) handle(req request) (response, bool) {
	switch req.Method {
	case "initialize":
		return okResponse(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]any{
				"name":    "infra-lab-mcp",
				"version": "dev",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
		}), true
	case "notifications/initialized":
		return response{}, false
	case "tools/list":
		tools := make([]tool, 0, len(s.tools))
		for _, handler := range s.tools {
			tools = append(tools, handler.tool)
		}
		sort.Slice(tools, func(i, j int) bool {
			return tools[i].Name < tools[j].Name
		})
		return okResponse(req.ID, map[string]any{"tools": tools}), true
	case "tools/call":
		return s.callTool(req), true
	default:
		return errorResponse(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method)), true
	}
}

func (s *Server) callTool(req request) response {
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "invalid tools/call params")
	}
	handler, ok := s.tools[params.Name]
	if !ok {
		return errorResponse(req.ID, -32602, "unknown tool: "+params.Name)
	}
	result, err := handler.call(params.Arguments)
	if err != nil {
		return okResponse(req.ID, toolResult{
			Content: []toolContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		})
	}
	return okResponse(req.ID, result)
}

func okResponse(id any, result any) response {
	return response{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id any, code int, message string) response {
	return response{JSONRPC: "2.0", ID: id, Error: &responseError{Code: code, Message: message}}
}
