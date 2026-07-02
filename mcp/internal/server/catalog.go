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
	handlers["infra_lab.what_can_i_do"] = toolHandler{
		tool: tool{
			Name:        "infra_lab.what_can_i_do",
			Description: "Explain the current infra-lab MCP capabilities, categorized tools, safe execution flows, and recommended prompts.",
			InputSchema: emptySchema(),
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

func handlersForCatalog(info bootstrapInfo, handlers map[string]toolHandler) map[string]toolHandler {
	if handlers != nil {
		return handlers
	}
	generated := readOnlyTools(info)
	addWhatCanIDoTool(generated)
	return generated
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
	for name := range handlers {
		item := classifyTool(name)
		if item.Name == "" {
			continue
		}
		cat, ok := index[toolCategoryName(name)]
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
				Steps:       []string{"infra_lab.addon_install_plan", "infra_lab.addon_install_prepare", "infra_lab.operation_approve", "infra_lab.addon_install_commit", "infra_lab.operation_status"},
			},
			{
				Name:        "envUp",
				Title:       "New env up",
				Description: "존재하지 않는 env를 승인 기반으로 생성한다.",
				Steps:       []string{"infra_lab.up_plan", "infra_lab.env_up_prepare", "infra_lab.operation_approve", "infra_lab.env_up_commit", "infra_lab.operation_status"},
			},
			{
				Name:        "envDownClean",
				Title:       "Env down and clean",
				Description: "기존 env를 승인 기반으로 내리고 state를 정리한다.",
				Steps:       []string{"infra_lab.down_plan", "infra_lab.env_down_prepare", "infra_lab.operation_approve", "infra_lab.env_down_commit", "infra_lab.env_clean_prepare", "infra_lab.operation_approve", "infra_lab.env_clean_commit"},
			},
			{
				Name:        "envRebuild",
				Title:       "Env rebuild",
				Description: "기존 env를 승인 기반으로 재빌드한다.",
				Steps:       []string{"infra_lab.rebuild_plan", "infra_lab.env_rebuild_prepare", "infra_lab.operation_approve", "infra_lab.env_rebuild_commit", "infra_lab.operation_status"},
			},
		},
	}
}

func toolCategoryName(name string) string {
	switch name {
	case "infra_lab.version", "infra_lab.capabilities", "infra_lab.doctor", "infra_lab.env_list", "infra_lab.env_status", "infra_lab.k8s_status", "infra_lab.vm_list", "infra_lab.vm_list_all", "infra_lab.vm_version", "infra_lab.profile_list", "infra_lab.profile_show", "infra_lab.profile_validate", "infra_lab.setup_check", "infra_lab.what_can_i_do":
		return "readOnly"
	case "infra_lab.collect_snapshot", "infra_lab.summarize_health":
		return "evidence"
	case "infra_lab.up_plan", "infra_lab.down_plan", "infra_lab.rebuild_plan", "infra_lab.addon_install_plan", "infra_lab.addon_uninstall_plan":
		return "planOnly"
	case "infra_lab.profile_clone", "infra_lab.profile_save_as", "infra_lab.profile_validate_and_save":
		return "profileWrite"
	case "infra_lab.addon_install_prepare", "infra_lab.addon_install_commit", "infra_lab.env_up_prepare", "infra_lab.env_up_commit":
		return "approvedMutation"
	case "infra_lab.env_down_prepare", "infra_lab.env_down_commit", "infra_lab.env_clean_prepare", "infra_lab.env_clean_commit", "infra_lab.env_rebuild_prepare", "infra_lab.env_rebuild_commit", "infra_lab.addon_uninstall_prepare", "infra_lab.addon_uninstall_commit":
		return "destructive"
	case "infra_lab.operation_approve", "infra_lab.operation_cancel", "infra_lab.operation_status", "infra_lab.operation_logs", "infra_lab.operation_locks", "infra_lab.operation_unlock_stale":
		return "operation"
	default:
		return ""
	}
}

func classifyTool(name string) toolCatalogItem {
	category := toolCategoryName(name)
	if category == "" {
		return toolCatalogItem{}
	}
	item := toolCatalogItem{
		Name:             name,
		Purpose:          toolPurpose(name),
		Mutates:          category == "profileWrite" || category == "approvedMutation" || category == "destructive" || name == "infra_lab.operation_approve" || name == "infra_lab.operation_cancel" || name == "infra_lab.operation_unlock_stale",
		Destructive:      category == "destructive",
		RequiresApproval: category == "approvedMutation" || category == "destructive",
	}
	return item
}

