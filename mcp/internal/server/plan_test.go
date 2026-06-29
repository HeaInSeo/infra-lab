package server

import (
	"encoding/json"
	"testing"
)

func TestCreatePlanEnvUp(t *testing.T) {
	raw, err := createPlan([]string{"env_up", "profile=lab"})
	if err != nil {
		t.Fatal(err)
	}
	env := decodePlan(t, raw)
	if env.Command != "plan.up" {
		t.Fatalf("command = %q, want plan.up", env.Command)
	}
	if env.Data.Destructive {
		t.Fatal("env_up should not be destructive")
	}
	if env.Data.Risk != "MEDIUM" {
		t.Fatalf("risk = %q, want MEDIUM", env.Data.Risk)
	}
	if env.Data.PlanFingerprint == "" || env.Data.TargetFingerprint == "" {
		t.Fatal("expected fingerprints")
	}
}

func TestCreatePlanRebuild(t *testing.T) {
	raw, err := createPlan([]string{"rebuild", "env=lab", "profile=lab"})
	if err != nil {
		t.Fatal(err)
	}
	env := decodePlan(t, raw)
	if env.Command != "plan.rebuild" {
		t.Fatalf("command = %q, want plan.rebuild", env.Command)
	}
	if !env.Data.Destructive {
		t.Fatal("rebuild should be destructive")
	}
	if env.Data.Risk != "HIGH" {
		t.Fatalf("risk = %q, want HIGH", env.Data.Risk)
	}
}

func TestCreatePlanAddonUninstall(t *testing.T) {
	raw, err := createPlan([]string{"addon_uninstall", "env=lab", "addon=metrics-server"})
	if err != nil {
		t.Fatal(err)
	}
	env := decodePlan(t, raw)
	if !env.Data.Destructive {
		t.Fatal("addon_uninstall should be destructive")
	}
	if env.Data.Addon != "metrics-server" {
		t.Fatalf("addon = %q, want metrics-server", env.Data.Addon)
	}
}

func decodePlan(t *testing.T, raw string) planEnvelope {
	t.Helper()
	var env planEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatal(err)
	}
	return env
}
