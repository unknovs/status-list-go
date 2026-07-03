package errors

import "errors"

// Sentinel errors for internal operations.
// These errors can be checked using errors.Is() for type-safe error handling.
var (
	// ErrNotFound indicates that the requested resource was not found
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists indicates that the resource already exists
	ErrAlreadyExists = errors.New("already exists")

	// ErrVersionMismatch indicates a concurrent modification conflict
	ErrVersionMismatch = errors.New("version mismatch")
)
