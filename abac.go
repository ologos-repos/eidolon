package eidolon

// AccessRule defines when a role can perform an operation on an artifact side
// within a specific set of phases.
//
// Evaluation semantics (checked in order):
//  1. DeniedIn: if the current phase is in this list, access is denied.
//  2. AllowedIn: if non-empty and the current phase is NOT in this list, access is denied.
//  3. If AllowedIn is empty and DeniedIn didn't match, access is allowed.
//
// Operation and ArtifactSide types are defined in eidolon.go.
// AccessError is defined in errors.go.
type AccessRule struct {
	Role      string       // role name (domain-specific, e.g. "client", "operator", "admin")
	Side      ArtifactSide
	Operation Operation
	AllowedIn []string     // phases where this rule grants access (empty = all phases)
	DeniedIn  []string     // phases where this rule explicitly denies (checked first)
}

// AccessMatrix is a configurable set of access rules evaluated in order.
// It is default-deny: if no rule matches, access is denied.
type AccessMatrix struct {
	rules    []AccessRule
	pipeline *Pipeline
}

// NewAccessMatrix creates an empty AccessMatrix backed by the given pipeline.
// The pipeline is used for any future phase-index-based operations; for now
// rule evaluation uses string matching.
func NewAccessMatrix(pipeline *Pipeline) *AccessMatrix {
	return &AccessMatrix{
		pipeline: pipeline,
		rules:    nil,
	}
}

// AddRule appends a single access rule to the matrix.
func (m *AccessMatrix) AddRule(rule AccessRule) {
	m.rules = append(m.rules, rule)
}

// AddRules appends multiple access rules to the matrix.
func (m *AccessMatrix) AddRules(rules ...AccessRule) {
	m.rules = append(m.rules, rules...)
}

// Check returns nil if the operation is allowed, or an *AccessError if denied.
//
// Evaluation order for each matching rule (same role, side, and operation):
//  1. If phase is in rule.DeniedIn → deny immediately.
//  2. If rule.AllowedIn is non-empty and phase is not in it → deny.
//  3. Otherwise this rule allows access → return nil.
//
// If no rule matches at all → default-deny.
func (m *AccessMatrix) Check(role, phase string, side ArtifactSide, op Operation) error {
	matched := false
	for _, r := range m.rules {
		if r.Role != role || r.Side != side || r.Operation != op {
			continue
		}
		matched = true

		// 1. DeniedIn takes precedence.
		if containsString(r.DeniedIn, phase) {
			return &AccessError{
				Role:   role,
				Phase:  phase,
				Side:   side,
				Op:     op,
				Reason: "phase is in DeniedIn list",
			}
		}

		// 2. AllowedIn restricts to specific phases.
		if len(r.AllowedIn) > 0 && !containsString(r.AllowedIn, phase) {
			// This rule doesn't grant access in this phase, but another rule might.
			continue
		}

		// 3. Rule grants access.
		return nil
	}

	// Default-deny: either no rules matched at all, or every matching rule
	// had a non-empty AllowedIn that excluded this phase.
	reason := "no matching rule"
	if matched {
		reason = "no rule allows access in this phase"
	}
	return &AccessError{
		Role:   role,
		Phase:  phase,
		Side:   side,
		Op:     op,
		Reason: reason,
	}
}

// containsString returns true if s appears in the slice.
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
