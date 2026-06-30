package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type planEnvelope struct {
	OK              bool     `json:"ok"`
	Command         string   `json:"command"`
	ContractVersion string   `json:"contractVersion"`
	Data            planData `json:"data"`
	Warnings        []any    `json:"warnings"`
	Errors          []any    `json:"errors"`
}

type planData struct {
	PlanID            string       `json:"planId"`
	PlanFingerprint   string       `json:"planFingerprint"`
	TargetFingerprint string       `json:"targetFingerprint"`
	Action            string       `json:"action"`
	Env               string       `json:"env,omitempty"`
	Profile           string       `json:"profile,omitempty"`
	Addon             string       `json:"addon,omitempty"`
	Destructive       bool         `json:"destructive"`
	RequiresApproval  bool         `json:"requiresApproval"`
	Risk              string       `json:"risk"`
	CreatedAt         string       `json:"createdAt"`
	ExpiresAt         string       `json:"expiresAt"`
	Reasons           []planReason `json:"reasons"`
	Steps             []string     `json:"steps"`
	Blocked           bool         `json:"blocked"`
}

type planReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func createPlan(args []string) (string, error) {
	action := "unknown"
	if len(args) > 0 {
		action = args[0]
	}
	fields := map[string]string{}
	for _, arg := range args[1:] {
		key, value, ok := strings.Cut(arg, "=")
		if ok {
			fields[key] = value
		}
	}

	now := time.Now().UTC()
	data := planData{
		PlanID:           "plan_" + now.Format("20060102_150405") + "_" + action,
		Action:           action,
		Env:              fields["env"],
		Profile:          fields["profile"],
		Addon:            fields["addon"],
		CreatedAt:        now.Format(time.RFC3339),
		ExpiresAt:        now.Add(2 * time.Hour).Format(time.RFC3339),
		Reasons:          []planReason{},
		Steps:            planSteps(action),
		Blocked:          false,
		Destructive:      planDestructive(action),
		RequiresApproval: true,
		Risk:             planRisk(action),
	}
	if !data.Destructive && action == "env_up" {
		data.Risk = "MEDIUM"
	}
	data.TargetFingerprint = digest("target", action, data.Env, data.Profile, data.Addon)
	data.PlanFingerprint = digest("plan", action, data.Env, data.Profile, data.Addon, strings.Join(data.Steps, "|"))
	data.Reasons = planReasons(data)

	env := planEnvelope{
		OK:              true,
		Command:         "plan." + strings.TrimPrefix(action, "env_"),
		ContractVersion: supportedContractVersion,
		Data:            data,
		Warnings:        []any{},
		Errors:          []any{},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		return "", err
	}
	if err := writePlan(data.PlanID, buf.Bytes()); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func writePlan(planID string, data []byte) error {
	dir, err := planStoreDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create plan store: %w", err)
	}
	path := filepath.Join(dir, planID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write plan: %w", err)
	}
	return nil
}

func planStoreDir() (string, error) {
	if dir := os.Getenv("INFRA_LAB_PLAN_STORE"); dir != "" {
		return dir, nil
	}
	root, err := infraLabRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state", ".plans"), nil
}

func planDestructive(action string) bool {
	switch action {
	case "env_down", "rebuild", "addon_uninstall":
		return true
	default:
		return false
	}
}

func planRisk(action string) string {
	switch action {
	case "env_down", "rebuild", "addon_uninstall":
		return "HIGH"
	case "addon_install":
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func planSteps(action string) []string {
	switch action {
	case "env_up":
		return []string{"validate profile", "collect pre-snapshot", "run env up", "collect post-snapshot"}
	case "env_down":
		return []string{"collect pre-snapshot", "run env down", "collect post-snapshot"}
	case "rebuild":
		return []string{"validate profile", "collect pre-snapshot", "run env down", "clean state", "run env up", "collect post-snapshot"}
	case "addon_install":
		return []string{"collect pre-snapshot", "run addon install", "run addon verify", "collect post-snapshot"}
	case "addon_uninstall":
		return []string{"collect pre-snapshot", "run addon uninstall", "collect post-snapshot"}
	default:
		return []string{"collect pre-snapshot", "collect post-snapshot"}
	}
}

func planReasons(data planData) []planReason {
	reasons := []planReason{}
	if data.Destructive {
		reasons = append(reasons, planReason{Code: "DESTRUCTIVE_ACTION", Message: data.Action + " is destructive and requires approval"})
	}
	if data.RequiresApproval {
		reasons = append(reasons, planReason{Code: "APPROVAL_REQUIRED", Message: "prepare/commit approval flow is required before execution"})
	}
	if data.Profile == "" && (data.Action == "env_up" || data.Action == "rebuild") {
		reasons = append(reasons, planReason{Code: "PROFILE_NOT_SPECIFIED", Message: "profile was not specified in plan input"})
	}
	if data.Env == "" && (data.Action == "env_down" || data.Action == "addon_install" || data.Action == "addon_uninstall") {
		reasons = append(reasons, planReason{Code: "ENV_NOT_SPECIFIED", Message: "env was not specified in plan input"})
	}
	return reasons
}

func digest(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
