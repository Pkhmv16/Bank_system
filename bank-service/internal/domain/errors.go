package domain

import "errors"

var (
	ErrNotFound           = errors.New("resource not found")
	ErrConflict           = errors.New("resource conflict")
	ErrValidation         = errors.New("validation error")
	ErrNotEnoughFunds     = errors.New("not enough funds")
	ErrAccessDenied       = errors.New("access denied")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrExternalTimeout    = errors.New("external service timeout")
)

type Error struct {
	Kind    error
	Message string
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Kind }

func NewError(kind error, message string) error {
	return &Error{Kind: kind, Message: message}
}
