package server

import (
	"encoding/json"
	"fmt"
	"time"
)

const supportedContractVersion = "infra-lab.contract/v1"

type bootstrapInfo struct {
	InfraLabVersion string
	ContractVersion string
	Capabilities    map[string]bool
}

type ilabEnvelope struct {
	OK              bool            `json:"ok"`
	Command         string          `json:"command"`
	ContractVersion string          `json:"contractVersion"`
	Data            json.RawMessage `json:"data"`
	Errors          []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

type capabilitiesPayload struct {
	InfraLabVersion string   `json:"infraLabVersion"`
	ContractVersion string   `json:"contractVersion"`
	Capabilities    []string `json:"capabilities"`
}

func bootstrap() (bootstrapInfo, error) {
	versionRaw, _, err := runILab([]string{"version"}, 30*time.Second)
	if err != nil {
		return bootstrapInfo{}, fmt.Errorf("bootstrap version: %w", err)
	}
	if err := validateEnvelope(versionRaw, "version"); err != nil {
		return bootstrapInfo{}, fmt.Errorf("bootstrap version: %w", err)
	}

	capabilitiesRaw, _, err := runILab([]string{"capabilities"}, 30*time.Second)
	if err != nil {
		return bootstrapInfo{}, fmt.Errorf("bootstrap capabilities: %w", err)
	}
	var env ilabEnvelope
	if err := json.Unmarshal([]byte(capabilitiesRaw), &env); err != nil {
		return bootstrapInfo{}, fmt.Errorf("parse capabilities envelope: %w", err)
	}
	if err := validateParsedEnvelope(env, "capabilities"); err != nil {
		return bootstrapInfo{}, err
	}

	var data capabilitiesPayload
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return bootstrapInfo{}, fmt.Errorf("parse capabilities data: %w", err)
	}
	if data.ContractVersion != supportedContractVersion {
		return bootstrapInfo{}, fmt.Errorf("unsupported contract version: %s", data.ContractVersion)
	}
	capabilities := map[string]bool{}
	for _, capability := range data.Capabilities {
		capabilities[capability] = true
	}
	for _, required := range []string{"version.v1", "capabilities.v1"} {
		if !capabilities[required] {
			return bootstrapInfo{}, fmt.Errorf("missing bootstrap capability: %s", required)
		}
	}

	return bootstrapInfo{
		InfraLabVersion: data.InfraLabVersion,
		ContractVersion: data.ContractVersion,
		Capabilities:    capabilities,
	}, nil
}

func validateEnvelope(raw, command string) error {
	var env ilabEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return fmt.Errorf("parse %s envelope: %w", command, err)
	}
	return validateParsedEnvelope(env, command)
}

func validateParsedEnvelope(env ilabEnvelope, command string) error {
	if !env.OK {
		if len(env.Errors) > 0 {
			return fmt.Errorf("%s failed: %s", command, env.Errors[0].Message)
		}
		return fmt.Errorf("%s failed", command)
	}
	if env.Command != command {
		return fmt.Errorf("unexpected command %q, want %q", env.Command, command)
	}
	if env.ContractVersion != supportedContractVersion {
		return fmt.Errorf("unsupported contract version: %s", env.ContractVersion)
	}
	return nil
}
