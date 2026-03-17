package eidolon

import (
	"errors"
	"testing"
)

// testPipeline builds the 7-phase CrewPort-like pipeline used across ABAC tests.
func testABACPipeline(t *testing.T) *Pipeline {
	t.Helper()
	cfg := PipelineConfig{
		InitialPhase: "scoping",
		Phases: []Phase{
			{Name: "scoping", Label: "Scoping"},
			{Name: "working", Label: "Working"},
			{Name: "review", Label: "Review"},
			{Name: "revision", Label: "Revision"},
			{Name: "delivery", Label: "Delivery"},
			{Name: "dispute", Label: "Dispute"},
			{Name: "complete", Label: "Complete"},
		},
		Transitions: map[string]map[string]bool{
			"scoping":  {"working": true},
			"working":  {"review": true},
			"review":   {"revision": true, "delivery": true},
			"revision": {"working": true, "review": true},
			"delivery": {"dispute": true, "complete": true},
			"dispute":  {"revision": true, "complete": true},
		},
	}
	p, err := NewPipeline(cfg)
	if err != nil {
		t.Fatalf("testABACPipeline: %v", err)
	}
	return p
}

// crewPortRules returns an access matrix modelled on CrewPort's contracts domain:
//   - admin: full access everywhere
//   - operator: read+write output in working/revision phases; read input always
//   - client: read only; denied write/delete everywhere; can read output only after review
func crewPortMatrix(t *testing.T) *AccessMatrix {
	t.Helper()
	p := testABACPipeline(t)
	m := NewAccessMatrix(p)

	// Admin: read/write/delete/presign/list on both sides everywhere.
	for _, side := range []ArtifactSide{SideInput, SideOutput} {
		for _, op := range []Operation{OpRead, OpWrite, OpDelete, OpPresign, OpList} {
			m.AddRule(AccessRule{Role: "admin", Side: side, Operation: op})
		}
	}

	// Operator: read input always.
	m.AddRule(AccessRule{Role: "operator", Side: SideInput, Operation: OpRead})
	m.AddRule(AccessRule{Role: "operator", Side: SideInput, Operation: OpList})

	// Operator: write/presign output only during working and revision.
	m.AddRule(AccessRule{
		Role:      "operator",
		Side:      SideOutput,
		Operation: OpWrite,
		AllowedIn: []string{"working", "revision"},
	})
	m.AddRule(AccessRule{
		Role:      "operator",
		Side:      SideOutput,
		Operation: OpPresign,
		AllowedIn: []string{"working", "revision"},
	})
	// Operator: read+list output always.
	m.AddRule(AccessRule{Role: "operator", Side: SideOutput, Operation: OpRead})
	m.AddRule(AccessRule{Role: "operator", Side: SideOutput, Operation: OpList})

	// Client: read+list input always.
	m.AddRule(AccessRule{Role: "client", Side: SideInput, Operation: OpRead})
	m.AddRule(AccessRule{Role: "client", Side: SideInput, Operation: OpList})

	// Client: read+list output — allowed only from review onward; denied during scoping/working.
	m.AddRule(AccessRule{
		Role:      "client",
		Side:      SideOutput,
		Operation: OpRead,
		AllowedIn: []string{"review", "revision", "delivery", "dispute", "complete"},
	})
	m.AddRule(AccessRule{
		Role:      "client",
		Side:      SideOutput,
		Operation: OpList,
		AllowedIn: []string{"review", "revision", "delivery", "dispute", "complete"},
	})

	// Client: explicitly deny write/delete/presign on both sides everywhere.
	for _, side := range []ArtifactSide{SideInput, SideOutput} {
		for _, op := range []Operation{OpWrite, OpDelete, OpPresign} {
			m.AddRule(AccessRule{
				Role:      "client",
				Side:      side,
				Operation: op,
				DeniedIn:  []string{"scoping", "working", "review", "revision", "delivery", "dispute", "complete"},
			})
		}
	}

	return m
}

// ─── Admin tests ────────────────────────────────────────────────────────────

