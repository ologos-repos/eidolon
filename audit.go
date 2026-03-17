package eidolon

import "time"

// TransitionRecord is an immutable record of a phase transition. Once written,
// it should never be modified — the audit trail is append-only.
type TransitionRecord struct {
	ID          string
	EntityID    string
	FromPhase   string
	ToPhase     string
	IsKickBack  bool   // true when transitioning to an earlier phase
	IsOverride  bool   // true when a privileged actor forced a reset
	Reason      string
	TriggeredBy string    // user ID, agent ID, or "system"
	CreatedAt   time.Time
}
