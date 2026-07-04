package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type toolHandler struct {
	tool     tool
	metadata toolMetadata
	call     func(json.RawMessage) (toolResult, error)
}

type toolMetadata struct {
	RequiredCapabilities []string `json:"requiredCapabilities"`
	Category             string   `json:"category"`
	Risk                 string   `json:"risk"`
	Destructive          bool     `json:"destructive"`
	RequiresApproval     bool     `json:"requiresApproval"`
	Source               string   `json:"source"`
	Stage                string   `json:"stage"`
}

type envArg struct {
	Env string `json:"env,omitempty"`
}

type profileArg struct {
	Name string `json:"name"`
}

type vmVersionArg struct {
	VM string `json:"vm"`
}

type planArg struct {
	Env     string `json:"env,omitempty"`
	Profile string `json:"profile,omitempty"`
	Addon   string `json:"addon,omitempty"`
}

type profileWriteArg struct {
	Name     string `json:"name"`
	Source   string `json:"source,omitempty"`
	Backend  string `json:"backend,omitempty"`
	CNI      string `json:"cni,omitempty"`
	Masters  int    `json:"masters,omitempty"`
	Workers  int    `json:"workers,omitempty"`
	OSImage  string `json:"osImage,omitempty"`
	StateDir string `json:"stateDir,omitempty"`
}

type addonPrepareArg struct {
	Env   string `json:"env"`
	Addon string `json:"addon"`
}

type envUpPrepareArg struct {
	Profile string `json:"profile"`
	Env     string `json:"env,omitempty"`
}

type destructivePrepareArg struct {
	Env     string `json:"env,omitempty"`
	Profile string `json:"profile,omitempty"`
	Addon   string `json:"addon,omitempty"`
}

type addonCommitArg struct {
	OperationID   string `json:"operationId"`
	ApprovalToken string `json:"approvalToken"`
}

type operationArg struct {
	OperationID string `json:"operationId"`
}

