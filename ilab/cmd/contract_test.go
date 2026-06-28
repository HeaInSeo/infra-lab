package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/HeaInSeo/infra-lab/ilab/internal/output"
)

func TestGoldenVersionContract(t *testing.T) {
	data := versionData{
		InfraLabVersion: "dev",
		GitCommit:       "unknown",
		BuildDate:       "unknown",
	}
	assertGoldenEnvelope(t, "version.golden.json", output.Success("version", data))
}

func TestGoldenCapabilitiesContract(t *testing.T) {
	assertGoldenEnvelope(t, "capabilities.golden.json", output.Success("capabilities", currentCapabilitiesData()))
}

func TestGoldenProfileValidateErrorContract(t *testing.T) {
	env := output.Failure("profile.validate", []output.ErrorInfo{{
		Code:    "PROFILE_INVALID",
		Message: "libvirt.sshPublicKey is required",
		Field:   "libvirt.sshPublicKey",
	}})
	assertGoldenEnvelope(t, "profile_validate_error.golden.json", env)
}

func assertGoldenEnvelope(t *testing.T, name string, env output.Envelope) {
	t.Helper()

	var got bytes.Buffer
	if err := output.WriteJSON(&got, env); err != nil {
		t.Fatal(err)
	}

	goldenPath := filepath.Join("..", "testdata", "contracts", name)
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}

	var gotJSON any
	if err := json.Unmarshal(got.Bytes(), &gotJSON); err != nil {
		t.Fatalf("generated invalid JSON: %v", err)
	}
	var wantJSON any
	if err := json.Unmarshal(want, &wantJSON); err != nil {
		t.Fatalf("golden invalid JSON: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(got.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got.String(), string(want))
	}
}
