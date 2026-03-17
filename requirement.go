package eidolon

import "time"

// Requirement is a formal requirement derived from acceptance criteria or
// external sources. Requirements are versioned — when updated, the old version
// is superseded and a new one becomes current.
type Requirement struct {
	ID           string
	EntityID     string    // the lifecycle entity this belongs to
	ReqID        string    // human-readable ID (e.g. "REQ-001")
	Title        string
	Description  string
	SourceID     string    // traces to acceptance criterion or external source
	Priority     Priority
	Version      int
	IsCurrent    bool
	SupersededBy *string   // ID of requirement that supersedes this one
	ProposedBy   string    // who/what proposed it (user, agent, system)
	CreatedAt    time.Time
}

// Priority describes the relative importance of a requirement.
type Priority string

const (
	PriorityCritical Priority = "critical"
	PriorityHigh     Priority = "high"
	PriorityMedium   Priority = "medium"
	PriorityLow      Priority = "low"
)

// PlanItem is a task, milestone, decision, or dependency in the build plan.
// Like requirements, plan items are versioned.
type PlanItem struct {
	ID          string
	EntityID    string
	Title       string
	Description string
	ItemType    PlanItemType
	Sequence    int
	Version     int
	IsCurrent   bool
	CreatedAt   time.Time
}

// PlanItemType describes what kind of item appears in the plan.
type PlanItemType string

const (
	PlanItemTask       PlanItemType = "task"
	PlanItemMilestone  PlanItemType = "milestone"
	PlanItemDecision   PlanItemType = "decision"
	PlanItemDependency PlanItemType = "dependency"
)

// RequirementMapping links a requirement to a plan item (M:M relationship).
// A single requirement can map to many plan items and vice versa.
type RequirementMapping struct {
	PlanItemID    string
	RequirementID string
}