func readOnlyTools(capabilities map[string]bool) map[string]toolHandler {
	handlers := map[string]toolHandler{}
	add := func(capability, name, description string, schema map[string]any, ilabArgs func(json.RawMessage) ([]string, error), timeout time.Duration) {
		if !capabilities[capability] {
			return
		}
		addTool(handlers, name, description, schema, directToolMeta(capability, "read_only", "LOW", "Stage 1"), ilabArgs, timeout)
	}
	addSynthetic := func(required []string, name, description string, schema map[string]any, meta toolMetadata, ilabArgs func(json.RawMessage) ([]string, error), timeout time.Duration) {
		for _, capability := range required {
			if !capabilities[capability] {
				return
			}
		}
		meta.RequiredCapabilities = append([]string(nil), required...)
		meta.Source = "mcp-synthetic"
		addTool(handlers, name, description, schema, meta, ilabArgs, timeout)
	}

	add("version.v1", "infra_lab.version", "Show infra-lab version metadata.", emptySchema(), noArgs("version"), 30*time.Second)
	add("capabilities.v1", "infra_lab.capabilities", "Show ilab JSON contract capabilities.", emptySchema(), noArgs("capabilities"), 30*time.Second)
	add("doctor.v1", "infra_lab.doctor", "Diagnose infra-lab prerequisites and local state.", emptySchema(), noArgs("doctor"), 30*time.Second)
	add("env.list.v1", "infra_lab.env_list", "List managed infra-lab environments.", emptySchema(), noArgs("env", "list"), 30*time.Second)
	add("env.status.v1", "infra_lab.env_status", "Show status for one or all environments.", envSchema(), envArgs("env", "status"), 30*time.Second)
	add("k8s.status.v1", "infra_lab.k8s_status", "Show Kubernetes node and pod status.", envSchema(), envArgs("k8s", "status"), 60*time.Second)
	add("vm.list.v1", "infra_lab.vm_list", "List managed VMs.", emptySchema(), noArgs("vm", "list"), 30*time.Second)
	add("vm.list.v1", "infra_lab.vm_list_all", "List managed and unmanaged VMs.", emptySchema(), noArgs("vm", "list", "--all"), 30*time.Second)
	add("vm.version.v1", "infra_lab.vm_version", "Read infra-lab build metadata from a VM.", vmVersionSchema(), vmVersionArgs(), 30*time.Second)
	add("profile.list.v1", "infra_lab.profile_list", "List available profiles.", emptySchema(), noArgs("profile", "list"), 30*time.Second)
	add("profile.show.v1", "infra_lab.profile_show", "Show normalized profile data.", profileSchema(), profileArgs("profile", "show"), 30*time.Second)
	add("profile.validate.v1", "infra_lab.profile_validate", "Validate a profile.", profileSchema(), profileArgs("profile", "validate"), 30*time.Second)
	addSynthetic([]string{"env.status.v1", "vm.list.v1", "k8s.status.v1"}, "infra_lab.collect_snapshot", "Collect a read-only infra-lab health snapshot.", envSchema(), syntheticToolMeta("evidence", "LOW", false, false, "Stage 2"), snapshotArgs(), 90*time.Second)
	addSynthetic([]string{"env.status.v1", "vm.list.v1", "k8s.status.v1"}, "infra_lab.summarize_health", "Summarize read-only infra-lab snapshot health.", envSchema(), syntheticToolMeta("evidence", "LOW", false, false, "Stage 2"), healthSummaryArgs(), 90*time.Second)
	addSynthetic([]string{"profile.validate.v1"}, "infra_lab.up_plan", "Create a plan-only env up proposal.", planSchema(false), syntheticToolMeta("plan", "MEDIUM", false, true, "Stage 3"), planArgs("env_up"), 30*time.Second)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.down_plan", "Create a plan-only env down proposal.", planSchema(false), syntheticToolMeta("plan", "HIGH", true, true, "Stage 3"), planArgs("env_down"), 30*time.Second)
	addSynthetic([]string{"profile.validate.v1", "env.status.v1"}, "infra_lab.rebuild_plan", "Create a plan-only env rebuild proposal.", planSchema(false), syntheticToolMeta("plan", "HIGH", true, true, "Stage 3"), planArgs("rebuild"), 30*time.Second)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.addon_install_plan", "Create a plan-only addon install proposal.", planSchema(true), syntheticToolMeta("plan", "MEDIUM", false, true, "Stage 3"), planArgs("addon_install"), 30*time.Second)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.addon_uninstall_plan", "Create a plan-only addon uninstall proposal.", planSchema(true), syntheticToolMeta("plan", "HIGH", true, true, "Stage 3"), planArgs("addon_uninstall"), 30*time.Second)
	addSynthetic([]string{"profile.validate.v1"}, "infra_lab.profile_clone", "Clone a profile into the user profile directory without overwriting existing files.", profileWriteSchema(true), syntheticToolMeta("profile_write", "MEDIUM", false, false, "Stage 4"), profileWriteArgs("clone"), 30*time.Second)
	addSynthetic([]string{"profile.validate.v1"}, "infra_lab.profile_save_as", "Create a new profile in the user profile directory without touching infrastructure.", profileWriteSchema(false), syntheticToolMeta("profile_write", "MEDIUM", false, false, "Stage 4"), profileWriteArgs("save_as"), 30*time.Second)
	addSynthetic([]string{"profile.validate.v1"}, "infra_lab.profile_validate_and_save", "Validate and save a new profile in the user profile directory.", profileWriteSchema(false), syntheticToolMeta("profile_write", "MEDIUM", false, false, "Stage 4"), profileWriteArgs("validate_and_save"), 30*time.Second)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.addon_install_prepare", "Prepare an approved addon install operation.", addonPrepareSchema(), syntheticToolMeta("approved_mutation", "MEDIUM", false, true, "Stage 5"), addonPrepareArgs(), 30*time.Second)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.addon_install_commit", "Commit a prepared addon install operation after approval.", addonCommitSchema(), syntheticToolMeta("approved_mutation", "MEDIUM", false, true, "Stage 5"), addonCommitArgs(), 15*time.Minute)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.operation_status", "Read an infra-lab operation status record.", operationSchema(), syntheticToolMeta("operation", "LOW", false, false, "Stage 5"), operationArgs("status"), 30*time.Second)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.operation_logs", "Read stdout/stderr logs for an infra-lab operation.", operationSchema(), syntheticToolMeta("operation", "LOW", false, false, "Stage 5"), operationArgs("logs"), 30*time.Second)
	addSynthetic([]string{"profile.validate.v1", "env.status.v1"}, "infra_lab.env_up_prepare", "Prepare an approved new environment creation operation.", envUpPrepareSchema(), syntheticToolMeta("approved_env_up", "HIGH", false, true, "Stage 6"), envUpPrepareArgs(), 30*time.Second)
	addSynthetic([]string{"profile.validate.v1", "env.status.v1"}, "infra_lab.env_up_commit", "Commit a prepared new environment creation operation after approval.", addonCommitSchema(), syntheticToolMeta("approved_env_up", "HIGH", false, true, "Stage 6"), envUpCommitArgs(), 30*time.Minute)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.env_down_prepare", "Prepare an approved destructive env down operation.", destructivePrepareSchema(false, false), syntheticToolMeta("destructive_execution", "HIGH", true, true, "Stage 7"), destructivePrepareArgs("env_down_prepare"), 30*time.Second)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.env_down_commit", "Commit a prepared env down operation after approval.", addonCommitSchema(), syntheticToolMeta("destructive_execution", "HIGH", true, true, "Stage 7"), destructiveCommitArgs("env_down_commit"), 30*time.Minute)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.env_clean_prepare", "Prepare an approved destructive env clean operation.", destructivePrepareSchema(false, false), syntheticToolMeta("destructive_execution", "HIGH", true, true, "Stage 7"), destructivePrepareArgs("env_clean_prepare"), 30*time.Second)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.env_clean_commit", "Commit a prepared env clean operation after approval.", addonCommitSchema(), syntheticToolMeta("destructive_execution", "HIGH", true, true, "Stage 7"), destructiveCommitArgs("env_clean_commit"), 10*time.Minute)
	addSynthetic([]string{"profile.validate.v1", "env.status.v1"}, "infra_lab.env_rebuild_prepare", "Prepare an approved destructive env rebuild operation.", destructivePrepareSchema(true, false), syntheticToolMeta("destructive_execution", "HIGH", true, true, "Stage 7"), destructivePrepareArgs("env_rebuild_prepare"), 30*time.Second)
	addSynthetic([]string{"profile.validate.v1", "env.status.v1"}, "infra_lab.env_rebuild_commit", "Commit a prepared env rebuild operation after approval.", addonCommitSchema(), syntheticToolMeta("destructive_execution", "HIGH", true, true, "Stage 7"), destructiveCommitArgs("env_rebuild_commit"), 45*time.Minute)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.addon_uninstall_prepare", "Prepare an approved destructive addon uninstall operation.", destructivePrepareSchema(false, true), syntheticToolMeta("destructive_execution", "HIGH", true, true, "Stage 7"), destructivePrepareArgs("addon_uninstall_prepare"), 30*time.Second)
	addSynthetic([]string{"env.status.v1"}, "infra_lab.addon_uninstall_commit", "Commit a prepared addon uninstall operation after approval.", addonCommitSchema(), syntheticToolMeta("destructive_execution", "HIGH", true, true, "Stage 7"), destructiveCommitArgs("addon_uninstall_commit"), 15*time.Minute)
	if capabilities["version.v1"] && capabilities["capabilities.v1"] {
		addToolCatalog(handlers)
	}

	return handlers
}

