package eidolon

import "context"

// Gate is a condition that must be satisfied before a forward phase transition.
// Gates are skipped for kickback (backward) transitions.
type Gate interface {
	Check(ctx context.Context, entityID string, from, to string) error
}

// GateFunc adapts a plain function to the Gate interface.
type GateFunc func(ctx context.Context, entityID string, from, to string) error

// Check implements Gate by calling the underlying function.
func (f GateFunc) Check(ctx context.Context, entityID string, from, to string) error {
	return f(ctx, entityID, from, to)
}

// GateRegistry maps (from, to) transitions to ordered slices of Gate checks.
// All registered gates for a transition must pass for the transition to proceed.
type GateRegistry struct {
	pipeline *Pipeline
	gates    map[string]map[string][]Gate // from -> to -> []Gate
}

// NewGateRegistry creates a GateRegistry backed by the given Pipeline.
func NewGateRegistry(pipeline *Pipeline) *GateRegistry {
	return &GateRegistry{
		pipeline: pipeline,
		gates:    make(map[string]map[string][]Gate),
	}
}

// Register adds a gate to the (from, to) transition.
// Multiple gates can be registered for the same transition; they are run in
// registration order and the first error stops execution.
func (r *GateRegistry) Register(from, to string, gate Gate) {
	if r.gates[from] == nil {
		r.gates[from] = make(map[string][]Gate)
	}
	r.gates[from][to] = append(r.gates[from][to], gate)
}

// Enforce runs all registered gates for the (from, to) transition.
//
// Kickback (backward) transitions bypass all gates.
// For forward transitions, each registered gate is checked in order;
// the first gate error is wrapped in a GateError and returned.
func (r *GateRegistry) Enforce(ctx context.Context, entityID, from, to string) error {
	// Kickbacks skip gates entirely.
	if r.pipeline.IsKickBack(from, to) {
		return nil
	}

	targets, ok := r.gates[from]
	if !ok {
		return nil
	}
	gateList, ok := targets[to]
	if !ok {
		return nil
	}

	for _, g := range gateList {
		if err := g.Check(ctx, entityID, from, to); err != nil {
			return &GateError{
				From:     from,
				To:       to,
				EntityID: entityID,
				Reason:   err.Error(),
			}
		}
	}
	return nil
}

// AllGates returns a Gate that passes only when ALL of the given gates pass.
// Gates are checked in order; the first failure is returned immediately.
func AllGates(gates ...Gate) Gate {
	return GateFunc(func(ctx context.Context, entityID string, from, to string) error {
		for _, g := range gates {
			if err := g.Check(ctx, entityID, from, to); err != nil {
				return err
			}
		}
		return nil
	})
}

// AnyGate returns a Gate that passes when AT LEAST ONE of the given gates passes.
// If all gates fail, the last error is returned.
// If no gates are provided, it returns nil (passes vacuously).
func AnyGate(gates ...Gate) Gate {
	return GateFunc(func(ctx context.Context, entityID string, from, to string) error {
		if len(gates) == 0 {
			return nil
		}
		var lastErr error
		for _, g := range gates {
			if err := g.Check(ctx, entityID, from, to); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}
		return lastErr
	})
}

// SkipOnKickBack wraps a gate and skips it when the transition is a kickback.
// This is useful when registering individual gates that should not be enforced
// on backward transitions, even if the GateRegistry's kickback bypass is
// overridden by a custom enforcement loop.
func SkipOnKickBack(pipeline *Pipeline, gate Gate) Gate {
	return GateFunc(func(ctx context.Context, entityID string, from, to string) error {
		if pipeline.IsKickBack(from, to) {
			return nil
		}
		return gate.Check(ctx, entityID, from, to)
	})
}
