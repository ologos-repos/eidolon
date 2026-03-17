package eidolon

import (
	"errors"
	"testing"
)

// testPipeline builds a simple 3-phase pipeline for most tests:
//
//	draft -> review -> approved
//	review -> draft  (kickback)
func testPipeline(t *testing.T) *Pipeline {
	t.Helper()
	p, err := NewPipeline(PipelineConfig{
		Phases: []Phase{
			{Name: "draft", Label: "Draft", Description: "Work in progress"},
			{Name: "review", Label: "Review", Description: "Under review"},
			{Name: "approved", Label: "Approved", Description: "Fully approved"},
		},
		Transitions: map[string]map[string]bool{
			"draft":  {"review": true},
			"review": {"approved": true, "draft": true}, // draft is a kickback
		},
		InitialPhase: "draft",
	})
	if err != nil {
		t.Fatalf("testPipeline: %v", err)
	}
	return p
}

// ---------------------------------------------------------------------------
// NewPipeline validation
// ---------------------------------------------------------------------------

func TestNewPipeline_Valid(t *testing.T) {
	p := testPipeline(t)
	if p == nil {
		t.Fatal("expected non-nil pipeline")
	}
}

func TestNewPipeline_EmptyPhases(t *testing.T) {
	_, err := NewPipeline(PipelineConfig{
		Phases:       []Phase{},
		Transitions:  nil,
		InitialPhase: "",
	})
	if err == nil {
		t.Fatal("expected error for empty phases")
	}
}