func directToolMeta(capability, category, risk, stage string) toolMetadata {
	return toolMetadata{
		RequiredCapabilities: []string{capability},
		Category:             category,
		Risk:                 risk,
		Destructive:          false,
		RequiresApproval:     false,
		Source:               "ilab-capability",
		Stage:                stage,
	}
}

func syntheticToolMeta(category, risk string, destructive, requiresApproval bool, stage string) toolMetadata {
	return toolMetadata{
		Category:         category,
		Risk:             risk,
		Destructive:      destructive,
		RequiresApproval: requiresApproval,
		Stage:            stage,
	}
}

func addTool(handlers map[string]toolHandler, name, description string, schema map[string]any, metadata toolMetadata, ilabArgs func(json.RawMessage) ([]string, error), timeout time.Duration) {
	handlers[name] = toolHandler{
		tool: tool{
			Name:        name,
			Description: description,
			InputSchema: schema,
		},
		metadata: metadata,
		call: func(raw json.RawMessage) (toolResult, error) {
			args, err := ilabArgs(raw)
			if err != nil {
				return toolResult{}, err
			}
			if len(args) > 0 && args[0] == "__snapshot__" {
				command := "snapshot.collect"
				if len(args) > 1 && args[1] == "__health__" {
					command = "health.summarize"
					args = append(args[:1], args[2:]...)
				}
				env := ""
				if len(args) > 1 {
					env = args[1]
				}
				out, err := collectSnapshot(command, env, timeout)
				if err != nil {
					return toolResult{}, err
				}
				return toolResult{Content: []toolContent{{Type: "text", Text: out}}}, nil
			}
			if len(args) > 0 && args[0] == "__plan__" {
				out, err := createPlan(args[1:])
				if err != nil {
					return toolResult{}, err
				}
				return toolResult{Content: []toolContent{{Type: "text", Text: out}}}, nil
			}
			if len(args) > 0 && args[0] == "__profile_write__" {
				out, err := writeProfile(args[1:], timeout)
				if err != nil {
					return toolResult{}, err
				}
				return toolResult{Content: []toolContent{{Type: "text", Text: out}}}, nil
			}
			if len(args) > 0 && args[0] == "__operation__" {
				out, err := handleOperation(args[1:], timeout)
				if err != nil {
					return toolResult{}, err
				}
				return toolResult{Content: []toolContent{{Type: "text", Text: out}}}, nil
			}
			out, isErr, err := runILab(args, timeout)
			if err != nil {
				return toolResult{}, err
			}
			return toolResult{
				Content: []toolContent{{Type: "text", Text: out}},
				IsError: isErr,
			}, nil
		},
	}
}

