package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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

func TestOperationToolsRegisteredWithEnvStatusCapability(t *testing.T) {
	handlers := readOnlyTools(map[string]bool{"env.status.v1": true})
	for _, name := range []string{
		"infra_lab.addon_install_prepare",
		"infra_lab.addon_install_commit",
		"infra_lab.operation_status",
		"infra_lab.operation_logs",
	} {
		if _, ok := handlers[name]; !ok {
			t.Fatalf("expected %s to be registered", name)
		}
	}
}
