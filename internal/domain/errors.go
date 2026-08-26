package domain

import "fmt"

type ErrorCode string

const (
	CodeInvalid     ErrorCode = "invalid"
	CodeConflict    ErrorCode = "conflict"
	CodeGate        ErrorCode = "gate_failed"
	CodeNotFound    ErrorCode = "not_found"
	CodeIdempotency ErrorCode = "idempotency_conflict"
	CodeFrozen      ErrorCode = "frozen"
	CodeIntegrity   ErrorCode = "integrity_failed"
)

type BusinessError struct {
	Code    ErrorCode         `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *BusinessError) Error() string { return e.Message }
func Invalid(message string, fields map[string]string) error {
	return &BusinessError{Code: CodeInvalid, Message: message, Fields: fields}
}
func Gate(message string) error { return &BusinessError{Code: CodeGate, Message: message} }
func Frozen() error {
	return &BusinessError{Code: CodeFrozen, Message: "个案已归档，业务事实不可变更"}
}
func Integrity(message string) error {
	return &BusinessError{Code: CodeIntegrity, Message: message}
}
func Conflict(expected, actual int64) error {
	return &BusinessError{Code: CodeConflict, Message: fmt.Sprintf("修订号冲突：期望 %d，当前 %d", expected, actual)}
}
func CodeOf(err error) ErrorCode {
	if e, ok := err.(*BusinessError); ok {
		return e.Code
	}
	return "internal"
}
