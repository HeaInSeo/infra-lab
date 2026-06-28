package output

const (
	ExitDomain  = 1
	ExitUsage   = 2
	ExitRuntime = 3
	ExitTimeout = 124
)

type ContractError struct {
	Code    string
	Message string
	Field   string
	Exit    int
	Err     error
}

func NewError(code, message string, exitCode int) *ContractError {
	return &ContractError{
		Code:    code,
		Message: message,
		Exit:    exitCode,
	}
}

func WrapError(code, message string, exitCode int, err error) *ContractError {
	return &ContractError{
		Code:    code,
		Message: message,
		Exit:    exitCode,
		Err:     err,
	}
}

func (e *ContractError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func (e *ContractError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *ContractError) ExitCode() int {
	if e == nil || e.Exit == 0 {
		return ExitDomain
	}
	return e.Exit
}

func (e *ContractError) Info() ErrorInfo {
	if e == nil {
		return ErrorInfo{Code: "COMMAND_FAILED", Message: "command failed"}
	}
	message := e.Message
	if message == "" && e.Err != nil {
		message = e.Err.Error()
	}
	if message == "" {
		message = e.Code
	}
	return ErrorInfo{
		Code:    e.Code,
		Message: message,
		Field:   e.Field,
	}
}
