package eidolon

import "fmt"

// PipelineConfig defines a phase graph: the set of phases, allowed transitions,
// and the starting phase for new entities.
type PipelineConfig struct {
	// Phases is the ordered list of phases in the pipeline.
	// Index in this slice determines display order and kickback detection.
	Phases []Phase

	// Transitions maps (from phase name) -> (set of valid to phase names).
	// Use map[string]map[string]bool where the inner bool is always true.
	Transitions map[string]map[string]bool

	// InitialPhase is the phase name assigned to newly created entities.
	InitialPhase string
}

// Phase is a named stage in a lifecycle pipeline.
type Phase struct {
	Name        string
	Label       string
	Description string
}

// Pipeline is the runtime state machine engine built from a PipelineConfig.
// It is immutable after construction — create a new Pipeline to change config.
type Pipeline struct {
	config PipelineConfig
	index  map[string]int // phase name -> index in config.Phases
}

// NewPipeline validates the config and builds a ready-to-use Pipeline.
//
// Validation rules:
//   - At least 1 phase must be defined.
//   - InitialPhase must exist in the Phases list.
//   - All transition targets must be valid phase names.
//   - Self-transitions (from == to) are not allowed.
func NewPipeline(config PipelineConfig) (*Pipeline, error) {
	if len(config.Phases) == 0 {
		return nil, fmt.Errorf("eidolon: pipeline must have at least one phase")
	}

	// Build the index and validate for duplicate names.
	idx := make(map[string]int, len(config.Phases))
	for i, p := range config.Phases {
		if p.Name == "" {
			return nil, fmt.Errorf("eidolon: phase at index %d has an empty name", i)
		}
		if _, exists := idx[p.Name]; exists {
			return nil, fmt.Errorf("eidolon: duplicate phase name %q", p.Name)
		}
		idx[p.Name] = i
	}

	// Validate InitialPhase.
	if _, ok := idx[config.InitialPhase]; !ok {
		return nil, fmt.Errorf("eidolon: initial phase %q not found in phase list", config.InitialPhase)
	}

	// Validate all transitions.
	for from, targets := range config.Transitions {
		if _, ok := idx[from]; !ok {
			return nil, fmt.Errorf("eidolon: transition source phase %q is not a valid phase", from)
		}
		for to := range targets {
			if _, ok := idx[to]; !ok {
				return nil, fmt.Errorf("eidolon: transition target phase %q is not a valid phase (from %q)", to, from)
			}
			if from == to {
				return nil, fmt.Errorf("eidolon: self-transition not allowed for phase %q", from)
			}
		}
	}

	return &Pipeline{config: config, index: idx}, nil
}

// Validate checks whether the transition from current to next is legal
// according to the configured transition graph.
func (p *Pipeline) Validate(current, next string) error {
	if _, ok := p.index[current]; !ok {
		return &TransitionError{From: current, To: next,
			Reason: fmt.Sprintf("unknown phase %q", current)}
	}
	if _, ok := p.index[next]; !ok {
		return &TransitionError{From: current, To: next,
			Reason: fmt.Sprintf("unknown phase %q", next)}
	}
	targets, ok := p.config.Transitions[current]
	if !ok || !targets[next] {
		return &TransitionError{From: current, To: next,
			Reason: "transition not defined"}
	}
	return nil
}

// IsKickBack reports whether transitioning from current to next is a backward
// (kick-back) move — i.e. next appears earlier in the phase list than current.
// Returns false if either phase is unknown.
func (p *Pipeline) IsKickBack(current, next string) bool {
	ci, cok := p.index[current]
	ni, nok := p.index[next]
	if !cok || !nok {
		return false
	}
	return ni < ci
}

// Available returns the list of phase names that are valid transitions from
// the given current phase, in pipeline order.
func (p *Pipeline) Available(current string) []string {
	targets, ok := p.config.Transitions[current]
	if !ok {
		return nil
	}
	result := make([]string, 0, len(targets))
	// Iterate in pipeline order for stable output.
	for _, ph := range p.config.Phases {
		if targets[ph.Name] {
			result = append(result, ph.Name)
		}
	}
	return result
}

// ValidateReset checks that target is a legal backward reset from current.
// The target must be earlier in the pipeline (lower index) than current.
func (p *Pipeline) ValidateReset(current, target string) error {
	ci, cok := p.index[current]
	ti, tok := p.index[target]
	if !cok {
		return fmt.Errorf("%w: unknown current phase %q", ErrUnknownPhase, current)
	}
	if !tok {
		return fmt.Errorf("%w: unknown target phase %q", ErrUnknownPhase, target)
	}
	if ti >= ci {
		return fmt.Errorf("%w: reset target %q is not earlier than current phase %q",
			ErrInvalidTransition, target, current)
	}
	return nil
}

// PhasesAfter returns the names of all phases that come after the given phase
// in the pipeline, in order. Useful for cascade-delete operations.
func (p *Pipeline) PhasesAfter(phase string) []string {
	idx, ok := p.index[phase]
	if !ok {
		return nil
	}
	after := make([]string, 0, len(p.config.Phases)-idx-1)
	for i := idx + 1; i < len(p.config.Phases); i++ {
		after = append(after, p.config.Phases[i].Name)
	}
	return after
}

// Index returns the zero-based display order index of the phase.
// Returns -1 if the phase is unknown.
func (p *Pipeline) Index(phase string) int {
	if i, ok := p.index[phase]; ok {
		return i
	}
	return -1
}

// Initial returns the name of the initial phase.
func (p *Pipeline) Initial() string {
	return p.config.InitialPhase
}

// Phases returns the ordered phase list (a copy to prevent mutation).
func (p *Pipeline) Phases() []Phase {
	out := make([]Phase, len(p.config.Phases))
	copy(out, p.config.Phases)
	return out
}

// Config returns a copy of the pipeline configuration.
func (p *Pipeline) Config() PipelineConfig {
	// Deep-copy the transitions map.
	transitions := make(map[string]map[string]bool, len(p.config.Transitions))
	for from, targets := range p.config.Transitions {
		tc := make(map[string]bool, len(targets))
		for to, v := range targets {
			tc[to] = v
		}
		transitions[from] = tc
	}
	phases := make([]Phase, len(p.config.Phases))
	copy(phases, p.config.Phases)
	return PipelineConfig{
		Phases:       phases,
		Transitions:  transitions,
		InitialPhase: p.config.InitialPhase,
	}
}
