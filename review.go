package eidolon

import (
	"context"
	"fmt"
	"time"
)

// ReviewConfig configures the review window behavior.
type ReviewConfig struct {
	WindowDuration   time.Duration // total review window (default 48h)
	ExtensionPerItem time.Duration // auto-release extension per active item (default 24h)
	AutoRelease      bool          // whether to auto-release when window expires
}

// DefaultReviewConfig returns the standard review configuration: 48h window,
// 24h extension per active item, auto-release enabled.
func DefaultReviewConfig() ReviewConfig {
	return ReviewConfig{
		WindowDuration:   time.Duration(DefaultWindowHours) * time.Hour,
		ExtensionPerItem: time.Duration(DefaultExtensionHours) * time.Hour,
		AutoRelease:      true,
	}
}

// ReviewWindow represents an active review period for an entity.
type ReviewWindow struct {
	ID            string
	EntityID      string
	StartedAt     time.Time
	ExpiresAt     time.Time
	AutoReleaseAt time.Time
	Status        ReviewStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ReviewStatus describes the current state of a review window.
type ReviewStatus string

const (
	ReviewActive       ReviewStatus = "active"
	ReviewApproved     ReviewStatus = "approved"
	ReviewAutoReleased ReviewStatus = "auto_released"
	ReviewExpired      ReviewStatus = "expired"
)

// ReviewResult is returned by ReviewEngine.Check.
type ReviewResult struct {
	Window        *ReviewWindow
	TimeRemaining time.Duration
	IsExpired     bool
	HasWindow     bool
}

// ExpiredReview is returned by ProcessExpired for windows ready for auto-release.
type ExpiredReview struct {
	ReviewID string
	EntityID string
}

// ReviewEngine manages review windows for lifecycle entities.
type ReviewEngine struct {
	config ReviewConfig
	store  ReviewStore
}

// NewReviewEngine creates a ReviewEngine with the given config and store.
func NewReviewEngine(config ReviewConfig, store ReviewStore) *ReviewEngine {
	return &ReviewEngine{
		config: config,
		store:  store,
	}
}

// Start opens a review window for the entity. It is idempotent: if an active
// window already exists it is returned unchanged.
func (e *ReviewEngine) Start(ctx context.Context, entityID string) (*ReviewWindow, error) {
	existing, err := e.store.GetActiveReview(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("review.Start: check existing: %w", err)
	}
	if existing != nil {
		return existing, nil
	}

	now := time.Now().UTC()
	rw := ReviewWindow{
		ID:            newID(),
		EntityID:      entityID,
		StartedAt:     now,
		ExpiresAt:     now.Add(e.config.WindowDuration),
		AutoReleaseAt: now.Add(e.config.WindowDuration),
		Status:        ReviewActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := e.store.CreateReviewWindow(ctx, rw); err != nil {
		return nil, fmt.Errorf("review.Start: create window: %w", err)
	}
	return &rw, nil
}

// Approve marks the review window for the entity as approved.
func (e *ReviewEngine) Approve(ctx context.Context, entityID string) error {
	existing, err := e.store.GetActiveReview(ctx, entityID)
	if err != nil {
		return fmt.Errorf("review.Approve: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("review.Approve: %w", ErrNotFound)
	}
	return e.store.UpdateReviewStatus(ctx, entityID, ReviewApproved)
}

// Check returns the current review state for an entity.
func (e *ReviewEngine) Check(ctx context.Context, entityID string) (*ReviewResult, error) {
	rw, err := e.store.GetActiveReview(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("review.Check: %w", err)
	}
	if rw == nil {
		return &ReviewResult{HasWindow: false}, nil
	}

	now := time.Now().UTC()
	remaining := rw.AutoReleaseAt.Sub(now)
	if remaining < 0 {
		remaining = 0
	}

	return &ReviewResult{
		Window:        rw,
		TimeRemaining: remaining,
		IsExpired:     now.After(rw.AutoReleaseAt),
		HasWindow:     true,
	}, nil
}

// ProcessExpired finds all expired review windows. For each expired window it
// counts active items: if there are any, the auto-release time is extended by
// ExtensionPerItem * count and the window is kept. If there are no active items
// (or AutoRelease is disabled), the window is returned in the result slice for
// the caller to act on (e.g. transition the entity forward).
func (e *ReviewEngine) ProcessExpired(ctx context.Context) ([]ExpiredReview, error) {
	now := time.Now().UTC()
	expired, err := e.store.GetExpiredReviews(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("review.ProcessExpired: get expired: %w", err)
	}

	var ready []ExpiredReview
	for _, exp := range expired {
		count, err := e.store.CountActiveItems(ctx, exp.EntityID)
		if err != nil {
			return nil, fmt.Errorf("review.ProcessExpired: count items for %s: %w", exp.EntityID, err)
		}

		if count > 0 && e.config.ExtensionPerItem > 0 {
			// Extend the auto-release time.
			extension := e.config.ExtensionPerItem * time.Duration(count)
			newAutoRelease := now.Add(extension)
			if err := e.store.ExtendAutoRelease(ctx, exp.EntityID, newAutoRelease); err != nil {
				return nil, fmt.Errorf("review.ProcessExpired: extend %s: %w", exp.EntityID, err)
			}
			continue
		}

		if e.config.AutoRelease {
			if err := e.store.UpdateReviewStatus(ctx, exp.EntityID, ReviewAutoReleased); err != nil {
				return nil, fmt.Errorf("review.ProcessExpired: auto-release %s: %w", exp.EntityID, err)
			}
			ready = append(ready, exp)
		}
	}
	return ready, nil
}

// newID generates a simple unique ID. In production this would use UUID or
// similar; here we use a time-based string for zero external dependencies.
func newID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