func TestABACAdminFullAccess(t *testing.T) {
	m := crewPortMatrix(t)

	phases := []string{"scoping", "working", "review", "delivery", "complete"}
	sides := []ArtifactSide{SideInput, SideOutput}
	ops := []Operation{OpRead, OpWrite, OpDelete, OpPresign, OpList}

	for _, phase := range phases {
		for _, side := range sides {
			for _, op := range ops {
				if err := m.Check("admin", phase, side, op); err != nil {
					t.Errorf("admin should have full access: phase=%s side=%s op=%s: %v",
						phase, side, op, err)
				}
			}
		}
	}
}

// ─── Operator tests ──────────────────────────────────────────────────────────

func TestABACOperatorReadInputAlways(t *testing.T) {
	m := crewPortMatrix(t)
	phases := []string{"scoping", "working", "review", "revision", "delivery", "dispute", "complete"}

	for _, phase := range phases {
		if err := m.Check("operator", phase, SideInput, OpRead); err != nil {
			t.Errorf("operator should always read input, phase=%s: %v", phase, err)
		}
	}
}

func TestABACOperatorWriteOutputOnlyInWorkingRevision(t *testing.T) {
	m := crewPortMatrix(t)

	allowed := []string{"working", "revision"}
	for _, phase := range allowed {
		if err := m.Check("operator", phase, SideOutput, OpWrite); err != nil {
			t.Errorf("operator should write output in %s: %v", phase, err)
		}
	}

	denied := []string{"scoping", "review", "delivery", "dispute", "complete"}
	for _, phase := range denied {
		err := m.Check("operator", phase, SideOutput, OpWrite)
		if err == nil {
			t.Errorf("operator should NOT write output in phase %s", phase)
		}
		var ae *AccessError
		if !errors.As(err, &ae) {
			t.Errorf("expected *AccessError for operator write output in %s, got %T", phase, err)
		}
		if !errors.Is(err, ErrAccessDenied) {
			t.Errorf("expected ErrAccessDenied for operator write output in %s", phase)
		}
	}
}

func TestABACOperatorReadOutputAlways(t *testing.T) {
	m := crewPortMatrix(t)
	phases := []string{"scoping", "working", "review", "delivery", "complete"}
	for _, phase := range phases {
		if err := m.Check("operator", phase, SideOutput, OpRead); err != nil {
			t.Errorf("operator should always read output, phase=%s: %v", phase, err)
		}
	}
}

// ─── Client tests ────────────────────────────────────────────────────────────

func TestABACClientReadInputAlways(t *testing.T) {
	m := crewPortMatrix(t)
	phases := []string{"scoping", "working", "review", "revision", "complete"}
	for _, phase := range phases {
		if err := m.Check("client", phase, SideInput, OpRead); err != nil {
			t.Errorf("client should always read input, phase=%s: %v", phase, err)
		}
	}
}

func TestABACClientReadOutputOnlyAfterReview(t *testing.T) {
	m := crewPortMatrix(t)

	allowed := []string{"review", "revision", "delivery", "dispute", "complete"}
	for _, phase := range allowed {
		if err := m.Check("client", phase, SideOutput, OpRead); err != nil {
			t.Errorf("client should read output in %s: %v", phase, err)
		}
	}

	denied := []string{"scoping", "working"}
	for _, phase := range denied {
		if err := m.Check("client", phase, SideOutput, OpRead); err == nil {
			t.Errorf("client should NOT read output in phase %s", phase)
		}
	}
}

func TestABACClientCannotWriteAnywhere(t *testing.T) {
	m := crewPortMatrix(t)
	phases := []string{"scoping", "working", "review", "revision", "delivery", "dispute", "complete"}
	sides := []ArtifactSide{SideInput, SideOutput}

	for _, phase := range phases {
		for _, side := range sides {
			err := m.Check("client", phase, side, OpWrite)
			if err == nil {
				t.Errorf("client should never write: phase=%s side=%s", phase, side)
			}
		}
	}
}

func TestABACClientCannotDeleteAnywhere(t *testing.T) {
	m := crewPortMatrix(t)
	phases := []string{"scoping", "working", "review", "delivery", "complete"}

	for _, phase := range phases {
		err := m.Check("client", phase, SideOutput, OpDelete)
		if err == nil {
			t.Errorf("client should never delete: phase=%s", phase)
		}
	}
}

