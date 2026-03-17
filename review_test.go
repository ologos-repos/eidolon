package eidolon

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ─── In-memory mock ReviewStore ──────────────────────────────────────────────

type mockReviewStore struct {
	windows     map[string]*ReviewWindow // entityID -> active window
	activeItems map[string]int           // entityID -> count of active items
}

func newMockReviewStore() *mockReviewStore {
	return &mockReviewStore{
		windows:     make(map[string]*ReviewWindow),
		activeItems: make(map[string]int),
	}
}

func (s *mockReviewStore) CreateReviewWindow(_ context.Context, rw ReviewWindow) error {
	if _, exists := s.windows[rw.EntityID]; exists {
		return ErrConflict
	}
	copy := rw
	s.windows[rw.EntityID] = &copy
	return nil
}

func (s *mockReviewStore) GetActiveReview(_ context.Context, entityID string) (*ReviewWindow, error) {
	rw, ok := s.windows[entityID]
	if !ok || rw.Status != ReviewActive {
		return nil, nil
	}
	copy := *rw
	return &copy, nil
}

func (s *mockReviewStore) UpdateReviewStatus(_ context.Context, entityID string, status ReviewStatus) error {
	rw, ok := s.windows[entityID]
	if !ok {
		return ErrNotFound
	}
	rw.Status = status
	rw.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *mockReviewStore) ExtendAutoRelease(_ context.Context, entityID string, newAutoRelease time.Time) error {
	rw, ok := s.windows[entityID]
	if !ok {
		return ErrNotFound
	}
	rw.AutoReleaseAt = newAutoRelease
	rw.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *mockReviewStore) GetExpiredReviews(_ context.Context, now time.Time) ([]ExpiredReview, error) {
	var result []ExpiredReview
	for _, rw := range s.windows {
		if rw.Status == ReviewActive && !rw.AutoReleaseAt.After(now) {
			result = append(result, ExpiredReview{ReviewID: rw.ID, EntityID: rw.EntityID})
		}
	}
	return result, nil
}

func (s *mockReviewStore) CountActiveItems(_ context.Context, entityID string) (int, error) {
	return s.activeItems[entityID], nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func newTestEngine(store *mockReviewStore) *ReviewEngine {
	return NewReviewEngine(DefaultReviewConfig(), store)
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestReviewStart(t *testing.T) {
	store := newMockReviewStore()
	e := newTestEngine(store)
	ctx := context.Background()

	rw, err := e.Start(ctx, "entity-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if rw == nil {
		t.Fatal("Start returned nil window")
	}
	if rw.EntityID != "entity-1" {
		t.Errorf("EntityID = %q, want %q", rw.EntityID, "entity-1")
	}
	if rw.Status != ReviewActive {
		t.Errorf("Status = %q, want %q", rw.Status, ReviewActive)
	}
	if rw.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should not be zero")
	}
	if rw.AutoReleaseAt.IsZero() {
		t.Error("AutoReleaseAt should not be zero")
	}
	// Window should be ~48h from now.
	expectedExpiry := time.Now().Add(48 * time.Hour)
	diff := rw.ExpiresAt.Sub(expectedExpiry)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt not within 5s of 48h from now: diff=%v", diff)
	}
}

func TestReviewStartIdempotent(t *testing.T) {
	store := newMockReviewStore()
	e := newTestEngine(store)
	ctx := context.Background()

	rw1, err := e.Start(ctx, "entity-2")
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}

	rw2, err := e.Start(ctx, "entity-2")
	if err != nil {
		t.Fatalf("second Start: %v", err)
	}

	// Should return the existing window, not create a new one.
	if rw1.ID != rw2.ID {
		t.Errorf("second Start returned different window ID: %q vs %q", rw1.ID, rw2.ID)
	}
}

