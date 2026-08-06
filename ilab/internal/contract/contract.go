// Package contract models the infra-lab runtime context contract:
// state/<env>/runtime-contract.json. The contract is a secret-free record of
// what infra-lab observed live when it stood up an environment (cluster
// identity, Harbor hostname, TLS CA fingerprint). Consumers must reconcile it
// against the live cluster/TLS before trusting any endpoint — a contract that
// is missing or disagrees with the live cluster fails closed.
//
// This package is deliberately pure: it never shells out and never touches the
// network. Live observation is injected as an Observation by the caller, which
// keeps write, reconcile, and digest logic deterministic and unit-testable.
package contract

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SchemaVersion identifies the runtime-context contract shape. Any breaking
// change to the fields below must bump this string.
const SchemaVersion = "infra-lab.runtime-context.v1"

// FileName is the fixed basename under state/<env>/.
const FileName = "runtime-contract.json"

// Contract is the on-disk runtime context. All values are observed live at
// capture time. It carries no secret material: caFile is a locator only and
// caSHA256 is a public certificate fingerprint.
type Contract struct {
	SchemaVersion  string        `json:"schemaVersion"`
	EnvName        string        `json:"envName"`
	InfraLabCommit string        `json:"infraLabCommit"`
	Kubernetes     KubernetesRef `json:"kubernetes"`
	Harbor         HarborRef     `json:"harbor"`
	ObservedAt     string        `json:"observedAt"`
}

// KubernetesRef pins the environment to a specific live cluster. ClusterIdentity
// is the stable fingerprint (kube-system namespace UID) used to detect a
// recreated cluster: a rebuilt cluster gets a fresh UID, so an old contract no
// longer reconciles and callers fail closed.
type KubernetesRef struct {
	KubeconfigPath  string `json:"kubeconfigPath"`
	Context         string `json:"context"`
	ClusterIdentity string `json:"clusterIdentity"`
}

// HarborRef records the observed Harbor front door. CAFile is a secret locator
// (a path or in-cluster secret reference) — never the CA bytes. CASHA256 is the
// CA certificate fingerprint used to detect TLS drift.
type HarborRef struct {
	Hostname    string `json:"hostname"`
	ExternalURL string `json:"externalURL"`
	Gateway     string `json:"gateway"`
	CAFile      string `json:"caFile"`
	CASHA256    string `json:"caSHA256"`
}

// Observation is a live reading of the cluster and Harbor front door. It is the
// single source both capture (to build a contract) and resolve (to reconcile an
// existing contract) consume. Callers populate it from the live cluster/TLS;
// tests populate it from fixtures.
type Observation struct {
	ClusterIdentity string
	Context         string
	KubeconfigPath  string
	HarborHostname  string
	HarborGateway   string
	CAFileLocator   string
	CASHA256        string
}

// FromObservation builds a Contract from a live Observation. observedAt is
// injected so callers (and tests) control the timestamp.
func FromObservation(envName, infraLabCommit string, obs Observation, observedAt time.Time) Contract {
	hostname := obs.HarborHostname
	externalURL := ""
	if hostname != "" {
		externalURL = "https://" + hostname
	}
	return Contract{
		SchemaVersion:  SchemaVersion,
		EnvName:        envName,
		InfraLabCommit: infraLabCommit,
		Kubernetes: KubernetesRef{
			KubeconfigPath:  obs.KubeconfigPath,
			Context:         obs.Context,
			ClusterIdentity: obs.ClusterIdentity,
		},
		Harbor: HarborRef{
			Hostname:    hostname,
			ExternalURL: externalURL,
			Gateway:     obs.HarborGateway,
			CAFile:      obs.CAFileLocator,
			CASHA256:    obs.CASHA256,
		},
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
	}
}

// Validate rejects a contract that is structurally incomplete. A contract that
// cannot pin a cluster identity or CA fingerprint is unusable: reconcile would
// have nothing to check against, so we refuse it rather than let a hollow
// contract pass live checks by comparing empty to empty.
func (c Contract) Validate() error {
	switch {
	case c.SchemaVersion != SchemaVersion:
		return fmt.Errorf("unsupported schemaVersion %q (want %q)", c.SchemaVersion, SchemaVersion)
	case c.Kubernetes.ClusterIdentity == "":
		return fmt.Errorf("kubernetes.clusterIdentity is empty")
	case c.Harbor.Hostname == "":
		return fmt.Errorf("harbor.hostname is empty")
	case c.Harbor.CASHA256 == "":
		return fmt.Errorf("harbor.caSHA256 is empty")
	}
	return nil
}

// Marshal renders the contract as stable, indented JSON with a trailing
// newline, matching the ilab JSON envelope style. The byte form is what gets
// digested, so it must be deterministic.
func Marshal(c Contract) ([]byte, error) {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Digest returns the hex sha256 of the given bytes. Used as verification
// evidence for both the written contract and resolve results.
func Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf("sha256:%x", sum)
}

// Write atomically writes the contract to path (temp file in the same directory
// + rename) so a reader never observes a half-written contract. It returns the
// digest of the exact bytes written. The parent directory must already exist —
// it is state/<env>/, created by env-up.
func Write(path string, c Contract) (string, error) {
	data, err := Marshal(c)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".runtime-contract-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return "", err
	}
	return Digest(data), nil
}

// Load reads and parses the contract at path. It returns the parsed contract,
// the raw bytes (for digesting), and any error. A missing file is reported via
// os.IsNotExist so callers can map it to a fail-closed CONTRACT_MISSING.
func Load(path string) (Contract, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Contract{}, nil, err
	}
	var c Contract
	if err := json.Unmarshal(raw, &c); err != nil {
		return Contract{}, raw, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, raw, nil
}

// Path returns state/<env>/runtime-contract.json under root.
func Path(root, envName string) string {
	return filepath.Join(root, "state", envName, FileName)
}

// Mismatch describes one field where the stored contract disagrees with the
// live observation. A non-empty slice means the caller must fail closed.
type Mismatch struct {
	Field    string
	Code     string
	Expected string
	Actual   string
}

// Reconcile compares a stored contract against a live observation and returns
// every field that disagrees. An empty result means the contract still
// describes the live cluster and Harbor front door. This is the check that
// makes a recreated cluster (new clusterIdentity) or rotated CA (new caSHA256)
// invalidate an old contract automatically.
func Reconcile(c Contract, obs Observation) []Mismatch {
	var out []Mismatch
	if c.Kubernetes.ClusterIdentity != obs.ClusterIdentity {
		out = append(out, Mismatch{
			Field:    "kubernetes.clusterIdentity",
			Code:     "CONTRACT_CLUSTER_MISMATCH",
			Expected: c.Kubernetes.ClusterIdentity,
			Actual:   obs.ClusterIdentity,
		})
	}
	if c.Harbor.Hostname != obs.HarborHostname {
		out = append(out, Mismatch{
			Field:    "harbor.hostname",
			Code:     "CONTRACT_HARBOR_MISMATCH",
			Expected: c.Harbor.Hostname,
			Actual:   obs.HarborHostname,
		})
	}
	if c.Harbor.CASHA256 != obs.CASHA256 {
		out = append(out, Mismatch{
			Field:    "harbor.caSHA256",
			Code:     "CONTRACT_TLS_MISMATCH",
			Expected: c.Harbor.CASHA256,
			Actual:   obs.CASHA256,
		})
	}
	return out
}
