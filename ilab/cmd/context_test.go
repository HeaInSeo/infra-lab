package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HeaInSeo/infra-lab/ilab/internal/contract"
	"github.com/HeaInSeo/infra-lab/ilab/internal/output"
)

// fakeObserver returns a canned live observation (or error), standing in for a
// real cluster so resolver logic is testable without kubectl.
type fakeObserver struct {
	obs contract.Observation
	err error
}

func (f fakeObserver) Observe(_, _ string) (contract.Observation, error) {
	return f.obs, f.err
}

func liveObs() contract.Observation {
	return contract.Observation{
		ClusterIdentity: "uid-live-1",
		Context:         "kubernetes-admin@kubernetes",
		KubeconfigPath:  "state/test-env/kubeconfig",
		HarborHostname:  "harbor.lab.local",
		HarborGateway:   "harbor/harbor-gateway",
		CAFileLocator:   "secret://harbor/harbor-nginx#ca.crt",
		CASHA256:        "sha256:aaaa",
	}
}

// newRoot lays out an infra-lab root with a state/<env>/ dir so FindRoot and
// contract.Path resolve.
func newRoot(t *testing.T, env string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "k8s-tool.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "state", env), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var ce *output.ContractError
	if !errors.As(err, &ce) {
		t.Fatalf("error is not a ContractError: %v", err)
	}
	return ce.Code
}

// Completion criterion: capture then resolve succeeds with no endpoint argument.
func TestCaptureThenResolveSucceeds(t *testing.T) {
	root := newRoot(t, "test-env")
	obs := fakeObserver{obs: liveObs()}

	if _, err := captureContract(root, "test-env", obs, func() time.Time { return time.Unix(0, 0) }); err != nil {
		t.Fatalf("capture: %v", err)
	}
	data, err := resolveContract(root, "test-env", obs)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !data.Reconciled {
		t.Fatalf("expected reconciled=true")
	}
	if data.HarborHostname != "harbor.lab.local" {
		t.Fatalf("hostname = %q", data.HarborHostname)
	}
	if data.ContractDigest == "" || data.ResolveDigest == "" {
		t.Fatalf("expected evidence digests, got contract=%q resolve=%q", data.ContractDigest, data.ResolveDigest)
	}
}

// Completion criterion: missing contract fails closed.
func TestResolveMissingContractFailsClosed(t *testing.T) {
	root := newRoot(t, "test-env")
	_, err := resolveContract(root, "test-env", fakeObserver{obs: liveObs()})
	if err == nil {
		t.Fatal("expected fail-closed error for missing contract")
	}
	if code := codeOf(t, err); code != "CONTRACT_MISSING" {
		t.Fatalf("code = %q, want CONTRACT_MISSING", code)
	}
}

// Completion criterion: cluster recreate (new identity) auto-invalidates an old
// contract via reconcile mismatch — resolve fails closed, emits no endpoint.
func TestResolveClusterRecreateFailsClosed(t *testing.T) {
	root := newRoot(t, "test-env")
	if _, err := captureContract(root, "test-env", fakeObserver{obs: liveObs()},
		func() time.Time { return time.Unix(0, 0) }); err != nil {
		t.Fatalf("capture: %v", err)
	}

	rebuilt := liveObs()
	rebuilt.ClusterIdentity = "uid-rebuilt-2"
	data, err := resolveContract(root, "test-env", fakeObserver{obs: rebuilt})
	if err == nil {
		t.Fatal("expected fail-closed error after cluster recreate")
	}
	if data.HarborHostname != "" {
		t.Fatalf("no endpoint must be emitted on mismatch, got %q", data.HarborHostname)
	}
	if code := codeOf(t, err); code != "CONTRACT_CLUSTER_MISMATCH" {
		t.Fatalf("code = %q, want CONTRACT_CLUSTER_MISMATCH", code)
	}
}