func TestReviewApprove(t *testing.T) {
	store := newMockReviewStore()
	e := newTestEngine(store)
	ctx := context.Background()

	_, err := e.Start(ctx, "entity-3")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := e.Approve(ctx, "entity-3"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// After approval the window is no longer active.
	rw, err := store.GetActiveReview(ctx, "entity-3")
	if err != nil {
		t.Fatalf("GetActiveReview after approve: %v", err)
	}
	if rw != nil {
		t.Errorf("expected no active window after approval, got status=%q", rw.Status)
	}
}

func TestReviewApproveNotFound(t *testing.T) {
	store := newMockReviewStore()
	e := newTestEngine(store)
	ctx := context.Background()

	err := e.Approve(ctx, "no-such-entity")
	if err == nil {
		t.Fatal("expected error approving non-existent review")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestReviewCheck(t *testing.T) {
	store := newMockReviewStore()
	e := newTestEngine(store)
	ctx := context.Background()

	// No window yet.
	result, err := e.Check(ctx, "entity-4")
	if err != nil {
		t.Fatalf("Check with no window: %v", err)
	}
	if result.HasWindow {
		t.Error("expected HasWindow=false before Start")
	}

	// Start a window.
	_, err = e.Start(ctx, "entity-4")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	result, err = e.Check(ctx, "entity-4")
	if err != nil {
		t.Fatalf("Check after Start: %v", err)
	}
	if !result.HasWindow {
		t.Error("expected HasWindow=true after Start")
	}
	if result.IsExpired {
		t.Error("freshly started window should not be expired")
	}
	// ~48h remaining.
	if result.TimeRemaining < 47*time.Hour || result.TimeRemaining > 49*time.Hour {
		t.Errorf("TimeRemaining = %v, want ~48h", result.TimeRemaining)
	}
	if result.Window == nil {
		t.Error("result.Window should not be nil")
	}
}

func TestReviewCheckExpired(t *testing.T) {
	store := newMockReviewStore()
	e := newTestEngine(store)
	ctx := context.Background()

	_, err := e.Start(ctx, "entity-5")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Manually set the AutoReleaseAt to the past.
	rw := store.windows["entity-5"]
	rw.AutoReleaseAt = time.Now().Add(-1 * time.Hour)

	result, err := e.Check(ctx, "entity-5")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.IsExpired {
		t.Error("expected IsExpired=true for past AutoReleaseAt")
	}
	if result.TimeRemaining != 0 {
		t.Errorf("TimeRemaining should be 0 when expired, got %v", result.TimeRemaining)
	}
}

func TestReviewProcessExpiredNoItems(t *testing.T) {
	store := newMockReviewStore()
	e := newTestEngine(store)
	ctx := context.Background()

	_, err := e.Start(ctx, "entity-6")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wind the clock: make the window expire.
	rw := store.windows["entity-6"]
	rw.AutoReleaseAt = time.Now().Add(-1 * time.Minute)

	// No active items — should auto-release immediately.
	released, err := e.ProcessExpired(ctx)
	if err != nil {
		t.Fatalf("ProcessExpired: %v", err)
	}
	if len(released) != 1 {
		t.Fatalf("expected 1 released, got %d", len(released))
	}
	if released[0].EntityID != "entity-6" {
		t.Errorf("EntityID = %q, want %q", released[0].EntityID, "entity-6")
	}

	// Window should now be auto-released.
	if rw.Status != ReviewAutoReleased {
		t.Errorf("Status = %q, want %q", rw.Status, ReviewAutoReleased)
	}
}

func TestReviewProcessExpiredWithItems(t *testing.T) {
	store := newMockReviewStore()
	e := newTestEngine(store)
	ctx := context.Background()

	_, err := e.Start(ctx, "entity-7")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wind the clock.
	rw := store.windows["entity-7"]
	rw.AutoReleaseAt = time.Now().Add(-1 * time.Minute)

	// Set 2 active items — should extend by 2 * 24h = 48h.
	store.activeItems["entity-7"] = 2

	released, err := e.ProcessExpired(ctx)
	if err != nil {
		t.Fatalf("ProcessExpired: %v", err)
	}
	// Should not be in the released list (extended instead).
	if len(released) != 0 {
		t.Errorf("expected 0 released (extended), got %d", len(released))
	}

	// AutoReleaseAt should be ~48h from now.
	expected := time.Now().Add(48 * time.Hour)
	diff := rw.AutoReleaseAt.Sub(expected)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("AutoReleaseAt not extended by 48h: diff=%v", diff)
	}
	// Status should still be active.
	if rw.Status != ReviewActive {
		t.Errorf("Status = %q, want %q", rw.Status, ReviewActive)
	}
}

func TestReviewProcessExpiredAutoReleaseDisabled(t *testing.T) {
	store := newMockReviewStore()
	cfg := ReviewConfig{
		WindowDuration:   48 * time.Hour,
		ExtensionPerItem: 0, // no extension
		AutoRelease:      false,
	}
	e := NewReviewEngine(cfg, store)
	ctx := context.Background()

	_, err := e.Start(ctx, "entity-8")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	rw := store.windows["entity-8"]
	rw.AutoReleaseAt = time.Now().Add(-1 * time.Minute)

	released, err := e.ProcessExpired(ctx)
	if err != nil {
		t.Fatalf("ProcessExpired: %v", err)
	}
	// AutoRelease=false — should not appear in released list and status unchanged.
	if len(released) != 0 {
		t.Errorf("expected 0 released when AutoRelease=false, got %d", len(released))
	}
	if rw.Status != ReviewActive {
		t.Errorf("Status should stay active when AutoRelease=false, got %q", rw.Status)
	}
}

func TestReviewProcessExpiredMultipleEntities(t *testing.T) {
	store := newMockReviewStore()
	e := newTestEngine(store)
	ctx := context.Background()

	// entity-A: expired, no items → released
	_, _ = e.Start(ctx, "entity-A")
	store.windows["entity-A"].AutoReleaseAt = time.Now().Add(-1 * time.Minute)

	// entity-B: expired, 1 item → extended
	_, _ = e.Start(ctx, "entity-B")
	store.windows["entity-B"].AutoReleaseAt = time.Now().Add(-1 * time.Minute)
	store.activeItems["entity-B"] = 1

	// entity-C: not expired yet → not touched
	_, _ = e.Start(ctx, "entity-C")
	// entity-C.AutoReleaseAt is ~48h in the future

	released, err := e.ProcessExpired(ctx)
	if err != nil {
		t.Fatalf("ProcessExpired: %v", err)
	}
	if len(released) != 1 {
		t.Fatalf("expected 1 released, got %d", len(released))
	}
	if released[0].EntityID != "entity-A" {
		t.Errorf("expected entity-A released, got %q", released[0].EntityID)
	}

	// entity-B status unchanged.
	if store.windows["entity-B"].Status != ReviewActive {
		t.Errorf("entity-B should still be active")
	}
	// entity-C status unchanged.
	if store.windows["entity-C"].Status != ReviewActive {
		t.Errorf("entity-C should still be active")
	}
}
