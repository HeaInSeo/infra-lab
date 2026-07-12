package server

import (
	"encoding/json"
	"sort"
)

type toolCatalog struct {
	Summary    toolCatalogSummary    `json:"summary"`
	Categories []toolCatalogCategory `json:"categories"`
	Flows      []toolFlow            `json:"flows"`
}

type toolCatalogSummary struct {
	Total            int `json:"total"`
	ReadOnly         int `json:"readOnly"`
	Evidence         int `json:"evidence"`
	PlanOnly         int `json:"planOnly"`
	ProfileWrite     int `json:"profileWrite"`
	ApprovedMutation int `json:"approvedMutation"`
	Destructive      int `json:"destructive"`
	Operation        int `json:"operation"`
}

type toolCatalogCategory struct {
	Name        string            `json:"name"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Tools       []toolCatalogItem `json:"tools"`
}

type toolCatalogItem struct {
	Name             string `json:"name"`
	Purpose          string `json:"purpose"`
	Mutates          bool   `json:"mutates"`
	Destructive      bool   `json:"destructive"`
	RequiresApproval bool   `json:"requiresApproval"`
}

type toolFlow struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
}

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

type whatCanIDoEnvelope struct {
	OK              bool             `json:"ok"`
	Command         string           `json:"command"`
	ContractVersion string           `json:"contractVersion"`
	Data            whatCanIDoData   `json:"data"`
	Warnings        []setupFinding   `json:"warnings"`
	Errors          []setupErrorInfo `json:"errors"`
}

type whatCanIDoData struct {
	Tools              toolCatalog `json:"tools"`
	Safety             []string    `json:"safety"`
	RecommendedPrompts []string    `json:"recommendedPrompts"`
}

func addWhatCanIDoTool(handlers map[string]toolHandler) {
	handlers["what_can_i_do"] = toolHandler{
		tool: tool{
			Name:        "what_can_i_do",
			Description: "Explain the current infra-lab MCP capabilities, categorized tools, safe execution flows, and recommended prompts.",
			InputSchema: emptySchema(),
		},
		metadata: toolMetadata{
			RequiredCapabilities: []string{"version.v1", "capabilities.v1"},
			Category:             "read_only",
			Risk:                 "LOW",
			Source:               "mcp-internal",
			Stage:                "Stage 1",
		},
		call: func(_ json.RawMessage) (toolResult, error) {
			raw, err := whatCanIDoJSON(handlers)
			if err != nil {
				return toolResult{}, err
			}
			return toolResult{Content: []toolContent{{Type: "text", Text: raw}}}, nil
		},
	}
}

func addToolCatalog(handlers map[string]toolHandler) {
	handlers["tool_catalog"] = toolHandler{
		tool: tool{
			Name:        "tool_catalog",
			Description: "List currently registered infra-lab MCP tools with their capability gates and risk metadata.",
			InputSchema: emptySchema(),
		},
		metadata: toolMetadata{
			RequiredCapabilities: []string{"version.v1", "capabilities.v1"},
			Category:             "introspection",
			Risk:                 "LOW",
			Source:               "mcp-internal",
			Stage:                "Stage 1",
		},
		call: func(_ json.RawMessage) (toolResult, error) {
			raw, err := encodeSuccessEnvelope("mcp.tool_catalog", buildToolCatalog(handlers))
			if err != nil {
				return toolResult{}, err
			}
			return toolResult{Content: []toolContent{{Type: "text", Text: raw}}}, nil
		},
	}
}

func handlersForCatalog(info bootstrapInfo, handlers map[string]toolHandler) map[string]toolHandler {
	if handlers != nil {
		return handlers
	}
	return readOnlyTools(info)
}

func whatCanIDoJSON(handlers map[string]toolHandler) (string, error) {
	env := whatCanIDoEnvelope{
		OK:              true,
		Command:         "mcp.what_can_i_do",
		ContractVersion: supportedContractVersion,
		Data: whatCanIDoData{
			Tools: toolCatalogFromHandlers(handlers),
			Safety: []string{
				"infra-lab MCP never exposes raw shell, raw kubectl, raw ssh, raw tofu, or raw script execution.",
				"Mutation and destructive tools must use prepare -> operation_approve -> commit.",
				"env_up/env_down/env_clean/env_rebuild commit changes VM/runtime state and should run on the remote lab host.",
				"Use plan-only tools before prepare whenever possible.",
			},
			RecommendedPrompts: []string{
				"infra-lab MCP로 what_can_i_do를 실행하고 가능한 작업을 카테고리별로 보여줘.",
				"infra-lab MCP로 새 env 생성 계획만 만들고 risk와 blocked 여부를 알려줘. 실행하지 마.",
				"infra-lab MCP로 env_up_prepare를 만들고 operationId, target, risk, steps를 보여줘. commit은 하지 마.",
				"방금 만든 operation을 승인하고, 승인된 operation만 commit해줘.",
			},
		},
		Warnings: []setupFinding{},
		Errors:   []setupErrorInfo{},
	}
	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func toolCatalogFromHandlers(handlers map[string]toolHandler) toolCatalog {
	categories := []toolCatalogCategory{
		{Name: "readOnly", Title: "조회/진단", Description: "state를 변경하지 않는 기본 조회 도구"},
		{Name: "evidence", Title: "증거 수집", Description: "진단 근거를 수집하고 health를 요약하는 도구"},
		{Name: "planOnly", Title: "계획 전용", Description: "실행 없이 변경 계획과 위험도를 계산하는 도구"},
		{Name: "profileWrite", Title: "Profile 파일 작성", Description: "VM/Kubernetes를 건드리지 않고 profile만 저장하는 도구"},
		{Name: "approvedMutation", Title: "승인형 실행", Description: "prepare -> approve -> commit 흐름을 따르는 실행 도구"},
		{Name: "destructive", Title: "파괴적 승인형 실행", Description: "삭제/정리/재빌드처럼 높은 위험의 승인형 실행 도구"},
		{Name: "operation", Title: "Operation 관리", Description: "승인, 취소, 상태, 로그, lock을 관리하는 도구"},
	}

	index := map[string]int{}
	for i := range categories {
		index[categories[i].Name] = i
	}
	for _, handler := range handlers {
		item := classifyTool(handler)
		if item.Name == "" {
			continue
		}
		cat, ok := index[summaryCategoryName(handler.metadata.Category)]
		if !ok {
			continue
		}
		categories[cat].Tools = append(categories[cat].Tools, item)
	}
	for i := range categories {
		sort.Slice(categories[i].Tools, func(a, b int) bool {
			return categories[i].Tools[a].Name < categories[i].Tools[b].Name
		})
	}

	summary := toolCatalogSummary{}
	for _, category := range categories {
		count := len(category.Tools)
		summary.Total += count
		switch category.Name {
		case "readOnly":
			summary.ReadOnly = count
		case "evidence":
			summary.Evidence = count
		case "planOnly":
			summary.PlanOnly = count
		case "profileWrite":
			summary.ProfileWrite = count
		case "approvedMutation":
			summary.ApprovedMutation = count
		case "destructive":
			summary.Destructive = count
		case "operation":
			summary.Operation = count
		}
	}

	return toolCatalog{
		Summary:    summary,
		Categories: categories,
		Flows: []toolFlow{
			{
				Name:        "addonInstall",
				Title:       "Addon install",
				Description: "기존 env에 addon을 승인 기반으로 설치한다.",
				Steps:       []string{"addon_install_plan", "addon_install_prepare", "operation_approve", "addon_install_commit", "operation_status"},
			},
			{
				Name:        "envUp",
				Title:       "New env up",
				Description: "존재하지 않는 env를 승인 기반으로 생성한다.",
				Steps:       []string{"up_plan", "env_up_prepare", "operation_approve", "env_up_commit", "operation_status"},
			},
			{
				Name:        "envDownClean",
				Title:       "Env down and clean",
				Description: "기존 env를 승인 기반으로 내리고 state를 정리한다.",
				Steps:       []string{"down_plan", "env_down_prepare", "operation_approve", "env_down_commit", "env_clean_prepare", "operation_approve", "env_clean_commit"},
			},
			{
				Name:        "envRebuild",
				Title:       "Env rebuild",
				Description: "기존 env를 승인 기반으로 재빌드한다.",
				Steps:       []string{"rebuild_plan", "env_rebuild_prepare", "operation_approve", "env_rebuild_commit", "operation_status"},
			},
			{
				Name:        "containerImageBuildPush",
				Title:       "Container image build and push",
				Description: "허용된 source context에서 이미지를 빌드하고 registry tag로 push한다.",
				Steps:       []string{"container_image_build_push_prepare", "operation_approve", "container_image_build_push_commit", "operation_status", "operation_logs"},
			},
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

func summaryCategoryName(category string) string {
	switch category {
	case "read_only", "introspection":
		return "readOnly"
	case "evidence":
		return "evidence"
	case "plan":
		return "planOnly"
	case "profile_write":
		return "profileWrite"
	case "approved_mutation", "approved_env_up":
		return "approvedMutation"
	case "destructive_execution":
		return "destructive"
	case "operation":
		return "operation"
	default:
		return ""
	}
}

func classifyTool(handler toolHandler) toolCatalogItem {
	category := summaryCategoryName(handler.metadata.Category)
	if category == "" {
		return toolCatalogItem{}
	}
	return toolCatalogItem{
		Name:             handler.tool.Name,
		Purpose:          toolPurpose(handler.tool.Name),
		Mutates:          category == "profileWrite" || category == "approvedMutation" || category == "destructive" || handler.tool.Name == "operation_approve" || handler.tool.Name == "operation_cancel" || handler.tool.Name == "operation_unlock_stale",
		Destructive:      handler.metadata.Destructive,
		RequiresApproval: handler.metadata.RequiresApproval,
	}
}

func toolPurpose(name string) string {
	purposes := map[string]string{
		"setup_check":                        "MCP readiness와 client 등록 가이드를 확인한다.",
		"what_can_i_do":                      "현재 MCP로 가능한 작업을 카테고리별로 설명한다.",
		"tool_catalog":                       "현재 등록된 MCP tool의 capability gate와 risk metadata를 조회한다.",
		"version":                            "infra-lab 버전 정보를 조회한다.",
		"capabilities":                       "ilab JSON capability 목록을 조회한다.",
		"doctor":                             "host prerequisite와 local state를 진단한다.",
		"env_list":                           "관리 중인 env 목록을 조회한다.",
		"env_status":                         "env 상태를 조회한다.",
		"k8s_status":                         "Kubernetes node/pod 상태를 조회한다.",
		"vm_list":                            "관리 VM 목록을 조회한다.",
		"vm_list_all":                        "관리/비관리 VM 목록을 조회한다.",
		"vm_version":                         "VM 내부 infra-lab build metadata와 guest OS(/etc/os-release) 정보를 조회한다.",
		"profile_list":                       "사용 가능한 profile 목록을 조회한다.",
		"profile_show":                       "profile 정규화 내용을 조회한다.",
		"profile_validate":                   "profile 유효성을 검증한다.",
		"collect_snapshot":                   "env/profile/VM/Kubernetes 증거를 수집한다.",
		"summarize_health":                   "snapshot 기반 health를 요약한다.",
		"up_plan":                            "새 env 생성 계획만 만든다.",
		"down_plan":                          "env down 계획만 만든다.",
		"rebuild_plan":                       "env rebuild 계획만 만든다.",
		"addon_install_plan":                 "addon install 계획만 만든다.",
		"addon_uninstall_plan":               "addon uninstall 계획만 만든다.",
		"profile_clone":                      "profile을 user profile dir로 복제한다.",
		"profile_save_as":                    "새 profile을 user profile dir에 저장한다.",
		"profile_validate_and_save":          "profile을 검증하고 저장한다.",
		"addon_install_prepare":              "addon install operation을 준비한다.",
		"addon_install_commit":               "승인된 addon install operation을 실행한다.",
		"env_up_prepare":                     "새 env 생성 operation을 준비한다.",
		"env_up_commit":                      "승인된 새 env 생성 operation을 실행한다.",
		"env_down_prepare":                   "env down operation을 준비한다.",
		"env_down_commit":                    "승인된 env down operation을 실행한다.",
		"env_clean_prepare":                  "env clean operation을 준비한다.",
		"env_clean_commit":                   "승인된 env clean operation을 실행한다.",
		"env_rebuild_prepare":                "env rebuild operation을 준비한다.",
		"env_rebuild_commit":                 "승인된 env rebuild operation을 실행한다.",
		"addon_uninstall_prepare":            "addon uninstall operation을 준비한다.",
		"addon_uninstall_commit":             "승인된 addon uninstall operation을 실행한다.",
		"libvirt_vm_resume_prepare":          "libvirt VM resume operation을 준비한다.",
		"libvirt_vm_resume_commit":           "승인된 libvirt VM resume operation을 실행한다.",
		"container_image_build_push_prepare": "container image build/push operation을 준비한다.",
		"container_image_build_push_commit":  "승인된 container image build/push operation을 실행한다.",
		"operation_approve":                  "준비된 operation을 승인한다.",
		"operation_cancel":                   "실행 전 operation을 취소한다.",
		"operation_status":                   "operation 상태를 조회한다.",
		"operation_logs":                     "operation stdout/stderr 로그를 조회한다.",
		"operation_locks":                    "현재 env lock 목록을 조회한다.",
		"operation_unlock_stale":             "만료된 stale lock만 해제한다.",
	}
	if purpose, ok := purposes[name]; ok {
		return purpose
	}
	return "infra-lab MCP tool"
}
