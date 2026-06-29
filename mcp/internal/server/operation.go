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
	case "status":
		return operationStatus(fields["operationId"])
	case "logs":
		return operationLogs(fields["operationId"])
	default:
		return "", fmt.Errorf("unsupported operation action: %s", action)
	}
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
	operationID := fields["operationId"]
	token := fields["approvalToken"]
	op, err := readOperation(operationID)
	if err != nil {
		return "", err
	}
	if op.Tool != "addon_install" {
		return "", fmt.Errorf("operation tool mismatch: %s", op.Tool)
	}
	if op.Status != "PREPARED" {
		return "", fmt.Errorf("operation status must be PREPARED, got %s", op.Status)
	}
	expiresAt, err := time.Parse(time.RFC3339, op.ExpiresAt)
	if err != nil {
		return "", err
	}
	if time.Now().UTC().After(expiresAt) {
		op.Status = "EXPIRED"
		_, _ = writeOperation(op)
		return "", fmt.Errorf("APPROVAL_TOKEN_EXPIRED: operation expired")
	}
	expected, err := approvalToken(op)
	if err != nil {
		return "", err
	}
	if !hmac.Equal([]byte(expected), []byte(token)) {
		return "", fmt.Errorf("APPROVAL_TOKEN_INVALID")
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

func runAddonCommand(op operationRecord, timeout time.Duration) error {
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
	for _, args := range [][]string{
		{"addons-install", "optional", op.Target.Addon},
		{"addons-verify", "optional", op.Target.Addon},
	} {
		cmd := exec.CommandContext(ctx, "bash", append([]string{filepath.Join(root, "scripts", "k8s-tool.sh")}, args...)...)
		cmd.Env = append(os.Environ(), "INFRA_LAB_ROOT="+root, "ENV_NAME="+op.Target.Env)
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("COMMAND_TIMEOUT: addon command timed out after %s", timeout)
			}
			return err
		}
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
		"tool":        "addon_install_commit",
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
	for _, part := range []string{op.OperationID, op.Tool, op.Target.Env, op.Target.Addon, op.Target.TargetFingerprint, op.Risk, op.ExpiresAt} {
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
