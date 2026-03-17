package eidolon

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// errBlockAll is a sentinel error returned by a gate that always fails.
var errBlockAll = errors.New("gate: blocked")

// blockGate is a Gate that always fails with errBlockAll.
var blockGate = GateFunc(func(_ context.Context, _ string, _, _ string) error {
	return errBlockAll
})

// passGate is a Gate that always passes.
var passGate = GateFunc(func(_ context.Context, _ string, _, _ string) error {
	return nil
})

// errGate builds a gate that fails with the given message.
func errGate(msg string) Gate {
	return GateFunc(func(_ context.Context, _ string, _, _ string) error {
		return errors.New(msg)
	})
}

// gateTestPipeline builds the standard 3-phase pipeline for gate tests.
func gateTestPipeline(t *testing.T) *Pipeline {
	t.Helper()
	return testPipeline(t)
}

// ---------------------------------------------------------------------------
// GateFunc
// ---------------------------------------------------------------------------

func TestGateFunc_Implements(t *testing.T) {
	var _ Gate = GateFunc(nil)
}

func TestGateFunc_Pass(t *testing.T) {
	g := GateFunc(func(ctx context.Context, id, from, to string) error { return nil })
	if err := g.Check(context.Background(), "e1", "draft", "review"); err != nil {
		t.Errorf("expected pass, got: %v", err)
	}
}

func TestGateFunc_Fail(t *testing.T) {
	g := GateFunc(func(ctx context.Context, id, from, to string) error {
		return fmt.Errorf("denied: %s->%s", from, to)
	})
	err := g.Check(context.Background(), "e1", "draft", "review")
	if err == nil {
		t.Fatal("expected error from failing gate")
	}
}

// ---------------------------------------------------------------------------
// GateRegistry — basic registration and enforcement
// ---------------------------------------------------------------------------

func TestGateRegistry_NoGates(t *testing.T) {
	p := gateTestPipeline(t)
	r := NewGateRegistry(p)
	// No gates registered — forward transition should pass.
	err := r.Enforce(context.Background(), "e1", "draft", "review")
	if err != nil {
		t.Errorf("expected pass with no gates, got: %v", err)
	}
}

func TestGateRegistry_ForwardGatePasses(t *testing.T) {
	p := gateTestPipeline(t)
	r := NewGateRegistry(p)
	r.Register("draft", "review", passGate)
	err := r.Enforce(context.Background(), "e1", "draft", "review")
	if err != nil {
		t.Errorf("expected pass, got: %v", err)
	}
}

func TestGateRegistry_ForwardGateBlocks(t *testing.T) {
	p := gateTestPipeline(t)
	r := NewGateRegistry(p)
	r.Register("draft", "review", blockGate)

	err := r.Enforce(context.Background(), "e1", "draft", "review")
	if err == nil {
		t.Fatal("expected gate to block forward transition")
	}
	var ge *GateError
	if !errors.As(err, &ge) {
		t.Errorf("expected *GateError, got %T", err)
	}
	if !errors.Is(err, ErrGateFailed) {
		t.Errorf("expected errors.Is ErrGateFailed")
	}
	if ge.EntityID != "e1" {
		t.Errorf("GateError.EntityID = %q, want %q", ge.EntityID, "e1")
	}
	if ge.From != "draft" || ge.To != "review" {
		t.Errorf("GateError.From/To = %q/%q", ge.From, ge.To)
	}
}

func TestGateRegistry_MultipleGates_FirstFails(t *testing.T) {
	p := gateTestPipeline(t)
	r := NewGateRegistry(p)
	r.Register("draft", "review", errGate("first"))
	r.Register("draft", "review", errGate("second"))

	err := r.Enforce(context.Background(), "e1", "draft", "review")
	if err == nil {
		t.Fatal("expected error")
	}
	var ge *GateError
	if !errors.As(err, &ge) {
		t.Errorf("expected *GateError, got %T", err)
	}
	// Reason should reflect the first gate's error.
	if ge.Reason == "" {
		t.Error("GateError.Reason should not be empty")
	}
}

