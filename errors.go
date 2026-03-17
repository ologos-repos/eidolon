package eidolon

import (
	"errors"
	"fmt"
)

// Sentinel errors — use errors.Is() to check for these.
var (
	ErrInvalidTransition = errors.New("eidolon: invalid phase transition")
	ErrUnknownPhase      = errors.New("eidolon: unknown phase")
	ErrNoActivePhase     = errors.New("eidolon: no active phase")
	ErrGateFailed        = errors.New("eidolon: gate check failed")
	ErrAccessDenied      = errors.New("eidolon: access denied")
	ErrNotFound          = errors.New("eidolon: not found")
	ErrConflict          = errors.New("eidolon: conflict")
)

// TransitionError wraps ErrInvalidTransition with from/to phase context.
type TransitionError struct {
	From   string
	To     string
	Reason string
}

func (e *TransitionError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("eidolon: invalid transition %q -> %q: %s", e.From, e.To, e.Reason)
	}
	return fmt.Sprintf("eidolon: invalid transition %q -> %q", e.From, e.To)
}

func (e *TransitionError) Unwrap() error {
	return ErrInvalidTransition
}

// GateError wraps ErrGateFailed with transition and entity context.
type GateError struct {
	From     string
	To       string
	EntityID string
	Reason   string
}

func (e *GateError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("eidolon: gate failed for entity %q on transition %q -> %q: %s",
			e.EntityID, e.From, e.To, e.Reason)
	}
	return fmt.Sprintf("eidolon: gate failed for entity %q on transition %q -> %q",
		e.EntityID, e.From, e.To)
}

func (e *GateError) Unwrap() error {
	return ErrGateFailed
}

// AccessError wraps ErrAccessDenied with ABAC context.
type AccessError struct {
	Role   string
	Phase  string
	Side   ArtifactSide
	Op     Operation
	Reason string
}

func (e *AccessError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("eidolon: access denied for role %q in phase %q (%s %s): %s",
			e.Role, e.Phase, e.Op, e.Side, e.Reason)
	}
	return fmt.Sprintf("eidolon: access denied for role %q in phase %q (%s %s)",
		e.Role, e.Phase, e.Op, e.Side)
}

func (e *AccessError) Unwrap() error {
	return ErrAccessDenied
}
