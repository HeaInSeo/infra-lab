package server

import (
	"bytes"
	"encoding/json"
)

type successEnvelope struct {
	OK              bool   `json:"ok"`
	Command         string `json:"command"`
	ContractVersion string `json:"contractVersion"`
	Data            any    `json:"data"`
	Warnings        []any  `json:"warnings"`
	Errors          []any  `json:"errors"`
}

func encodeSuccessEnvelope(command string, data any) (string, error) {
	env := successEnvelope{
		OK:              true,
		Command:         command,
		ContractVersion: supportedContractVersion,
		Data:            data,
		Warnings:        []any{},
		Errors:          []any{},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		return "", err
	}
	return buf.String(), nil
}