func toolPurpose(name string) string {
	purposes := map[string]string{
		"infra_lab.setup_check":               "MCP readiness와 client 등록 가이드를 확인한다.",
		"infra_lab.what_can_i_do":             "현재 MCP로 가능한 작업을 카테고리별로 설명한다.",
		"infra_lab.version":                   "infra-lab 버전 정보를 조회한다.",
		"infra_lab.capabilities":              "ilab JSON capability 목록을 조회한다.",
		"infra_lab.doctor":                    "host prerequisite와 local state를 진단한다.",
		"infra_lab.env_list":                  "관리 중인 env 목록을 조회한다.",
		"infra_lab.env_status":                "env 상태를 조회한다.",
		"infra_lab.k8s_status":                "Kubernetes node/pod 상태를 조회한다.",
		"infra_lab.vm_list":                   "관리 VM 목록을 조회한다.",
		"infra_lab.vm_list_all":               "관리/비관리 VM 목록을 조회한다.",
		"infra_lab.vm_version":                "VM 내부 infra-lab build metadata와 guest OS(/etc/os-release) 정보를 조회한다.",
		"infra_lab.profile_list":              "사용 가능한 profile 목록을 조회한다.",
		"infra_lab.profile_show":              "profile 정규화 내용을 조회한다.",
		"infra_lab.profile_validate":          "profile 유효성을 검증한다.",
		"infra_lab.collect_snapshot":          "env/profile/VM/Kubernetes 증거를 수집한다.",
		"infra_lab.summarize_health":          "snapshot 기반 health를 요약한다.",
		"infra_lab.up_plan":                   "새 env 생성 계획만 만든다.",
		"infra_lab.down_plan":                 "env down 계획만 만든다.",
		"infra_lab.rebuild_plan":              "env rebuild 계획만 만든다.",
		"infra_lab.addon_install_plan":        "addon install 계획만 만든다.",
		"infra_lab.addon_uninstall_plan":      "addon uninstall 계획만 만든다.",
		"infra_lab.profile_clone":             "profile을 user profile dir로 복제한다.",
		"infra_lab.profile_save_as":           "새 profile을 user profile dir에 저장한다.",
		"infra_lab.profile_validate_and_save": "profile을 검증하고 저장한다.",
		"infra_lab.addon_install_prepare":     "addon install operation을 준비한다.",
		"infra_lab.addon_install_commit":      "승인된 addon install operation을 실행한다.",
		"infra_lab.env_up_prepare":            "새 env 생성 operation을 준비한다.",
		"infra_lab.env_up_commit":             "승인된 새 env 생성 operation을 실행한다.",
		"infra_lab.env_down_prepare":          "env down operation을 준비한다.",
		"infra_lab.env_down_commit":           "승인된 env down operation을 실행한다.",
		"infra_lab.env_clean_prepare":         "env clean operation을 준비한다.",
		"infra_lab.env_clean_commit":          "승인된 env clean operation을 실행한다.",
		"infra_lab.env_rebuild_prepare":       "env rebuild operation을 준비한다.",
		"infra_lab.env_rebuild_commit":        "승인된 env rebuild operation을 실행한다.",
		"infra_lab.addon_uninstall_prepare":   "addon uninstall operation을 준비한다.",
		"infra_lab.addon_uninstall_commit":    "승인된 addon uninstall operation을 실행한다.",
		"infra_lab.operation_approve":         "준비된 operation을 승인한다.",
		"infra_lab.operation_cancel":          "실행 전 operation을 취소한다.",
		"infra_lab.operation_status":          "operation 상태를 조회한다.",
		"infra_lab.operation_logs":            "operation stdout/stderr 로그를 조회한다.",
		"infra_lab.operation_locks":           "현재 env lock 목록을 조회한다.",
		"infra_lab.operation_unlock_stale":    "만료된 stale lock만 해제한다.",
	}
	if purpose, ok := purposes[name]; ok {
		return purpose
	}
	return "infra-lab MCP tool"
}
