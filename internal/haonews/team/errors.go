package team

import (
	"fmt"
)

type ErrorCode string

const (
	ErrCodeNotFound     ErrorCode = "NOT_FOUND"
	ErrCodeEmptyID      ErrorCode = "EMPTY_ID"
	ErrCodeForbidden    ErrorCode = "FORBIDDEN"
	ErrCodeInvalidState ErrorCode = "INVALID_STATE"
	ErrCodeConflict     ErrorCode = "CONFLICT"
	ErrCodeNilStore     ErrorCode = "NIL_STORE"
	ErrCodeUnsupported  ErrorCode = "UNSUPPORTED"
)

type TeamError struct {
	Code    ErrorCode
	Context string
	Err     error
}

func (e *TeamError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil && e.Context != "" {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Context, e.Err)
	}
	if e.Err != nil {
		return fmt.Sprintf("[%s] %v", e.Code, e.Err)
	}
	if e.Context != "" {
		return fmt.Sprintf("[%s] %s", e.Code, e.Context)
	}
	return fmt.Sprintf("[%s]", e.Code)
}

func (e *TeamError) Unwrap() error { return e.Err }

func (e *TeamError) Is(target error) bool {
	other, ok := target.(*TeamError)
	if !ok {
		return false
	}
	return e.Code == other.Code
}

var (
	ErrNotFound     = &TeamError{Code: ErrCodeNotFound}
	ErrEmptyID      = &TeamError{Code: ErrCodeEmptyID}
	ErrForbidden    = &TeamError{Code: ErrCodeForbidden}
	ErrInvalidState = &TeamError{Code: ErrCodeInvalidState}
	ErrConflict     = &TeamError{Code: ErrCodeConflict}
	ErrNilStore     = &TeamError{Code: ErrCodeNilStore}
	ErrUnsupported  = &TeamError{Code: ErrCodeUnsupported}
)

func NewNilStoreError(subject string) error {
	return &TeamError{Code: ErrCodeNilStore, Context: subject}
}

func NewEmptyIDError(subject string) error {
	return &TeamError{Code: ErrCodeEmptyID, Context: subject}
}

func NewNotFoundError(subject string) error {
	return &TeamError{Code: ErrCodeNotFound, Context: subject}
}

func NewForbiddenError(action, agentID string) error {
	return &TeamError{Code: ErrCodeForbidden, Context: fmt.Sprintf("action=%s agent=%s", action, agentID)}
}

func NewInvalidTransitionError(from, to string) error {
	return &TeamError{Code: ErrCodeInvalidState, Context: fmt.Sprintf("from=%s to=%s", from, to)}
}

func NewUnsupportedError(subject string) error {
	return &TeamError{Code: ErrCodeUnsupported, Context: subject}
}