// ─── DeniedIn precedence test ────────────────────────────────────────────────

func TestABACDeniedInTakesPrecedenceOverAllowedIn(t *testing.T) {
	p := testABACPipeline(t)
	m := NewAccessMatrix(p)

	// Rule grants access in "working" and "review", but also explicitly denies
	// in "review" — the DeniedIn should win.
	m.AddRule(AccessRule{
		Role:      "tester",
		Side:      SideOutput,
		Operation: OpRead,
		AllowedIn: []string{"working", "review"},
		DeniedIn:  []string{"review"},
	})

	// "working" should be allowed (AllowedIn match, not in DeniedIn).
	if err := m.Check("tester", "working", SideOutput, OpRead); err != nil {
		t.Errorf("expected allowed in working: %v", err)
	}

	// "review" should be denied (DeniedIn takes precedence over AllowedIn).
	if err := m.Check("tester", "review", SideOutput, OpRead); err == nil {
		t.Error("expected denied in review (DeniedIn should win)")
	}
}

// ─── Default-deny test ───────────────────────────────────────────────────────

func TestABACDefaultDenyNoRules(t *testing.T) {
	p := testABACPipeline(t)
	m := NewAccessMatrix(p)

	err := m.Check("unknown_role", "working", SideOutput, OpRead)
	if err == nil {
		t.Fatal("expected default-deny for unknown role")
	}
	if !errors.Is(err, ErrAccessDenied) {
		t.Errorf("expected ErrAccessDenied, got: %v", err)
	}
	var ae *AccessError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AccessError, got %T", err)
	}
	if ae.Role != "unknown_role" {
		t.Errorf("AccessError.Role = %q, want %q", ae.Role, "unknown_role")
	}
}

func TestABACDefaultDenyNoMatchingOp(t *testing.T) {
	p := testABACPipeline(t)
	m := NewAccessMatrix(p)
	// Only grant read, not write.
	m.AddRule(AccessRule{Role: "viewer", Side: SideOutput, Operation: OpRead})

	if err := m.Check("viewer", "working", SideOutput, OpRead); err != nil {
		t.Errorf("expected read allowed: %v", err)
	}
	if err := m.Check("viewer", "working", SideOutput, OpWrite); err == nil {
		t.Error("expected write denied (no matching rule)")
	}
}

// ─── AccessError fields ──────────────────────────────────────────────────────

func TestABACAccessErrorFields(t *testing.T) {
	p := testABACPipeline(t)
	m := NewAccessMatrix(p)
	// No rules — anything is denied.

	err := m.Check("client", "working", SideOutput, OpDelete)
	var ae *AccessError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AccessError, got %T", err)
	}
	if ae.Role != "client" {
		t.Errorf("Role = %q, want %q", ae.Role, "client")
	}
	if ae.Phase != "working" {
		t.Errorf("Phase = %q, want %q", ae.Phase, "working")
	}
	if ae.Side != SideOutput {
		t.Errorf("Side = %q, want %q", ae.Side, SideOutput)
	}
	if ae.Op != OpDelete {
		t.Errorf("Op = %q, want %q", ae.Op, OpDelete)
	}
}

// ─── Multiple rules — first allow wins ───────────────────────────────────────

func TestABACMultipleRulesFirstAllowWins(t *testing.T) {
	p := testABACPipeline(t)
	m := NewAccessMatrix(p)

	// Two rules for the same (role, side, op):
	//   Rule 1: allowed only in "review"
	//   Rule 2: allowed everywhere (no AllowedIn restriction)
	// The second rule should allow access in "working" even though rule 1 doesn't.
	m.AddRule(AccessRule{
		Role: "multi", Side: SideOutput, Operation: OpRead,
		AllowedIn: []string{"review"},
	})
	m.AddRule(AccessRule{
		Role: "multi", Side: SideOutput, Operation: OpRead,
		// empty AllowedIn = always allowed
	})

	if err := m.Check("multi", "working", SideOutput, OpRead); err != nil {
		t.Errorf("expected allowed in working via second rule: %v", err)
	}
	if err := m.Check("multi", "review", SideOutput, OpRead); err != nil {
		t.Errorf("expected allowed in review: %v", err)
	}
}