func addonPrepareArgs() func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed addonPrepareArg
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		if parsed.Env == "" {
			return nil, fmt.Errorf("env is required")
		}
		if parsed.Addon == "" {
			return nil, fmt.Errorf("addon is required")
		}
		return []string{"__operation__", "addon_install_prepare", "env=" + parsed.Env, "addon=" + parsed.Addon}, nil
	}
}

func addonCommitArgs() func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed addonCommitArg
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		if parsed.OperationID == "" {
			return nil, fmt.Errorf("operationId is required")
		}
		if parsed.ApprovalToken == "" {
			return nil, fmt.Errorf("approvalToken is required")
		}
		return []string{"__operation__", "addon_install_commit", "operationId=" + parsed.OperationID, "approvalToken=" + parsed.ApprovalToken}, nil
	}
}

func envUpPrepareArgs() func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed envUpPrepareArg
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		if parsed.Profile == "" {
			return nil, fmt.Errorf("profile is required")
		}
		out := []string{"__operation__", "env_up_prepare", "profile=" + parsed.Profile}
		if parsed.Env != "" {
			out = append(out, "env="+parsed.Env)
		}
		return out, nil
	}
}

func envUpCommitArgs() func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed addonCommitArg
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		if parsed.OperationID == "" {
			return nil, fmt.Errorf("operationId is required")
		}
		if parsed.ApprovalToken == "" {
			return nil, fmt.Errorf("approvalToken is required")
		}
		return []string{"__operation__", "env_up_commit", "operationId=" + parsed.OperationID, "approvalToken=" + parsed.ApprovalToken}, nil
	}
}

