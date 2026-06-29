package server

import (
	"bytes"
	"encoding/json"
	"time"
)

type snapshotEnvelope struct {
	OK              bool              `json:"ok"`
	Command         string            `json:"command"`
	ContractVersion string            `json:"contractVersion"`
	Data            snapshotData      `json:"data"`
	Warnings        []snapshotWarning `json:"warnings"`
	Errors          []snapshotError   `json:"errors"`
}

type snapshotWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

type snapshotError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

type snapshotData struct {
	Env      string            `json:"env,omitempty"`
	Health   snapshotHealth    `json:"health"`
	Evidence snapshotEvidence  `json:"evidence"`
	Findings []snapshotFinding `json:"findings"`
}

type snapshotHealth struct {
	Risk    string `json:"risk"`
	Summary string `json:"summary"`
}

type snapshotEvidence struct {
	EnvStatus json.RawMessage `json:"envStatus,omitempty"`
	VMs       json.RawMessage `json:"vms,omitempty"`
	K8s       json.RawMessage `json:"k8s,omitempty"`
}

type snapshotFinding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func collectSnapshot(env string, timeout time.Duration) (string, error) {
	findings := []snapshotFinding{}
	warnings := []snapshotWarning{}
	evidence := snapshotEvidence{}

	envArgs := []string{"env", "status"}
	if env != "" {
		envArgs = append(envArgs, env)
	}
	if raw, isErr, err := runILab(envArgs, timeout); err != nil {
		findings = append(findings, snapshotFinding{Code: "ENV_STATUS_FAILED", Message: err.Error()})
	} else {
		evidence.EnvStatus = json.RawMessage(raw)
		if isErr {
			findings = append(findings, snapshotFinding{Code: "ENV_STATUS_ERROR", Message: "env status returned ok:false"})
		}
	}

	if raw, isErr, err := runILab([]string{"vm", "list"}, timeout); err != nil {
		findings = append(findings, snapshotFinding{Code: "VM_LIST_FAILED", Message: err.Error()})
	} else {
		evidence.VMs = json.RawMessage(raw)
		if isErr {
			findings = append(findings, snapshotFinding{Code: "VM_LIST_ERROR", Message: "vm list returned ok:false"})
		}
	}

	k8sArgs := []string{"k8s", "status"}
	if env != "" {
		k8sArgs = append(k8sArgs, env)
	}
	if raw, isErr, err := runILab(k8sArgs, timeout); err != nil {
		findings = append(findings, snapshotFinding{Code: "K8S_STATUS_FAILED", Message: err.Error()})
	} else {
		evidence.K8s = json.RawMessage(raw)
		if isErr {
			warnings = append(warnings, snapshotWarning{Code: "K8S_STATUS_UNAVAILABLE", Message: "k8s status returned ok:false"})
			findings = append(findings, snapshotFinding{Code: "K8S_STATUS_ERROR", Message: "k8s status returned ok:false"})
		}
	}

	risk := "LOW"
	summary := "Snapshot evidence collected"
	if len(findings) > 0 {
		risk = "MEDIUM"
		summary = "Snapshot collected with findings"
	}
	if evidence.EnvStatus == nil && evidence.VMs == nil && evidence.K8s == nil {
		risk = "UNKNOWN"
		summary = "Snapshot evidence unavailable"
	}

	envOut := snapshotEnvelope{
		OK:              true,
		Command:         "snapshot.collect",
		ContractVersion: supportedContractVersion,
		Data: snapshotData{
			Env:      env,
			Health:   snapshotHealth{Risk: risk, Summary: summary},
			Evidence: evidence,
			Findings: findings,
		},
		Warnings: warnings,
		Errors:   []snapshotError{},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(envOut); err != nil {
		return "", err
	}
	return buf.String(), nil
}
