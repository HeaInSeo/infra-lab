package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrepareAddonInstallCreatesOperation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("INFRA_LAB_OPERATION_STORE", filepath.Join(tmp, "operations"))
	t.Setenv("INFRA_LAB_CONFIG_HOME", filepath.Join(tmp, "config"))

	raw, err := prepareAddonInstall(map[string]string{"env": "lab", "addon": "metrics-server"})
	if err != nil {
		t.Fatal(err)
	}
	var env operationEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatal(err)
	}
	if env.Command != "addon.install.prepare" {
		t.Fatalf("command = %q, want addon.install.prepare", env.Command)
	}
	data, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatal(err)
	}
	var prepare operationPrepareData
	if err := json.Unmarshal(data, &prepare); err != nil {
		t.Fatal(err)
	}
	if prepare.OperationID == "" || prepare.ApprovalToken == "" {
		t.Fatalf("missing operation id or token: %#v", prepare)
	}
	if prepare.Target.Env != "lab" || prepare.Target.Addon != "metrics-server" {
		t.Fatalf("unexpected target: %#v", prepare.Target)
	}
	if _, err := os.Stat(filepath.Join(tmp, "operations", prepare.OperationID, "operation.json")); err != nil {
		t.Fatal(err)
	}
}

func TestAddonScope(t *testing.T) {
	if got := addonScope("metrics-server"); got != "base" {
		t.Fatalf("metrics-server scope = %q, want base", got)
	}
	if got := addonScope("local-path-storage"); got != "optional" {
		t.Fatalf("local-path-storage scope = %q, want optional", got)
	}
}

func TestCleanEnvVarsTargetsEnv(t *testing.T) {
	vars := cleanEnvVars("mcp-live-multipass")
	if vars["FORCE"] != "1" {
		t.Fatalf("FORCE = %q, want 1", vars["FORCE"])
	}
	if vars["ENV_NAME"] != "mcp-live-multipass" {
		t.Fatalf("ENV_NAME = %q, want mcp-live-multipass", vars["ENV_NAME"])
	}
}

func TestApprovalTokenRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("INFRA_LAB_CONFIG_HOME", filepath.Join(tmp, "config"))
	op := operationRecord{
		OperationID: "op_test",
		Tool:        "addon_install",
		Risk:        "MEDIUM",
		ExpiresAt:   "2026-06-29T00:00:00Z",
		Target: operationTarget{
			Env:               "lab",
			Addon:             "metrics-server",
			TargetFingerprint: "sha256:test",
		},
	}
	first, err := approvalToken(op)
	if err != nil {
		t.Fatal(err)
	}
	second, err := approvalToken(op)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("token not stable: %q != %q", first, second)
	}
	if first == "" {
		t.Fatal("expected token")
	}
}

func TestOperationToolsRegisteredWithRequiredCapabilities(t *testing.T) {
	handlers := readOnlyTools(bootstrapInfo{
		InfraLabVersion: "dev",
		ContractVersion: supportedContractVersion,
		Capabilities:    map[string]bool{"env.status.v1": true, "profile.validate.v1": true},
	})
	for _, name := range []string{
		"infra_lab.addon_install_prepare",
		"infra_lab.addon_install_commit",
		"infra_lab.env_up_prepare",
		"infra_lab.env_up_commit",
		"infra_lab.operation_status",
		"infra_lab.operation_logs",
		"infra_lab.operation_approve",
		"infra_lab.operation_cancel",
		"infra_lab.operation_locks",
		"infra_lab.operation_unlock_stale",
		"infra_lab.env_down_prepare",
		"infra_lab.env_down_commit",
		"infra_lab.env_clean_prepare",
		"infra_lab.env_clean_commit",
		"infra_lab.env_rebuild_prepare",
		"infra_lab.env_rebuild_commit",
		"infra_lab.addon_uninstall_prepare",
		"infra_lab.addon_uninstall_commit",
	} {
		if _, ok := handlers[name]; !ok {
			t.Fatalf("expected %s to be registered", name)
		}
	}
}

func TestOperationApproveAllowsCommitWithoutToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("INFRA_LAB_OPERATION_STORE", filepath.Join(tmp, "operations"))
	t.Setenv("INFRA_LAB_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("INFRA_LAB_AUDIT_PATH", filepath.Join(tmp, "audit", "operations.jsonl"))

	op := operationRecord{
		OperationID: "op_approve_test",
		Tool:        "addon_install",
		Status:      "PREPARED",
		Risk:        "MEDIUM",
		Target: operationTarget{
			Env:               "lab",
			Addon:             "metrics-server",
			TargetFingerprint: "sha256:test",
		},
		ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		Approval:  operationApproval{Required: true, Status: "required", Mode: "token-v1"},
	}
	if _, err := writeOperation(op); err != nil {
		t.Fatal(err)
	}
	if _, err := operationApprove(op.OperationID); err != nil {
		t.Fatal(err)
	}
	verified, err := verifyPreparedOperation(map[string]string{"operationId": op.OperationID}, "addon_install")
	if err != nil {
		t.Fatal(err)
	}
	if verified.Status != "APPROVED" || verified.Approval.Status != "approved" {
		t.Fatalf("unexpected approved operation: %#v", verified)
	}
}

func TestOperationCancel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("INFRA_LAB_OPERATION_STORE", filepath.Join(tmp, "operations"))
	t.Setenv("INFRA_LAB_AUDIT_PATH", filepath.Join(tmp, "audit", "operations.jsonl"))

	op := operationRecord{
		OperationID: "op_cancel_test",
		Tool:        "addon_install",
		Status:      "PREPARED",
		Risk:        "MEDIUM",
		Target:      operationTarget{Env: "lab", Addon: "metrics-server"},
		ExpiresAt:   time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		Approval:    operationApproval{Required: true, Status: "required", Mode: "token-v1"},
	}
	if _, err := writeOperation(op); err != nil {
		t.Fatal(err)
	}
	if _, err := operationCancel(op.OperationID); err != nil {
		t.Fatal(err)
	}
	cancelled, err := readOperation(op.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "CANCELLED" {
		t.Fatalf("status = %q, want CANCELLED", cancelled.Status)
	}
}

func TestOperationLocksAndUnlockStale(t *testing.T) {
	tmp := t.TempDir()
	lockDir := filepath.Join(tmp, "locks")
	t.Setenv("INFRA_LAB_LOCK_DIR", lockDir)
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(lockDir, "lab.lock")
	lockJSON := `{"operationId":"op_lock","env":"lab","tool":"addon_install_commit","startedAt":"2026-01-01T00:00:00Z","expiresAt":"2026-01-01T01:00:00Z","pid":123,"hostname":"test"}`
	if err := os.WriteFile(lockPath, []byte(lockJSON+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	raw, err := operationLocks()
	if err != nil {
		t.Fatal(err)
	}
	var env operationEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatal(err)
	}
	var locks operationLocksData
	if err := json.Unmarshal(data, &locks); err != nil {
		t.Fatal(err)
	}
	if len(locks.Locks) != 1 || !locks.Locks[0].Stale {
		t.Fatalf("unexpected locks: %#v", locks)
	}
	if _, err := operationUnlockStale("lab"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock should be removed, stat err=%v", err)
	}
}
