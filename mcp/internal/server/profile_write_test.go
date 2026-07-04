package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProfileWriteDirOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("INFRA_LAB_PROFILE_DIR", dir)
	got, err := profileWriteDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("profileWriteDir = %q, want %q", got, dir)
	}
}

func TestProfileForWriteDefaults(t *testing.T) {
	profile, err := profileForWrite("save_as", map[string]string{"name": "lab"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Backend != "multipass" {
		t.Fatalf("backend = %q, want multipass", profile.Backend)
	}
	if profile.Kubernetes.CNI != "flannel" {
		t.Fatalf("cni = %q, want flannel", profile.Kubernetes.CNI)
	}
	if profile.VM.Masters != 1 || profile.VM.Workers != 2 {
		t.Fatalf("node counts = %d/%d, want 1/2", profile.VM.Masters, profile.VM.Workers)
	}
}

func TestAppendAuditWritesJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit", "operations.jsonl")
	t.Setenv("INFRA_LAB_AUDIT_PATH", path)
	got, err := appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: "op_test",
		Tool:        "profile_save_as",
		Actor:       "agent",
		Risk:        "LOW",
		Target:      map[string]string{"profile": "lab"},
		Result:      "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("audit path = %q, want %q", got, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("audit lines = %d, want 1", len(lines))
	}
	var decoded profileAuditRecord
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OperationID != "op_test" || decoded.Result != "ok" {
		t.Fatalf("unexpected audit record: %#v", decoded)
	}
}

func TestProfileWriteToolsRegisteredWithValidateCapability(t *testing.T) {
	handlers := readOnlyTools(bootstrapInfo{
		InfraLabVersion: "dev",
		ContractVersion: supportedContractVersion,
		Capabilities:    map[string]bool{"profile.validate.v1": true},
	})
	for _, name := range []string{
		"infra_lab.profile_clone",
		"infra_lab.profile_save_as",
		"infra_lab.profile_validate_and_save",
	} {
		if _, ok := handlers[name]; !ok {
			t.Fatalf("expected %s to be registered", name)
		}
	}
}
