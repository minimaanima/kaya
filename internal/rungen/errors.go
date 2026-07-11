package rungen

import "errors"

var (
	ErrInvalidDefinition  = errors.New("invalid run definition")
	ErrUnsupportedVersion = errors.New("unsupported generator version")
	ErrNoPlayableRun      = errors.New("no playable run")
	ErrValidationLimit    = errors.New("validation state limit reached")
)
