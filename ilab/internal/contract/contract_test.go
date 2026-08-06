package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleObservation() Observation {
	return Observation{
		ClusterIdentity: "uid-abc-123",
		Context:         "kubernetes-admin@kubernetes",
		KubeconfigPath:  "state/test-env/kubeconfig",
		HarborHostname:  "harbor.lab.local",
		HarborGateway:   "harbor/harbor-gateway",
		CAFileLocator:   "secret://harbor/harbor-nginx#ca.crt",
		CASHA256:        "sha256:deadbeef",
	}
}

func TestFromObservationDerivesExternalURL(t *testing.T) {
	obs := sampleObservation()
	c := FromObservation("test-env", "abc123", obs, time.Unix(0, 0))

	if c.SchemaVersion != SchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", c.SchemaVersion, SchemaVersion)
	}
	if c.Harbor.ExternalURL != "https://harbor.lab.local" {
		t.Fatalf("externalURL = %q", c.Harbor.ExternalURL)
	}
	if c.Kubernetes.ClusterIdentity != "uid-abc-123" {
		t.Fatalf("clusterIdentity = %q", c.Kubernetes.ClusterIdentity)
	}
	if c.ObservedAt != "1970-01-01T00:00:00Z" {
		t.Fatalf("observedAt = %q", c.ObservedAt)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateRejectsHollowContract(t *testing.T) {
	cases := map[string]func(*Contract){
		"bad schema":  func(c *Contract) { c.SchemaVersion = "other" },
		"no cluster":  func(c *Contract) { c.Kubernetes.ClusterIdentity = "" },
		"no hostname": func(c *Contract) { c.Harbor.Hostname = "" },
		"no ca":       func(c *Contract) { c.Harbor.CASHA256 = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := FromObservation("e", "c", sampleObservation(), time.Unix(0, 0))
			mutate(&c)
			if err := c.Validate(); err == nil {
				t.Fatalf("expected validation error for %s", name)
			}
		})
	}
}

func TestWriteIsAtomicAndDigestsWrittenBytes(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state", "test-env")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := Path(dir, "test-env")
	c := FromObservation("test-env", "abc123", sampleObservation(), time.Unix(0, 0))

	digest, err := Write(path, c)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	loaded, raw, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := Digest(raw); got != digest {
		t.Fatalf("digest mismatch: write=%s load=%s", digest, got)
	}
	if loaded.Kubernetes.ClusterIdentity != c.Kubernetes.ClusterIdentity {
		t.Fatalf("round-trip clusterIdentity mismatch")
	}
	// No stray temp files left behind.
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestContractCarriesNoSecretMaterial(t *testing.T) {
	c := FromObservation("test-env", "abc123", sampleObservation(), time.Unix(0, 0))
	data, err := Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "BEGIN") || strings.Contains(string(data), "PRIVATE") {
		t.Fatalf("contract JSON appears to contain certificate/key material:\n%s", data)
	}
	// caFile must be a locator, not bytes.
	if !strings.HasPrefix(c.Harbor.CAFile, "secret://") && !filepath.IsAbs(c.Harbor.CAFile) {
		t.Fatalf("caFile is not a locator: %q", c.Harbor.CAFile)
	}
}

func TestReconcileMatch(t *testing.T) {
	obs := sampleObservation()
	c := FromObservation("test-env", "abc123", obs, time.Unix(0, 0))
	if m := Reconcile(c, obs); len(m) != 0 {
		t.Fatalf("expected match, got mismatches: %+v", m)
	}
}

func TestReconcileDetectsClusterRecreate(t *testing.T) {
	obs := sampleObservation()
	c := FromObservation("test-env", "abc123", obs, time.Unix(0, 0))

	// Cluster rebuilt -> fresh kube-system UID.
	live := obs
	live.ClusterIdentity = "uid-fresh-999"

	m := Reconcile(c, live)
	if len(m) != 1 || m[0].Code != "CONTRACT_CLUSTER_MISMATCH" {
		t.Fatalf("expected cluster mismatch, got %+v", m)
	}
}

func TestReconcileDetectsHostnameAndTLSDrift(t *testing.T) {
	obs := sampleObservation()
	c := FromObservation("test-env", "abc123", obs, time.Unix(0, 0))

	live := obs
	live.HarborHostname = "harbor.new.local"
	live.CASHA256 = "sha256:rotated"

	m := Reconcile(c, live)
	codes := map[string]bool{}
	for _, x := range m {
		codes[x.Code] = true
	}
	if !codes["CONTRACT_HARBOR_MISMATCH"] || !codes["CONTRACT_TLS_MISMATCH"] {
		t.Fatalf("expected hostname + TLS mismatch, got %+v", m)
	}
}

func TestLoadMissingIsNotExist(t *testing.T) {
	_, _, err := Load(Path(t.TempDir(), "absent"))
	if !os.IsNotExist(err) {
		t.Fatalf("expected IsNotExist, got %v", err)
	}
}
