package eidolon

import (
	"context"
	"time"
)

// PhaseStore manages phase state persistence and transition history for entities.
type PhaseStore interface {
	// GetPhase returns the current phase for the entity.
	GetPhase(ctx context.Context, entityID string) (string, error)

	// SetPhase unconditionally sets the entity's current phase.
	SetPhase(ctx context.Context, entityID, phase string) error

	// SetPhaseConditional atomically sets the phase only when the current
	// phase matches expectedPhase. Returns true if the update was applied,
	// false if the current phase didn't match (no-op, not an error).
	SetPhaseConditional(ctx context.Context, entityID, expectedPhase, newPhase string) (bool, error)

	// RecordTransition appends an immutable transition record.
	RecordTransition(ctx context.Context, record TransitionRecord) error

	// GetTransitionHistory returns all transition records for the entity in
	// creation order (oldest first).
	GetTransitionHistory(ctx context.Context, entityID string) ([]TransitionRecord, error)
}

// RequirementStore manages requirements and plan items.
type RequirementStore interface {
	// CreateRequirement persists a new requirement.
	CreateRequirement(ctx context.Context, req Requirement) error

	// GetRequirements returns requirements for the entity. If currentOnly is
	// true, only the current (non-superseded) versions are returned.
	GetRequirements(ctx context.Context, entityID string, currentOnly bool) ([]Requirement, error)

	// CountRequirements returns the number of current requirements for the entity.
	CountRequirements(ctx context.Context, entityID string) (int, error)

	// GetUnmappedRequirements returns current requirements that have no
	// RequirementMapping entries linking them to plan items.
	GetUnmappedRequirements(ctx context.Context, entityID string) ([]Requirement, error)

	// CreatePlanItem persists a new plan item.
	CreatePlanItem(ctx context.Context, item PlanItem) error

	// GetPlanItems returns all current plan items for the entity, ordered by Sequence.
	GetPlanItems(ctx context.Context, entityID string) ([]PlanItem, error)

	// MapRequirement creates a many-to-many link between a plan item and a requirement.
	MapRequirement(ctx context.Context, m RequirementMapping) error
}

// ArtifactStore manages artifacts on both input and output sides.
type ArtifactStore interface {
	// CreateArtifact persists a new artifact.
	CreateArtifact(ctx context.Context, a Artifact) error

	// ListArtifacts returns all current artifacts for the entity on the given side.
	ListArtifacts(ctx context.Context, entityID string, side ArtifactSide) ([]Artifact, error)

	// GetArtifact returns the artifact with the given ID.
	GetArtifact(ctx context.Context, id string) (*Artifact, error)

	// DeleteArtifact removes an artifact by ID.
	DeleteArtifact(ctx context.Context, id string) error

	// GetCoverage returns the number of requirement IDs covered by output
	// artifacts alongside the total number of current requirements, allowing
	// the caller to compute a coverage percentage.
	GetCoverage(ctx context.Context, entityID string) (covered, total int, err error)
}

// ReviewStore manages review windows for entities.
type ReviewStore interface {
	// CreateReviewWindow persists a new review window.
	CreateReviewWindow(ctx context.Context, rw ReviewWindow) error

	// GetActiveReview returns the active review window for the entity, or nil
	// if none exists. "Active" means status = ReviewActive.
	GetActiveReview(ctx context.Context, entityID string) (*ReviewWindow, error)

	// UpdateReviewStatus sets the status of the active review window for the entity.
	UpdateReviewStatus(ctx context.Context, entityID string, status ReviewStatus) error

	// ExtendAutoRelease updates the auto-release timestamp for the active
	// review window of the entity.
	ExtendAutoRelease(ctx context.Context, entityID string, newAutoRelease time.Time) error

	// GetExpiredReviews returns all active review windows whose AutoReleaseAt
	// is before or equal to now.
	GetExpiredReviews(ctx context.Context, now time.Time) ([]ExpiredReview, error)

	// CountActiveItems returns the number of open / in-progress items
	// (requirements, tasks, etc.) for the entity. Used to decide whether to
	// extend an expiring review window.
	CountActiveItems(ctx context.Context, entityID string) (int, error)
}

// ManifestStore manages delivery manifests.
type ManifestStore interface {
	// CreateManifest persists a delivery manifest for the entity.
	CreateManifest(ctx context.Context, m DeliveryManifest) error

	// GetLatestManifest returns the most recent manifest for the entity, or nil
	// if none exists.
	GetLatestManifest(ctx context.Context, entityID string) (*DeliveryManifest, error)

	// GetManifestVersion returns the version number of the latest manifest for
	// the entity, or 0 if none exists.
	GetManifestVersion(ctx context.Context, entityID string) (int, error)
}

// Store combines all sub-stores into a single interface for convenience.
// Implementations (e.g. pgstore.PGStore) should satisfy this interface.
type Store interface {
	PhaseStore
	RequirementStore
	ArtifactStore
	ReviewStore
	ManifestStore
}
