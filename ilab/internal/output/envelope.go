package output

const ContractVersion = "infra-lab.contract/v1"

// Envelope is the stable machine-readable response shape for ilab --json.
type Envelope struct {
	OK              bool        `json:"ok"`
	Command         string      `json:"command"`
	ContractVersion string      `json:"contractVersion"`
	Data            any         `json:"data"`
	Warnings        []Warning   `json:"warnings"`
	Errors          []ErrorInfo `json:"errors"`
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func Success(command string, data any, warnings ...Warning) Envelope {
	return normalize(Envelope{
		OK:              true,
		Command:         command,
		ContractVersion: ContractVersion,
		Data:            data,
		Warnings:        warnings,
	})
}

func Failure(command string, errors []ErrorInfo, warnings ...Warning) Envelope {
	return normalize(Envelope{
		OK:              false,
		Command:         command,
		ContractVersion: ContractVersion,
		Data:            nil,
		Warnings:        warnings,
		Errors:          errors,
	})
}

func normalize(env Envelope) Envelope {
	if env.ContractVersion == "" {
		env.ContractVersion = ContractVersion
	}
	if env.Warnings == nil {
		env.Warnings = []Warning{}
	}
	if env.Errors == nil {
		env.Errors = []ErrorInfo{}
	}
	return env
}
