package domain

import "errors"

var (
	ErrConfigMissing  = errors.New("config missing")
	ErrChangeNotFound = errors.New("change not found")
	ErrUnreachable    = errors.New("orchestrator unreachable")
	ErrInvalidYAML    = errors.New("invalid yaml")
	ErrNotARepo       = errors.New("not a git repository")
	ErrInvalidURL     = errors.New("invalid url")
)
