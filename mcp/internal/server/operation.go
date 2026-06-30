package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type operationEnvelope struct {
	OK              bool          `json:"ok"`
	Command         string        `json:"command"`
	ContractVersion string        `json:"contractVersion"`
	Data            any           `json:"data"`
	Warnings        []any         `json:"warnings"`
	Errors          []interface{} `json:"errors"`
}

type operationRecord struct {
	OperationID string            `json:"operationId"`
	Tool        string            `json:"tool"`
	Status      string            `json:"status"`
	Risk        string            `json:"risk"`
	Destructive bool              `json:"destructive"`
	Target      operationTarget   `json:"target"`
	CreatedAt   string            `json:"createdAt"`
	ExpiresAt   string            `json:"expiresAt"`
	StartedAt   string            `json:"startedAt,omitempty"`
	FinishedAt  string            `json:"finishedAt,omitempty"`
	Approval    operationApproval `json:"approval"`
	Steps       []operationStep   `json:"steps"`
	ErrorCode   string            `json:"errorCode,omitempty"`
	Error       string            `json:"error,omitempty"`
}

type operationTarget struct {
	Env               string `json:"env"`
	Addon             string `json:"addon,omitempty"`
	Profile           string `json:"profile,omitempty"`
	ProfileDigest     string `json:"profileDigest,omitempty"`
	TargetFingerprint string `json:"targetFingerprint"`
}

type operationApproval struct {
	Required  bool   `json:"required"`
	Status    string `json:"status"`
	Mode      string `json:"mode"`
	TokenHint string `json:"tokenHint,omitempty"`
}

type operationStep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type operationPrepareData struct {
	OperationID       string              `json:"operationId"`
	ApprovalToken     string              `json:"approvalToken"`
	ExpiresAt         string              `json:"expiresAt"`
	PlanFingerprint   string              `json:"planFingerprint"`
	TargetFingerprint string              `json:"targetFingerprint"`
	Approval          operationApproval   `json:"approval"`
	Risk              string              `json:"risk"`
	Target            operationTarget     `json:"target"`
	Steps             []string            `json:"steps"`
	Operation         operationRecord     `json:"operation"`
	OperationPath     string              `json:"operationPath"`
	Warnings          []map[string]string `json:"findings,omitempty"`
}

type operationLogsData struct {
	OperationID string `json:"operationId"`
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
}

type profileValidationEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		Profile struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"profile"`
		Normalized struct {
			StateDir string `json:"stateDir"`
		} `json:"normalized"`
	} `json:"data"`
}

func handleOperation(args []string, timeout time.Duration) (string, error) {
	action := ""
	if len(args) > 0 {
		action = args[0]
	}
	fields := mapFields(args[1:])
	switch action {
	case "addon_install_prepare":
		return prepareAddonInstall(fields)
	case "addon_install_commit":
		return commitAddonInstall(fields, timeout)
	case "env_up_prepare":
		return prepareEnvUp(fields)
	case "env_up_commit":
		return commitEnvUp(fields, timeout)
	case "env_down_prepare", "env_clean_prepare", "env_rebuild_prepare", "addon_uninstall_prepare":
		return prepareDestructive(action, fields)
	case "env_down_commit", "env_clean_commit", "env_rebuild_commit", "addon_uninstall_commit":
		return commitDestructive(action, fields, timeout)
	case "status":
		return operationStatus(fields["operationId"])
	case "logs":
		return operationLogs(fields["operationId"])
	default:
		return "", fmt.Errorf("unsupported operation action: %s", action)
	}
}

func prepareDestructive(action string, fields map[string]string) (string, error) {
	env := fields["env"]
	if env == "" {
		return "", fmt.Errorf("env is required")
	}
	if err := validateTokenPart(env); err != nil {
		return "", err
	}
	tool := strings.TrimSuffix(action, "_prepare")
	if tool == "env_rebuild" && fields["profile"] == "" {
		return "", fmt.Errorf("profile is required")
	}
	if tool == "addon_uninstall" && fields["addon"] == "" {
		return "", fmt.Errorf("addon is required")
	}
	if fields["profile"] != "" {
		validation, isErr, err := runILab([]string{"profile", "validate", fields["profile"]}, 30*time.Second)
		if err != nil {
			return "", err
		}
		if isErr {
			return "", fmt.Errorf("PROFILE_INVALID: %s", strings.TrimSpace(validation))
		}
	}

	profileDigest := ""
	if fields["profile"] != "" {
		profileDigest = digest("profile", fields["profile"])
	}
	now := time.Now().UTC()
	op := operationRecord{
		OperationID: operationID(tool),
		Tool:        tool,
		Status:      "PREPARED",
		Risk:        "HIGH",
		Destructive: true,
		Target: operationTarget{
			Env:               env,
			Addon:             fields["addon"],
			Profile:           fields["profile"],
			ProfileDigest:     profileDigest,
			TargetFingerprint: digest(tool, env, fields["addon"], fields["profile"]),
		},
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(1 * time.Hour).Format(time.RFC3339),
		Approval: operationApproval{
			Required: true,
			Status:   "required",
			Mode:     "token-v1",
		},
		Steps: destructiveSteps(tool),
	}
	token, err := approvalToken(op)
	if err != nil {
		return "", err
	}
	op.Approval.TokenHint = token[:min(18, len(token))]
	path, err := writeOperation(op)
	if err != nil {
		return "", err
	}
	stepNames := make([]string, 0, len(op.Steps))
	for _, step := range op.Steps {
		stepNames = append(stepNames, step.Name)
	}
	data := operationPrepareData{
		OperationID:       op.OperationID,
		ApprovalToken:     token,
		ExpiresAt:         op.ExpiresAt,
		PlanFingerprint:   digest("plan", tool, env, fields["addon"], fields["profile"]),
		TargetFingerprint: op.Target.TargetFingerprint,
		Approval:          op.Approval,
		Risk:              op.Risk,
		Target:            op.Target,
		Steps:             stepNames,
		Operation:         op,
		OperationPath:     path,
	}
	return encodeOperationEnvelope(commandForTool(tool)+".prepare", data)
}

func commitDestructive(action string, fields map[string]string, timeout time.Duration) (string, error) {
	tool := strings.TrimSuffix(action, "_commit")
	op, err := verifyPreparedOperation(fields, tool)
	if err != nil {
		return "", err
	}
	release, err := acquireEnvLock(op)
	if err != nil {
		return "", err
	}
	defer release()
	if _, err := appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        tool + "_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      auditTarget(op),
		Result:      "started",
	}); err != nil {
		return "", fmt.Errorf("AUDIT_WRITE_FAILED: %w", err)
	}

	op.Status = "RUNNING"
	op.StartedAt = time.Now().UTC().Format(time.RFC3339)
	op.Approval.Status = "approved"
	markStep(&op, "collect pre-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "pre-snapshot.json", op.Target.Env); err != nil {
		return failOperation(op, "SNAPSHOT_FAILED", err, "collect pre-snapshot")
	}
	markStep(&op, "collect pre-snapshot", "succeeded")
	runStep := destructiveRunStep(tool)
	markStep(&op, runStep, "running")
	_, _ = writeOperation(op)
	if err := runDestructiveCommand(op, timeout); err != nil {
		return failOperation(op, "COMMAND_FAILED", err, runStep)
	}
	markStep(&op, runStep, "succeeded")
	markStep(&op, "collect post-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "post-snapshot.json", op.Target.Env); err != nil {
		return failOperation(op, "SNAPSHOT_FAILED", err, "collect post-snapshot")
	}
	op.Status = "SUCCEEDED"
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	markStep(&op, "collect post-snapshot", "succeeded")
	_, _ = writeOperation(op)
	_, _ = appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        tool + "_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      auditTarget(op),
		Result:      "ok",
	})
	return encodeOperationEnvelope(commandForTool(tool)+".commit", op)
}

func prepareEnvUp(fields map[string]string) (string, error) {
	profile := fields["profile"]
	if profile == "" {
		return "", fmt.Errorf("profile is required")
	}
	validation, isErr, err := runILab([]string{"profile", "validate", profile}, 30*time.Second)
	if err != nil {
		return "", err
	}
	if isErr {
		return "", fmt.Errorf("PROFILE_INVALID: %s", strings.TrimSpace(validation))
	}
	var parsed profileValidationEnvelope
	if err := json.Unmarshal([]byte(validation), &parsed); err != nil {
		return "", err
	}
	env := fields["env"]
	if env == "" {
		env = filepath.Base(parsed.Data.Normalized.StateDir)
	}
	if env == "" || env == "." {
		env = parsed.Data.Profile.Name
	}
	if err := validateTokenPart(env); err != nil {
		return "", err
	}
	root, err := infraLabRoot()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(root, "state", env)); err == nil {
		return "", fmt.Errorf("ENV_ALREADY_EXISTS: %s", env)
	} else if !os.IsNotExist(err) {
		return "", err
	}

	profileDigest := digest("profile", profile, validation)
	now := time.Now().UTC()
	op := operationRecord{
		OperationID: operationID("env_up"),
		Tool:        "env_up",
		Status:      "PREPARED",
		Risk:        "HIGH",
		Destructive: false,
		Target: operationTarget{
			Env:               env,
			Profile:           profile,
			ProfileDigest:     profileDigest,
			TargetFingerprint: digest("env_up", env, profile, profileDigest),
		},
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(1 * time.Hour).Format(time.RFC3339),
		Approval: operationApproval{
			Required: true,
			Status:   "required",
			Mode:     "token-v1",
		},
		Steps: []operationStep{
			{Name: "validate profile", Status: "succeeded"},
			{Name: "collect pre-snapshot", Status: "pending"},
			{Name: "run env up", Status: "pending"},
			{Name: "collect post-snapshot", Status: "pending"},
		},
	}
	token, err := approvalToken(op)
	if err != nil {
		return "", err
	}
	op.Approval.TokenHint = token[:min(18, len(token))]
	path, err := writeOperation(op)
	if err != nil {
		return "", err
	}
	data := operationPrepareData{
		OperationID:       op.OperationID,
		ApprovalToken:     token,
		ExpiresAt:         op.ExpiresAt,
		PlanFingerprint:   digest("plan", "env_up", env, profile, profileDigest),
		TargetFingerprint: op.Target.TargetFingerprint,
		Approval:          op.Approval,
		Risk:              op.Risk,
		Target:            op.Target,
		Steps:             []string{"validate profile", "collect pre-snapshot", "run env up", "collect post-snapshot"},
		Operation:         op,
		OperationPath:     path,
	}
	return encodeOperationEnvelope("env.up.prepare", data)
}

func commitEnvUp(fields map[string]string, timeout time.Duration) (string, error) {
	op, err := verifyPreparedOperation(fields, "env_up")
	if err != nil {
		return "", err
	}
	root, err := infraLabRoot()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(root, "state", op.Target.Env)); err == nil {
		return "", fmt.Errorf("ENV_ALREADY_EXISTS: %s", op.Target.Env)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	release, err := acquireEnvLock(op)
	if err != nil {
		return "", err
	}
	defer release()
	if _, err := appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "env_up_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      map[string]string{"env": op.Target.Env, "profile": op.Target.Profile},
		Result:      "started",
	}); err != nil {
		return "", fmt.Errorf("AUDIT_WRITE_FAILED: %w", err)
	}

	op.Status = "RUNNING"
	op.StartedAt = time.Now().UTC().Format(time.RFC3339)
	op.Approval.Status = "approved"
	markStep(&op, "collect pre-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "pre-snapshot.json", op.Target.Env); err != nil {
		return failOperation(op, "SNAPSHOT_FAILED", err, "collect pre-snapshot")
	}
	markStep(&op, "collect pre-snapshot", "succeeded")
	markStep(&op, "run env up", "running")
	_, _ = writeOperation(op)
	if err := runEnvUpCommand(op, timeout); err != nil {
		return failOperation(op, "COMMAND_FAILED", err, "run env up")
	}
	markStep(&op, "run env up", "succeeded")
	markStep(&op, "collect post-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "post-snapshot.json", op.Target.Env); err != nil {
		return failOperation(op, "SNAPSHOT_FAILED", err, "collect post-snapshot")
	}
	op.Status = "SUCCEEDED"
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	markStep(&op, "collect post-snapshot", "succeeded")
	_, _ = writeOperation(op)
	_, _ = appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "env_up_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      map[string]string{"env": op.Target.Env, "profile": op.Target.Profile},
		Result:      "ok",
	})
	return encodeOperationEnvelope("env.up.commit", op)
}

func prepareAddonInstall(fields map[string]string) (string, error) {
	env := fields["env"]
	addon := fields["addon"]
	if env == "" || addon == "" {
		return "", fmt.Errorf("env and addon are required")
	}
	if err := validateTokenPart(env); err != nil {
		return "", err
	}
	if err := validateTokenPart(addon); err != nil {
		return "", err
	}

	now := time.Now().UTC()
	op := operationRecord{
		OperationID: operationID("addon_install"),
		Tool:        "addon_install",
		Status:      "PREPARED",
		Risk:        "MEDIUM",
		Destructive: false,
		Target: operationTarget{
			Env:               env,
			Addon:             addon,
			TargetFingerprint: digest("addon_install", env, addon),
		},
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(1 * time.Hour).Format(time.RFC3339),
		Approval: operationApproval{
			Required: true,
			Status:   "required",
			Mode:     "token-v1",
		},
		Steps: []operationStep{
			{Name: "collect pre-snapshot", Status: "pending"},
			{Name: "run addon install", Status: "pending"},
			{Name: "run addon verify", Status: "pending"},
			{Name: "collect post-snapshot", Status: "pending"},
		},
	}
	token, err := approvalToken(op)
	if err != nil {
		return "", err
	}
	op.Approval.TokenHint = token[:min(18, len(token))]
	path, err := writeOperation(op)
	if err != nil {
		return "", err
	}
	data := operationPrepareData{
		OperationID:       op.OperationID,
		ApprovalToken:     token,
		ExpiresAt:         op.ExpiresAt,
		PlanFingerprint:   digest("plan", "addon_install", env, addon),
		TargetFingerprint: op.Target.TargetFingerprint,
		Approval:          op.Approval,
		Risk:              op.Risk,
		Target:            op.Target,
		Steps:             []string{"collect pre-snapshot", "run addon install", "run addon verify", "collect post-snapshot"},
		Operation:         op,
		OperationPath:     path,
	}
	return encodeOperationEnvelope("addon.install.prepare", data)
}

func commitAddonInstall(fields map[string]string, timeout time.Duration) (string, error) {
	op, err := verifyPreparedOperation(fields, "addon_install")
	if err != nil {
		return "", err
	}

	release, err := acquireEnvLock(op)
	if err != nil {
		return "", err
	}
	defer release()

	if _, err := appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "addon_install_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      map[string]string{"env": op.Target.Env, "addon": op.Target.Addon},
		Result:      "started",
	}); err != nil {
		return "", fmt.Errorf("AUDIT_WRITE_FAILED: %w", err)
	}

	op.Status = "RUNNING"
	op.StartedAt = time.Now().UTC().Format(time.RFC3339)
	op.Approval.Status = "approved"
	markStep(&op, "collect pre-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "pre-snapshot.json", op.Target.Env); err != nil {
		op.Status = "FAILED"
		op.ErrorCode = "SNAPSHOT_FAILED"
		op.Error = err.Error()
		markStep(&op, "collect pre-snapshot", "failed")
		_, _ = writeOperation(op)
		return "", err
	}
	markStep(&op, "collect pre-snapshot", "succeeded")
	markStep(&op, "run addon install", "running")
	_, _ = writeOperation(op)

	err = runAddonCommand(op, timeout)
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		op.Status = "FAILED"
		op.ErrorCode = "COMMAND_FAILED"
		op.Error = err.Error()
		markStep(&op, "run addon install", "failed")
		_, _ = writeOperation(op)
		_, _ = appendAudit(profileAuditRecord{
			Time:        time.Now().UTC().Format(time.RFC3339),
			OperationID: op.OperationID,
			Tool:        "addon_install_commit",
			Actor:       "agent",
			Risk:        op.Risk,
			Target:      map[string]string{"env": op.Target.Env, "addon": op.Target.Addon},
			Result:      "failed",
		})
		return "", err
	}
	op.Status = "SUCCEEDED"
	markStep(&op, "run addon install", "succeeded")
	markStep(&op, "run addon verify", "succeeded")
	markStep(&op, "collect post-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "post-snapshot.json", op.Target.Env); err != nil {
		op.Status = "FAILED"
		op.ErrorCode = "SNAPSHOT_FAILED"
		op.Error = err.Error()
		markStep(&op, "collect post-snapshot", "failed")
		_, _ = writeOperation(op)
		return "", err
	}
	markStep(&op, "collect post-snapshot", "succeeded")
	_, _ = writeOperation(op)
	_, _ = appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "addon_install_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      map[string]string{"env": op.Target.Env, "addon": op.Target.Addon},
		Result:      "ok",
	})
	return encodeOperationEnvelope("addon.install.commit", op)
}

func verifyPreparedOperation(fields map[string]string, tool string) (operationRecord, error) {
	operationID := fields["operationId"]
	token := fields["approvalToken"]
	op, err := readOperation(operationID)
	if err != nil {
		return op, err
	}
	if op.Tool != tool {
		return op, fmt.Errorf("operation tool mismatch: %s", op.Tool)
	}
	if op.Status != "PREPARED" {
		return op, fmt.Errorf("operation status must be PREPARED, got %s", op.Status)
	}
	expiresAt, err := time.Parse(time.RFC3339, op.ExpiresAt)
	if err != nil {
		return op, err
	}
	if time.Now().UTC().After(expiresAt) {
		op.Status = "EXPIRED"
		_, _ = writeOperation(op)
		return op, fmt.Errorf("APPROVAL_TOKEN_EXPIRED: operation expired")
	}
	expected, err := approvalToken(op)
	if err != nil {
		return op, err
	}
	if !hmac.Equal([]byte(expected), []byte(token)) {
		return op, fmt.Errorf("APPROVAL_TOKEN_INVALID")
	}
	return op, nil
}

func failOperation(op operationRecord, code string, err error, step string) (string, error) {
	op.Status = "FAILED"
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	op.ErrorCode = code
	op.Error = err.Error()
	markStep(&op, step, "failed")
	_, _ = writeOperation(op)
	return "", err
}

func saveOperationSnapshot(operationID, name, env string) error {
	dir, err := operationDir(operationID)
	if err != nil {
		return err
	}
	raw, err := collectSnapshot("snapshot.collect", env, 90*time.Second)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(raw), 0644)
}

func runEnvUpCommand(op operationRecord, timeout time.Duration) error {
	root, err := infraLabRoot()
	if err != nil {
		return err
	}
	dir, err := operationDir(op.OperationID)
	if err != nil {
		return err
	}
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")
	stdout, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer stderr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "bin", "ilab"), "env", "up", op.Target.Profile)
	cmd.Env = append(os.Environ(), "INFRA_LAB_ROOT="+root)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("COMMAND_TIMEOUT: env up timed out after %s", timeout)
		}
		return err
	}
	return nil
}

func runDestructiveCommand(op operationRecord, timeout time.Duration) error {
	root, err := infraLabRoot()
	if err != nil {
		return err
	}
	switch op.Tool {
	case "env_down":
		return runLoggedCommand(op, timeout, filepath.Join(root, "bin", "ilab"), "env", "down", op.Target.Env)
	case "env_clean":
		return runLoggedCommandWithEnv(op, timeout, map[string]string{"FORCE": "1"}, "bash", filepath.Join(root, "scripts", "k8s-tool.sh"), "clean")
	case "env_rebuild":
		return runLoggedCommand(op, timeout, filepath.Join(root, "bin", "ilab"), "env", "rebuild", op.Target.Profile, "--approve")
	case "addon_uninstall":
		return runLoggedCommand(op, timeout, "bash", filepath.Join(root, "scripts", "k8s-tool.sh"), "addons-uninstall", "optional", op.Target.Addon)
	default:
		return fmt.Errorf("unsupported destructive tool: %s", op.Tool)
	}
}

func runAddonCommand(op operationRecord, timeout time.Duration) error {
	root, err := infraLabRoot()
	if err != nil {
		return err
	}
	for _, args := range [][]string{
		{"addons-install", "optional", op.Target.Addon},
		{"addons-verify", "optional", op.Target.Addon},
	} {
		if err := runLoggedCommand(op, timeout, "bash", append([]string{filepath.Join(root, "scripts", "k8s-tool.sh")}, args...)...); err != nil {
			return err
		}
	}
	return nil
}

func runLoggedCommand(op operationRecord, timeout time.Duration, command string, args ...string) error {
	return runLoggedCommandWithEnv(op, timeout, nil, command, args...)
}

func runLoggedCommandWithEnv(op operationRecord, timeout time.Duration, extraEnv map[string]string, command string, args ...string) error {
	root, err := infraLabRoot()
	if err != nil {
		return err
	}
	dir, err := operationDir(op.OperationID)
	if err != nil {
		return err
	}
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")
	stdout, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer stderr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = append(os.Environ(), "INFRA_LAB_ROOT="+root, "ENV_NAME="+op.Target.Env)
	for key, value := range extraEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("COMMAND_TIMEOUT: operation command timed out after %s", timeout)
		}
		return err
	}
	return nil
}

func operationStatus(operationID string) (string, error) {
	op, err := readOperation(operationID)
	if err != nil {
		return "", err
	}
	return encodeOperationEnvelope("operation.status", op)
}

func operationLogs(operationID string) (string, error) {
	dir, err := operationDir(operationID)
	if err != nil {
		return "", err
	}
	stdout, _ := os.ReadFile(filepath.Join(dir, "stdout.log"))
	stderr, _ := os.ReadFile(filepath.Join(dir, "stderr.log"))
	return encodeOperationEnvelope("operation.logs", operationLogsData{
		OperationID: operationID,
		Stdout:      string(stdout),
		Stderr:      string(stderr),
	})
}

func writeOperation(op operationRecord) (string, error) {
	dir, err := operationDir(op.OperationID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "operation.json")
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

func readOperation(operationID string) (operationRecord, error) {
	var op operationRecord
	if operationID == "" {
		return op, fmt.Errorf("operationId is required")
	}
	dir, err := operationDir(operationID)
	if err != nil {
		return op, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "operation.json"))
	if err != nil {
		return op, fmt.Errorf("OPERATION_NOT_FOUND: %s", operationID)
	}
	if err := json.Unmarshal(data, &op); err != nil {
		return op, err
	}
	return op, nil
}

func operationDir(operationID string) (string, error) {
	base, err := operationStoreDir()
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(operationID, `/\`) || operationID == "" {
		return "", fmt.Errorf("invalid operationId: %q", operationID)
	}
	return filepath.Join(base, operationID), nil
}

func operationStoreDir() (string, error) {
	if dir := os.Getenv("INFRA_LAB_OPERATION_STORE"); dir != "" {
		return dir, nil
	}
	root, err := infraLabRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state", ".operations"), nil
}

func acquireEnvLock(op operationRecord) (func(), error) {
	dir, err := lockDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, op.Target.Env+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("LOCK_HELD: %s", op.Target.Env)
		}
		return nil, err
	}
	lock := map[string]any{
		"operationId": op.OperationID,
		"env":         op.Target.Env,
		"tool":        op.Tool + "_commit",
		"startedAt":   time.Now().UTC().Format(time.RFC3339),
		"expiresAt":   time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339),
		"pid":         os.Getpid(),
	}
	if hostname, err := os.Hostname(); err == nil {
		lock["hostname"] = hostname
	}
	data, _ := json.MarshalIndent(lock, "", "  ")
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(path)
		return nil, writeErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return nil, closeErr
	}
	return func() { _ = os.Remove(path) }, nil
}

func lockDir() (string, error) {
	root, err := infraLabRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state", ".locks"), nil
}

func approvalToken(op operationRecord) (string, error) {
	secret, err := localSecret()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	for _, part := range []string{op.OperationID, op.Tool, op.Target.Env, op.Target.Addon, op.Target.Profile, op.Target.ProfileDigest, op.Target.TargetFingerprint, op.Risk, op.ExpiresAt} {
		_, _ = mac.Write([]byte(part))
		_, _ = mac.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(mac.Sum(nil)), nil
}

func localSecret() ([]byte, error) {
	path, err := secretPath()
	if err != nil {
		return nil, err
	}
	if data, err := os.ReadFile(path); err == nil {
		return bytes.TrimSpace(data), nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	encoded := []byte(hex.EncodeToString(secret) + "\n")
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(encoded), nil
}

func secretPath() (string, error) {
	if path := os.Getenv("INFRA_LAB_MCP_SECRET_PATH"); path != "" {
		return path, nil
	}
	configHome := infraLabConfigHome()
	if configHome == "" {
		return "", fmt.Errorf("SECRET_NOT_FOUND: config home not found")
	}
	return filepath.Join(configHome, "mcp", "secret"), nil
}

func markStep(op *operationRecord, name, status string) {
	for i := range op.Steps {
		if op.Steps[i].Name == name {
			op.Steps[i].Status = status
			return
		}
	}
}

func destructiveSteps(tool string) []operationStep {
	names := []string{"collect pre-snapshot", destructiveRunStep(tool), "collect post-snapshot"}
	out := make([]operationStep, 0, len(names))
	for _, name := range names {
		out = append(out, operationStep{Name: name, Status: "pending"})
	}
	return out
}

func destructiveRunStep(tool string) string {
	switch tool {
	case "env_down":
		return "run env down"
	case "env_clean":
		return "run env clean"
	case "env_rebuild":
		return "run env rebuild"
	case "addon_uninstall":
		return "run addon uninstall"
	default:
		return "run destructive command"
	}
}

func commandForTool(tool string) string {
	switch tool {
	case "env_down":
		return "env.down"
	case "env_clean":
		return "env.clean"
	case "env_rebuild":
		return "env.rebuild"
	case "addon_uninstall":
		return "addon.uninstall"
	default:
		return strings.ReplaceAll(tool, "_", ".")
	}
}

func auditTarget(op operationRecord) map[string]string {
	target := map[string]string{"env": op.Target.Env}
	if op.Target.Profile != "" {
		target["profile"] = op.Target.Profile
	}
	if op.Target.Addon != "" {
		target["addon"] = op.Target.Addon
	}
	return target
}

func encodeOperationEnvelope(command string, data any) (string, error) {
	env := operationEnvelope{
		OK:              true,
		Command:         command,
		ContractVersion: supportedContractVersion,
		Data:            data,
		Warnings:        []any{},
		Errors:          []interface{}{},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func validateTokenPart(value string) error {
	if strings.ContainsAny(value, `/\`) || strings.TrimSpace(value) != value {
		return fmt.Errorf("invalid value: %q", value)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