func destructivePrepareArgs(action string) func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed destructivePrepareArg
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		if parsed.Env == "" {
			return nil, fmt.Errorf("env is required")
		}
		out := []string{"__operation__", action, "env=" + parsed.Env}
		if parsed.Profile != "" {
			out = append(out, "profile="+parsed.Profile)
		}
		if parsed.Addon != "" {
			out = append(out, "addon="+parsed.Addon)
		}
		return out, nil
	}
}

func destructiveCommitArgs(action string) func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed addonCommitArg
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		if parsed.OperationID == "" {
			return nil, fmt.Errorf("operationId is required")
		}
		if parsed.ApprovalToken == "" {
			return nil, fmt.Errorf("approvalToken is required")
		}
		return []string{"__operation__", action, "operationId=" + parsed.OperationID, "approvalToken=" + parsed.ApprovalToken}, nil
	}
}

func operationArgs(action string) func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed operationArg
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		if parsed.OperationID == "" {
			return nil, fmt.Errorf("operationId is required")
		}
		return []string{"__operation__", action, "operationId=" + parsed.OperationID}, nil
	}
}

func profileWriteArgs(action string) func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed profileWriteArg
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		if parsed.Name == "" {
			return nil, fmt.Errorf("name is required")
		}
		if action == "clone" && parsed.Source == "" {
			return nil, fmt.Errorf("source is required")
		}
		out := []string{"__profile_write__", action, "name=" + parsed.Name}
		if parsed.Source != "" {
			out = append(out, "source="+parsed.Source)
		}
		if parsed.Backend != "" {
			out = append(out, "backend="+parsed.Backend)
		}
		if parsed.CNI != "" {
			out = append(out, "cni="+parsed.CNI)
		}
		if parsed.Masters > 0 {
			out = append(out, fmt.Sprintf("masters=%d", parsed.Masters))
		}
		if parsed.Workers > 0 {
			out = append(out, fmt.Sprintf("workers=%d", parsed.Workers))
		}
		if parsed.OSImage != "" {
			out = append(out, "osImage="+parsed.OSImage)
		}
		if parsed.StateDir != "" {
			out = append(out, "stateDir="+parsed.StateDir)
		}
		return out, nil
	}
}

func planArgs(action string) func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed planArg
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &parsed); err != nil {
				return nil, err
			}
		}
		out := []string{"__plan__", action}
		if parsed.Env != "" {
			out = append(out, "env="+parsed.Env)
		}
		if parsed.Profile != "" {
			out = append(out, "profile="+parsed.Profile)
		}
		if parsed.Addon != "" {
			out = append(out, "addon="+parsed.Addon)
		}
		return out, nil
	}
}

func snapshotArgs() func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed envArg
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &parsed); err != nil {
				return nil, err
			}
		}
		// Marker consumed by toolHandler before invoking ilab directly.
		if parsed.Env != "" {
			return []string{"__snapshot__", parsed.Env}, nil
		}
		return []string{"__snapshot__"}, nil
	}
}

func healthSummaryArgs() func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		args, err := snapshotArgs()(raw)
		if err != nil {
			return nil, err
		}
		out := []string{"__snapshot__", "__health__"}
		if len(args) > 1 {
			out = append(out, args[1])
		}
		return out, nil
	}
}

func runILab(args []string, timeout time.Duration) (string, bool, error) {
	root, err := infraLabRoot()
	if err != nil {
		return "", false, err
	}
	ilab := filepath.Join(root, "bin", "ilab")
	cmdArgs := append([]string{}, args...)
	cmdArgs = append(cmdArgs, "--json")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, ilab, cmdArgs...)
	cmd.Env = append(os.Environ(), "INFRA_LAB_ROOT="+root)
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		timeoutJSON := fmt.Sprintf(`{"ok":false,"command":"%s","contractVersion":"infra-lab.contract/v1","data":null,"warnings":[],"errors":[{"code":"COMMAND_TIMEOUT","message":"MCP runner killed ilab after %s"}]}`+"\n", commandName(args), timeout)
		return timeoutJSON, true, nil
	}
	if err != nil {
		if len(out) == 0 {
			return "", false, err
		}
		return string(out), true, nil
	}
	return string(out), false, nil
}

