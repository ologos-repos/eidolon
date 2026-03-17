// Package eidolon provides a reusable lifecycle state machine for entity-based
// workflows. It models phases, gates, requirements, artifacts, access control,
// and review windows as composable primitives that can be embedded into any
// Go application without external dependencies in the core package.
package eidolon

// EntityID is the primary key type used to identify lifecycle entities
// across all stores and engines.
type EntityID = string

// Version is the current Eidolon package version.
const Version = "0.1.0"

// DefaultWindowHours is the default review window duration in hours.
const DefaultWindowHours = 48

// DefaultExtensionHours is the default auto-release extension per active item.
const DefaultExtensionHours = 24

// ArtifactSide distinguishes input artifacts (specs, reference docs) from
// output artifacts (deliverables). Defined here so the ABAC error types in
// errors.go can reference it from the core package.
type ArtifactSide string

const (
	// SideInput represents input artifacts: specs, briefs, reference docs.
	SideInput ArtifactSide = "input"
	// SideOutput represents output artifacts: deliverables, builds, reports.
	SideOutput ArtifactSide = "output"
)

// Operation is a storage operation type used in ABAC rules.
type Operation string

const (
	OpRead    Operation = "read"
	OpWrite   Operation = "write"
	OpDelete  Operation = "delete"
	OpPresign Operation = "presign"
	OpList    Operation = "list"
)