func TestGateRegistry_MultipleGates_AllPass(t *testing.T) {
	p := gateTestPipeline(t)
	r := NewGateRegistry(p)
	r.Register("draft", "review", passGate)
	r.Register("draft", "review", passGate)

	err := r.Enforce(context.Background(), "e1", "draft", "review")
	if err != nil {
		t.Errorf("expected all-pass, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GateRegistry — kickback bypass
// ---------------------------------------------------------------------------

func TestGateRegistry_KickbackSkipsGates(t *testing.T) {
	p := gateTestPipeline(t)
	r := NewGateRegistry(p)
	// Register a blocking gate on the kickback transition (review -> draft).
	r.Register("review", "draft", blockGate)

	// The kickback should bypass the gate and succeed.
	err := r.Enforce(context.Background(), "e1", "review", "draft")
	if err != nil {
		t.Errorf("kickback should bypass gates, got: %v", err)
	}
}

func TestGateRegistry_ForwardNotKickback(t *testing.T) {
	p := gateTestPipeline(t)
	r := NewGateRegistry(p)
	// Gates on a forward transition are NOT skipped.
	r.Register("draft", "review", blockGate)

	err := r.Enforce(context.Background(), "e1", "draft", "review")
	if err == nil {
		t.Fatal("forward gate should NOT be skipped")
	}
}

func TestGateRegistry_UnregisteredTransition_Passes(t *testing.T) {
	p := gateTestPipeline(t)
	r := NewGateRegistry(p)
	// No gate registered for review->approved — should pass.
	err := r.Enforce(context.Background(), "e1", "review", "approved")
	if err != nil {
		t.Errorf("unregistered transition should pass, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// AllGates combinator
// ---------------------------------------------------------------------------

func TestAllGates_AllPass(t *testing.T) {
	g := AllGates(passGate, passGate, passGate)
	err := g.Check(context.Background(), "e1", "draft", "review")
	if err != nil {
		t.Errorf("expected all-pass, got: %v", err)
	}
}

func TestAllGates_FirstFails(t *testing.T) {
	g := AllGates(blockGate, passGate)
	err := g.Check(context.Background(), "e1", "draft", "review")
	if err == nil {
		t.Fatal("expected error when first gate fails")
	}
}

func TestAllGates_LastFails(t *testing.T) {
	g := AllGates(passGate, passGate, blockGate)
	err := g.Check(context.Background(), "e1", "draft", "review")
	if err == nil {
		t.Fatal("expected error when last gate fails")
	}
}

func TestAllGates_Empty(t *testing.T) {
	g := AllGates()
	err := g.Check(context.Background(), "e1", "draft", "review")
	if err != nil {
		t.Errorf("empty AllGates should pass, got: %v", err)
	}
}

func TestAllGates_ShortCircuit(t *testing.T) {
	// Verify only the first error is returned (short-circuit).
	called := false
	second := GateFunc(func(_ context.Context, _ string, _, _ string) error {
		called = true
		return errors.New("second gate")
	})
	g := AllGates(blockGate, second)
	_ = g.Check(context.Background(), "e1", "a", "b")
	if called {
		t.Error("AllGates should short-circuit after first failure")
	}
}

// ---------------------------------------------------------------------------
// AnyGate combinator
// ---------------------------------------------------------------------------

func TestAnyGate_AllFail(t *testing.T) {
	g := AnyGate(blockGate, blockGate)
	err := g.Check(context.Background(), "e1", "draft", "review")
	if err == nil {
		t.Fatal("expected error when all gates fail")
	}
}

func TestAnyGate_FirstPasses(t *testing.T) {
	g := AnyGate(passGate, blockGate)
	err := g.Check(context.Background(), "e1", "draft", "review")
	if err != nil {
		t.Errorf("expected pass when first gate passes, got: %v", err)
	}
}

func TestAnyGate_LastPasses(t *testing.T) {
	g := AnyGate(blockGate, blockGate, passGate)
	err := g.Check(context.Background(), "e1", "draft", "review")
	if err != nil {
		t.Errorf("expected pass when last gate passes, got: %v", err)
	}
}

func TestAnyGate_Empty(t *testing.T) {
	g := AnyGate()
	err := g.Check(context.Background(), "e1", "draft", "review")
	if err != nil {
		t.Errorf("empty AnyGate should pass vacuously, got: %v", err)
	}
}

func TestAnyGate_ReturnsLastError(t *testing.T) {
	g := AnyGate(errGate("first"), errGate("last"))
	err := g.Check(context.Background(), "e1", "draft", "review")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "last" {
		t.Errorf("expected last error message, got: %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// SkipOnKickBack combinator
// ---------------------------------------------------------------------------

func TestSkipOnKickBack_ForwardEnforced(t *testing.T) {
	p := gateTestPipeline(t)
	g := SkipOnKickBack(p, blockGate)

	// Forward transition: gate IS enforced.
	err := g.Check(context.Background(), "e1", "draft", "review")
	if err == nil {
		t.Fatal("expected gate to be enforced on forward transition")
	}
}

func TestSkipOnKickBack_KickbackSkipped(t *testing.T) {
	p := gateTestPipeline(t)
	g := SkipOnKickBack(p, blockGate)

	// Kickback transition: gate is SKIPPED.
	err := g.Check(context.Background(), "e1", "review", "draft")
	if err != nil {
		t.Errorf("expected gate skip on kickback, got: %v", err)
	}
}

func TestSkipOnKickBack_ForwardPassGate(t *testing.T) {
	p := gateTestPipeline(t)
	g := SkipOnKickBack(p, passGate)

	err := g.Check(context.Background(), "e1", "draft", "review")
	if err != nil {
		t.Errorf("expected pass through on forward, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Context propagation
// ---------------------------------------------------------------------------

func TestGateRegistry_ContextCancellation(t *testing.T) {
	p := gateTestPipeline(t)
	r := NewGateRegistry(p)

	// Gate that checks for cancelled context.
	r.Register("draft", "review", GateFunc(func(ctx context.Context, _ string, _, _ string) error {
		return ctx.Err()
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before enforcement

	err := r.Enforce(ctx, "e1", "draft", "review")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	var ge *GateError
	if !errors.As(err, &ge) {
		t.Errorf("expected *GateError wrapping ctx error, got %T", err)
	}
}