func TestNewPipeline_UnknownInitialPhase(t *testing.T) {
	_, err := NewPipeline(PipelineConfig{
		Phases:       []Phase{{Name: "alpha"}},
		Transitions:  nil,
		InitialPhase: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for unknown initial phase")
	}
}

func TestNewPipeline_UnknownTransitionSource(t *testing.T) {
	_, err := NewPipeline(PipelineConfig{
		Phases: []Phase{{Name: "alpha"}},
		Transitions: map[string]map[string]bool{
			"ghost": {"alpha": true},
		},
		InitialPhase: "alpha",
	})
	if err == nil {
		t.Fatal("expected error for unknown transition source phase")
	}
}

func TestNewPipeline_UnknownTransitionTarget(t *testing.T) {
	_, err := NewPipeline(PipelineConfig{
		Phases: []Phase{{Name: "alpha"}},
		Transitions: map[string]map[string]bool{
			"alpha": {"ghost": true},
		},
		InitialPhase: "alpha",
	})
	if err == nil {
		t.Fatal("expected error for unknown transition target phase")
	}
}

func TestNewPipeline_SelfTransition(t *testing.T) {
	_, err := NewPipeline(PipelineConfig{
		Phases: []Phase{{Name: "alpha"}, {Name: "beta"}},
		Transitions: map[string]map[string]bool{
			"alpha": {"alpha": true}, // self-transition
		},
		InitialPhase: "alpha",
	})
	if err == nil {
		t.Fatal("expected error for self-transition")
	}
}

func TestNewPipeline_DuplicatePhaseNames(t *testing.T) {
	_, err := NewPipeline(PipelineConfig{
		Phases: []Phase{
			{Name: "alpha"},
			{Name: "alpha"}, // duplicate
		},
		Transitions:  nil,
		InitialPhase: "alpha",
	})
	if err == nil {
		t.Fatal("expected error for duplicate phase name")
	}
}

func TestNewPipeline_EmptyPhaseName(t *testing.T) {
	_, err := NewPipeline(PipelineConfig{
		Phases:       []Phase{{Name: ""}},
		Transitions:  nil,
		InitialPhase: "",
	})
	if err == nil {
		t.Fatal("expected error for empty phase name")
	}
}

// ---------------------------------------------------------------------------
// Validate (transition checking)
// ---------------------------------------------------------------------------

func TestValidate_ValidForwardTransition(t *testing.T) {
	p := testPipeline(t)
	if err := p.Validate("draft", "review"); err != nil {
		t.Errorf("expected valid transition, got: %v", err)
	}
}

func TestValidate_ValidKickback(t *testing.T) {
	p := testPipeline(t)
	if err := p.Validate("review", "draft"); err != nil {
		t.Errorf("expected valid kickback transition, got: %v", err)
	}
}

func TestValidate_InvalidTransition(t *testing.T) {
	p := testPipeline(t)
	err := p.Validate("draft", "approved") // no direct path
	if err == nil {
		t.Fatal("expected error for undefined transition")
	}
	var te *TransitionError
	if !errors.As(err, &te) {
		t.Errorf("expected *TransitionError, got %T", err)
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected errors.Is(err, ErrInvalidTransition)")
	}
	if te.From != "draft" || te.To != "approved" {
		t.Errorf("wrong From/To: %q -> %q", te.From, te.To)
	}
}

func TestValidate_UnknownFromPhase(t *testing.T) {
	p := testPipeline(t)
	err := p.Validate("ghost", "review")
	if err == nil {
		t.Fatal("expected error for unknown source phase")
	}
	var te *TransitionError
	if !errors.As(err, &te) {
		t.Errorf("expected *TransitionError, got %T", err)
	}
}

func TestValidate_UnknownToPhase(t *testing.T) {
	p := testPipeline(t)
	err := p.Validate("draft", "ghost")
	if err == nil {
		t.Fatal("expected error for unknown target phase")
	}
}

// ---------------------------------------------------------------------------
// IsKickBack
// ---------------------------------------------------------------------------

func TestIsKickBack_Forward(t *testing.T) {
	p := testPipeline(t)
	if p.IsKickBack("draft", "review") {
		t.Error("draft->review should NOT be a kickback")
	}
	if p.IsKickBack("review", "approved") {
		t.Error("review->approved should NOT be a kickback")
	}
}

func TestIsKickBack_Backward(t *testing.T) {
	p := testPipeline(t)
	if !p.IsKickBack("review", "draft") {
		t.Error("review->draft SHOULD be a kickback")
	}
	if !p.IsKickBack("approved", "draft") {
		t.Error("approved->draft SHOULD be a kickback")
	}
	if !p.IsKickBack("approved", "review") {
		t.Error("approved->review SHOULD be a kickback")
	}
}

func TestIsKickBack_UnknownPhase(t *testing.T) {
	p := testPipeline(t)
	if p.IsKickBack("ghost", "draft") {
		t.Error("unknown phase should return false, not true")
	}
}

// ---------------------------------------------------------------------------
// Available
// ---------------------------------------------------------------------------

func TestAvailable_FromDraft(t *testing.T) {
	p := testPipeline(t)
	avail := p.Available("draft")
	if len(avail) != 1 || avail[0] != "review" {
		t.Errorf("expected [review], got %v", avail)
	}
}

func TestAvailable_FromReview(t *testing.T) {
	p := testPipeline(t)
	avail := p.Available("review")
	// Should be in pipeline order: draft(0), approved(2)
	if len(avail) != 2 {
		t.Fatalf("expected 2 transitions from review, got %v", avail)
	}
	// draft comes before approved in pipeline order
	if avail[0] != "draft" || avail[1] != "approved" {
		t.Errorf("expected [draft approved] in order, got %v", avail)
	}
}

func TestAvailable_FromApproved(t *testing.T) {
	p := testPipeline(t)
	avail := p.Available("approved")
	if len(avail) != 0 {
		t.Errorf("expected no transitions from approved, got %v", avail)
	}
}

func TestAvailable_UnknownPhase(t *testing.T) {
	p := testPipeline(t)
	avail := p.Available("ghost")
	if avail != nil {
		t.Errorf("expected nil for unknown phase, got %v", avail)
	}
}

// ---------------------------------------------------------------------------
// ValidateReset
// ---------------------------------------------------------------------------

func TestValidateReset_ValidBackward(t *testing.T) {
	p := testPipeline(t)
	if err := p.ValidateReset("approved", "draft"); err != nil {
		t.Errorf("expected valid reset, got: %v", err)
	}
	if err := p.ValidateReset("approved", "review"); err != nil {
		t.Errorf("expected valid reset, got: %v", err)
	}
	if err := p.ValidateReset("review", "draft"); err != nil {
		t.Errorf("expected valid reset, got: %v", err)
	}
}

func TestValidateReset_SamePhase(t *testing.T) {
	p := testPipeline(t)
	err := p.ValidateReset("review", "review")
	if err == nil {
		t.Fatal("expected error when resetting to same phase")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestValidateReset_ForwardTarget(t *testing.T) {
	p := testPipeline(t)
	err := p.ValidateReset("draft", "approved") // approved is ahead of draft
	if err == nil {
		t.Fatal("expected error when reset target is ahead of current")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestValidateReset_UnknownCurrent(t *testing.T) {
	p := testPipeline(t)
	err := p.ValidateReset("ghost", "draft")
	if err == nil {
		t.Fatal("expected error for unknown current phase")
	}
	if !errors.Is(err, ErrUnknownPhase) {
		t.Errorf("expected ErrUnknownPhase, got %v", err)
	}
}

func TestValidateReset_UnknownTarget(t *testing.T) {
	p := testPipeline(t)
	err := p.ValidateReset("approved", "ghost")
	if err == nil {
		t.Fatal("expected error for unknown target phase")
	}
	if !errors.Is(err, ErrUnknownPhase) {
		t.Errorf("expected ErrUnknownPhase, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// PhasesAfter
// ---------------------------------------------------------------------------

func TestPhasesAfter_FromDraft(t *testing.T) {
	p := testPipeline(t)
	after := p.PhasesAfter("draft")
	if len(after) != 2 || after[0] != "review" || after[1] != "approved" {
		t.Errorf("expected [review approved], got %v", after)
	}
}

func TestPhasesAfter_FromReview(t *testing.T) {
	p := testPipeline(t)
	after := p.PhasesAfter("review")
	if len(after) != 1 || after[0] != "approved" {
		t.Errorf("expected [approved], got %v", after)
	}
}

func TestPhasesAfter_FromLast(t *testing.T) {
	p := testPipeline(t)
	after := p.PhasesAfter("approved")
	if len(after) != 0 {
		t.Errorf("expected empty slice from last phase, got %v", after)
	}
}

func TestPhasesAfter_UnknownPhase(t *testing.T) {
	p := testPipeline(t)
	after := p.PhasesAfter("ghost")
	if after != nil {
		t.Errorf("expected nil for unknown phase, got %v", after)
	}
}

// ---------------------------------------------------------------------------
// Index, Initial, Phases, Config
// ---------------------------------------------------------------------------

func TestIndex(t *testing.T) {
	p := testPipeline(t)
	tests := []struct {
		phase string
		want  int
	}{
		{"draft", 0},
		{"review", 1},
		{"approved", 2},
		{"ghost", -1},
	}
	for _, tt := range tests {
		if got := p.Index(tt.phase); got != tt.want {
			t.Errorf("Index(%q) = %d, want %d", tt.phase, got, tt.want)
		}
	}
}

func TestInitial(t *testing.T) {
	p := testPipeline(t)
	if got := p.Initial(); got != "draft" {
		t.Errorf("Initial() = %q, want %q", got, "draft")
	}
}

func TestPhases_ReturnsCopy(t *testing.T) {
	p := testPipeline(t)
	phases := p.Phases()
	if len(phases) != 3 {
		t.Fatalf("expected 3 phases, got %d", len(phases))
	}
	// Mutating the returned slice must not affect the pipeline.
	phases[0].Name = "mutated"
	if p.Phases()[0].Name != "draft" {
		t.Error("Phases() returned a reference, not a copy")
	}
}

func TestConfig_ReturnsCopy(t *testing.T) {
	p := testPipeline(t)
	cfg := p.Config()
	// Mutate the returned config.
	cfg.InitialPhase = "mutated"
	cfg.Transitions["draft"]["review"] = false
	// Original must be unchanged.
	if p.Config().InitialPhase != "draft" {
		t.Error("Config() returned a reference to InitialPhase, not a copy")
	}
	if !p.Config().Transitions["draft"]["review"] {
		t.Error("Config() returned a shallow copy of Transitions")
	}
}

// ---------------------------------------------------------------------------
// TransitionError / GateError unwrapping
// ---------------------------------------------------------------------------

func TestTransitionError_Unwrap(t *testing.T) {
	te := &TransitionError{From: "a", To: "b", Reason: "test"}
	if !errors.Is(te, ErrInvalidTransition) {
		t.Error("TransitionError should unwrap to ErrInvalidTransition")
	}
	if te.Error() == "" {
		t.Error("TransitionError.Error() should not be empty")
	}
}

func TestGateError_Unwrap(t *testing.T) {
	ge := &GateError{From: "a", To: "b", EntityID: "e1", Reason: "blocked"}
	if !errors.Is(ge, ErrGateFailed) {
		t.Error("GateError should unwrap to ErrGateFailed")
	}
	if ge.Error() == "" {
		t.Error("GateError.Error() should not be empty")
	}
}

func TestAccessError_Unwrap(t *testing.T) {
	ae := &AccessError{Role: "client", Phase: "draft", Side: SideOutput, Op: OpWrite, Reason: "denied"}
	if !errors.Is(ae, ErrAccessDenied) {
		t.Error("AccessError should unwrap to ErrAccessDenied")
	}
	if ae.Error() == "" {
		t.Error("AccessError.Error() should not be empty")
	}
}
