package output

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSuccessEnvelopeHasRequiredFields(t *testing.T) {
	env := Success("version", map[string]string{"infraLabVersion": "dev"})

	if !env.OK {
		t.Fatal("expected ok=true")
	}
	if env.Command != "version" {
		t.Fatalf("command = %q, want version", env.Command)
	}
	if env.ContractVersion != ContractVersion {
		t.Fatalf("contractVersion = %q, want %q", env.ContractVersion, ContractVersion)
	}
	if env.Warnings == nil {
		t.Fatal("warnings must be a non-nil empty slice")
	}
	if env.Errors == nil {
		t.Fatal("errors must be a non-nil empty slice")
	}
}

func TestWriteJSONEncodesEmptyArrays(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, Success("capabilities", map[string]any{})); err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["warnings"].([]any); !ok {
		t.Fatalf("warnings encoded as %T, want array", decoded["warnings"])
	}
	if _, ok := decoded["errors"].([]any); !ok {
		t.Fatalf("errors encoded as %T, want array", decoded["errors"])
	}
}

func TestFailureEnvelopeUsesNullData(t *testing.T) {
	var buf bytes.Buffer
	env := Failure("profile.validate", []ErrorInfo{{Code: "PROFILE_INVALID", Message: "invalid"}})
	if err := WriteJSON(&buf, env); err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["data"] != nil {
		t.Fatalf("data = %#v, want nil", decoded["data"])
	}
}