// Completion criterion: a stale hostname in the contract fails against the live
// cluster (old hostname does not resolve).
func TestResolveStaleHostnameFailsClosed(t *testing.T) {
	root := newRoot(t, "test-env")
	// Capture a contract whose Harbor hostname is the OLD front door.
	stale := liveObs()
	stale.HarborHostname = "harbor.old.local"
	if _, err := captureContract(root, "test-env", fakeObserver{obs: stale},
		func() time.Time { return time.Unix(0, 0) }); err != nil {
		t.Fatalf("capture: %v", err)
	}
	// Live cluster now serves the new hostname.
	_, err := resolveContract(root, "test-env", fakeObserver{obs: liveObs()})
	if err == nil {
		t.Fatal("expected fail-closed error for stale hostname")
	}
	if code := codeOf(t, err); code != "CONTRACT_HARBOR_MISMATCH" {
		t.Fatalf("code = %q, want CONTRACT_HARBOR_MISMATCH", code)
	}
}

// Completion criterion: cannot observe live cluster -> fail closed, never
// emits the contract's stored endpoint on faith.
func TestResolveUnreachableFailsClosed(t *testing.T) {
	root := newRoot(t, "test-env")
	if _, err := captureContract(root, "test-env", fakeObserver{obs: liveObs()},
		func() time.Time { return time.Unix(0, 0) }); err != nil {
		t.Fatalf("capture: %v", err)
	}
	sentinel := output.NewError("CLUSTER_UNREACHABLE", "down", output.ExitDomain)
	_, err := resolveContract(root, "test-env", fakeObserver{err: sentinel})
	if err == nil {
		t.Fatal("expected fail-closed error when cluster is unreachable")
	}
	if code := codeOf(t, err); code != "CLUSTER_UNREACHABLE" {
		t.Fatalf("code = %q, want CLUSTER_UNREACHABLE", code)
	}
}

// Completion criterion: --format shell output contains no secret material.
func TestShellOutputHasNoSecret(t *testing.T) {
	root := newRoot(t, "test-env")
	if _, err := captureContract(root, "test-env", fakeObserver{obs: liveObs()},
		func() time.Time { return time.Unix(0, 0) }); err != nil {
		t.Fatalf("capture: %v", err)
	}
	data, err := resolveContract(root, "test-env", fakeObserver{obs: liveObs()})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	var buf bytes.Buffer
	if err := writeResolveShell(&buf, data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, forbidden := range []string{"BEGIN", "PRIVATE", "ca.crt\n", "-----"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("shell output contains secret marker %q:\n%s", forbidden, out)
		}
	}
	if !strings.Contains(out, "export ILAB_HARBOR_HOSTNAME='harbor.lab.local'") {
		t.Fatalf("shell output missing expected export:\n%s", out)
	}
	// The CA locator is a reference, not bytes; that is acceptable.
	if !strings.Contains(out, "export ILAB_HARBOR_CA_FILE='secret://harbor/harbor-nginx#ca.crt'") {
		t.Fatalf("shell output missing CA locator export:\n%s", out)
	}
}

// Completion criterion: contract JSON and resolve result carry a digest as
// verification evidence, and the contract digest is stable across reads.
func TestDigestsAsEvidence(t *testing.T) {
	root := newRoot(t, "test-env")
	cap, err := captureContract(root, "test-env", fakeObserver{obs: liveObs()},
		func() time.Time { return time.Unix(0, 0) })
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	data, err := resolveContract(root, "test-env", fakeObserver{obs: liveObs()})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cap.ContractDigest != data.ContractDigest {
		t.Fatalf("contract digest unstable: capture=%s resolve=%s", cap.ContractDigest, data.ContractDigest)
	}
	if !strings.HasPrefix(data.ResolveDigest, "sha256:") {
		t.Fatalf("resolve digest not a sha256: %q", data.ResolveDigest)
	}
}

func TestCaptureRequiresEnv(t *testing.T) {
	root := newRoot(t, "test-env")
	_, err := captureContract(root, "", fakeObserver{obs: liveObs()}, time.Now)
	if err == nil || codeOf(t, err) != "ENV_REQUIRED" {
		t.Fatalf("expected ENV_REQUIRED, got %v", err)
	}
}
