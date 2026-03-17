package eidolon

import "time"

// Artifact represents a file or reference associated with a lifecycle entity.
// Artifacts are versioned — each update supersedes the previous version.
// ArtifactSide is defined in eidolon.go (SideInput / SideOutput).
type Artifact struct {
	ID             string
	EntityID       string
	Side           ArtifactSide
	Name           string
	ContentType    string
	SizeBytes      int64
	StoragePath    string
	StorageBackend string       // e.g. "s3", "local", "tigris"
	Checksum       string       // SHA-256 hex digest
	ArtifactType   string       // file, url, text, git_ref, archive
	MappedCriteria []string     // links to acceptance criteria / requirement IDs
	Version        int
	IsCurrent      bool
	SupersededBy   *string      // ID of the artifact that supersedes this one
	ScanStatus     ScanStatus
	UploadedBy     string
	CreatedAt      time.Time
}

// ScanStatus describes the result of a virus / malware scan on an artifact.
type ScanStatus string

const (
	ScanPending  ScanStatus = "pending"
	ScanClean    ScanStatus = "clean"
	ScanInfected ScanStatus = "infected"
	ScanError    ScanStatus = "error"
	ScanSkipped  ScanStatus = "skipped"
)

// DeliveryManifest is a YAML document that maps output artifacts to the
// requirements they satisfy. Each entity can have multiple manifest versions;
// the latest version is used for coverage calculations.
type DeliveryManifest struct {
	ID        string
	EntityID  string
	Content   string    // YAML body
	Version   int
	CreatedBy string
	CreatedAt time.Time
}