func infraLabRoot() (string, error) {
	if root := os.Getenv("INFRA_LAB_ROOT"); root != "" {
		return root, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir, err := filepath.Abs(wd)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "k8s-tool.sh")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("not inside an infra-lab repository; set INFRA_LAB_ROOT")
}

func commandName(args []string) string {
	if len(args) == 0 {
		return "unknown"
	}
	if len(args) == 1 {
		return args[0]
	}
	return args[0] + "." + args[1]
}

func noArgs(args ...string) func(json.RawMessage) ([]string, error) {
	return func(_ json.RawMessage) ([]string, error) {
		return args, nil
	}
}

func envArgs(args ...string) func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed envArg
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &parsed); err != nil {
				return nil, err
			}
		}
		out := append([]string{}, args...)
		if parsed.Env != "" {
			out = append(out, parsed.Env)
		}
		return out, nil
	}
}

func profileArgs(args ...string) func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed profileArg
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		if parsed.Name == "" {
			return nil, fmt.Errorf("name is required")
		}
		out := append([]string{}, args...)
		out = append(out, parsed.Name)
		return out, nil
	}
}

func vmVersionArgs() func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		var parsed vmVersionArg
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		if parsed.VM == "" {
			return nil, fmt.Errorf("vm is required")
		}
		return []string{"vm", "version", parsed.VM}, nil
	}
}

func emptySchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
}

func envSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"env": map[string]any{"type": "string"},
		},
		"additionalProperties": false,
	}
}

func profileSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required":             []string{"name"},
		"additionalProperties": false,
	}
}

func vmVersionSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"vm": map[string]any{"type": "string"},
		},
		"required":             []string{"vm"},
		"additionalProperties": false,
	}
}

func planSchema(addon bool) map[string]any {
	properties := map[string]any{
		"env":     map[string]any{"type": "string"},
		"profile": map[string]any{"type": "string"},
	}
	if addon {
		properties["addon"] = map[string]any{"type": "string"}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
}

func profileWriteSchema(clone bool) map[string]any {
	properties := map[string]any{
		"name":     map[string]any{"type": "string"},
		"backend":  map[string]any{"type": "string"},
		"cni":      map[string]any{"type": "string"},
		"masters":  map[string]any{"type": "integer", "minimum": 1},
		"workers":  map[string]any{"type": "integer", "minimum": 1},
		"osImage":  map[string]any{"type": "string"},
		"stateDir": map[string]any{"type": "string"},
	}
	required := []string{"name"}
	if clone {
		properties["source"] = map[string]any{"type": "string"}
		required = append(required, "source")
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func addonPrepareSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"env":   map[string]any{"type": "string"},
			"addon": map[string]any{"type": "string"},
		},
		"required":             []string{"env", "addon"},
		"additionalProperties": false,
	}
}

func addonCommitSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operationId":   map[string]any{"type": "string"},
			"approvalToken": map[string]any{"type": "string"},
		},
		"required":             []string{"operationId", "approvalToken"},
		"additionalProperties": false,
	}
}

func operationSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operationId": map[string]any{"type": "string"},
		},
		"required":             []string{"operationId"},
		"additionalProperties": false,
	}
}

func envUpPrepareSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"profile": map[string]any{"type": "string"},
			"env":     map[string]any{"type": "string"},
		},
		"required":             []string{"profile"},
		"additionalProperties": false,
	}
}

func destructivePrepareSchema(profile, addon bool) map[string]any {
	properties := map[string]any{
		"env": map[string]any{"type": "string"},
	}
	required := []string{"env"}
	if profile {
		properties["profile"] = map[string]any{"type": "string"}
		required = append(required, "profile")
	}
	if addon {
		properties["addon"] = map[string]any{"type": "string"}
		required = append(required, "addon")
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}
