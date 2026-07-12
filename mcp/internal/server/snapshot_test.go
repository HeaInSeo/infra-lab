package server

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestCollectSnapshotLowRisk(t *testing.T) {
	raw, err := collectSnapshotWithRunner("snapshot.collect", "", time.Second, func(args []string, _ time.Duration) (string, bool, error) {
		return okEnvelope(commandName(args)), false, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	env := decodeSnapshot(t, raw)
	if env.Command != "snapshot.collect" {
		t.Fatalf("command = %q, want snapshot.collect", env.Command)
	}
	if env.Data.Health.Risk != "LOW" {
		t.Fatalf("risk = %q, want LOW", env.Data.Health.Risk)
	}
	if len(env.Data.Findings) != 0 {
		t.Fatalf("findings = %#v, want empty", env.Data.Findings)
	}
}

func TestCollectSnapshotK8sUnavailable(t *testing.T) {
	raw, err := collectSnapshotWithRunner("health.summarize", "lab", time.Second, func(args []string, _ time.Duration) (string, bool, error) {
		if reflect.DeepEqual(args, []string{"k8s", "status", "lab"}) {
			return failEnvelope("k8s.status"), true, nil
		}
		return okEnvelope(commandName(args)), false, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	env := decodeSnapshot(t, raw)
	if env.Command != "health.summarize" {
		t.Fatalf("command = %q, want health.summarize", env.Command)
	}
	if env.Data.Env != "lab" {
		t.Fatalf("env = %q, want lab", env.Data.Env)
	}
	if env.Data.Health.Risk != "MEDIUM" {
		t.Fatalf("risk = %q, want MEDIUM", env.Data.Health.Risk)
	}
	if len(env.Warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1", len(env.Warnings))
	}
}

func TestCollectSnapshotUnknownRiskWhenAllEvidenceFails(t *testing.T) {
	raw, err := collectSnapshotWithRunner("snapshot.collect", "", time.Second, func(_ []string, _ time.Duration) (string, bool, error) {
		return "", false, errors.New("boom")
	})
	if err != nil {
		t.Fatal(err)
	}

	env := decodeSnapshot(t, raw)
	if env.Data.Health.Risk != "UNKNOWN" {
		t.Fatalf("risk = %q, want UNKNOWN", env.Data.Health.Risk)
	}
	if len(env.Data.Findings) != 4 {
		t.Fatalf("findings len = %d, want 4", len(env.Data.Findings))
	}
}

func TestCollectSnapshotIncludesDoctorFindings(t *testing.T) {
	raw, err := collectSnapshotWithRunner("snapshot.collect", "", time.Second, func(args []string, _ time.Duration) (string, bool, error) {
		if reflect.DeepEqual(args, []string{"doctor"}) {
			return `{"ok":true,"command":"doctor","contractVersion":"infra-lab.contract/v1","data":{"findings":[{"code":"LIBVIRT_VM_PAUSED","message":"vm paused"}]},"warnings":[],"errors":[]}` + "\n", false, nil
		}
		return okEnvelope(commandName(args)), false, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	env := decodeSnapshot(t, raw)
	if len(env.Data.Findings) == 0 || env.Data.Findings[0].Code != "LIBVIRT_VM_PAUSED" {
		t.Fatalf("findings = %#v, want doctor finding", env.Data.Findings)
	}
	if env.Data.Health.Risk != "MEDIUM" {
		t.Fatalf("risk = %q, want MEDIUM", env.Data.Health.Risk)
	}
}

func decodeSnapshot(t *testing.T, raw string) snapshotEnvelope {
	t.Helper()
	var env snapshotEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatal(err)
	}
	return env
}

func okEnvelope(command string) string {
	return `{"ok":true,"command":"` + command + `","contractVersion":"infra-lab.contract/v1","data":{},"warnings":[],"errors":[]}` + "\n"
}

func failEnvelope(command string) string {
	return `{"ok":false,"command":"` + command + `","contractVersion":"infra-lab.contract/v1","data":null,"warnings":[],"errors":[{"code":"FAILED","message":"failed"}]}` + "\n"
}
