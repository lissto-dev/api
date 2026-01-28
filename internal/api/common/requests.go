package common

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// CreateBlueprintRequest for creating a blueprint
type CreateBlueprintRequest struct {
	// Required: docker-compose.yaml content
	Compose string `json:"compose" validate:"required"`

	// Deploy role fields
	Branch string `json:"branch,omitempty"`
	Author string `json:"author,omitempty"`
	// Repository name/URL for title fallback
	Repository string `json:"repository,omitempty"`
}

// Interface methods for namespace determination
func (r *CreateBlueprintRequest) GetBranch() string     { return r.Branch }
func (r *CreateBlueprintRequest) GetAuthor() string     { return r.Author }
func (r *CreateBlueprintRequest) GetRepository() string { return r.Repository }

// HashDockerCompose generates SHA256 hash of docker-compose content
func (r *CreateBlueprintRequest) HashDockerCompose() string {
	hash := sha256.Sum256([]byte(r.Compose))
	return hex.EncodeToString(hash[:])
}

// GenerateBlueprintName creates name from timestamp and hash
func GenerateBlueprintName(hash string) string {
	timestamp := time.Now().UTC().Format("20060102-150405")
	shortHash := hash
	if len(hash) > 8 {
		shortHash = hash[:8]
	}
	return fmt.Sprintf("%s-%s", timestamp, shortHash)
}

// GenerateStackName creates name from timestamp and commit/tag suffix
func GenerateStackName(commit, tag string) string {
	timestamp := time.Now().UTC().Format("20060102-150405")

	var suffix string
	if tag != "" {
		// Use tag as suffix, clean it up for valid naming
		suffix = sanitizeForName(tag)
	} else if commit != "" {
		// Use short commit hash as suffix
		shortCommit := commit
		if len(commit) > 8 {
			shortCommit = commit[:8]
		}
		suffix = shortCommit
	} else {
		// Generate random short string as fallback
		suffix = generateRandomSuffix()
	}

	return fmt.Sprintf("%s-%s", timestamp, suffix)
}

// sanitizeForName cleans up a string to be valid for Kubernetes resource names
func sanitizeForName(input string) string {
	// Remove invalid characters and replace with hyphens
	result := ""
	for _, char := range input {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' {
			result += string(char)
		} else {
			result += "-"
		}
	}

	// Ensure it's not too long (Kubernetes limit is 63 chars, we'll use 20 for suffix)
	if len(result) > 20 {
		result = result[:20]
	}

	// Remove leading/trailing hyphens
	for len(result) > 0 && result[0] == '-' {
		result = result[1:]
	}
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}

	// If empty after sanitization, use random suffix
	if result == "" {
		result = generateRandomSuffix()
	}

	return result
}

// generateRandomSuffix creates a random short string for naming
func generateRandomSuffix() string {
	bytes := make([]byte, 4)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// PrepareStackRequest for preparing stack images
type PrepareStackRequest struct {
	Blueprint string `json:"blueprint" validate:"required"`
	Env       string `json:"env" validate:"required"` // Required: Env name for calculating exposed service URLs
	Commit    string `json:"commit,omitempty"`        // Optional: Git commit hash
	Branch    string `json:"branch,omitempty"`
	Tag       string `json:"tag,omitempty"`
	Detailed  bool   `json:"detailed,omitempty"` // Whether to return detailed response with all candidates
}

func (r *PrepareStackRequest) GetBranch() string { return r.Branch }
func (r *PrepareStackRequest) GetCommit() string { return r.Commit }
func (r *PrepareStackRequest) GetTag() string    { return r.Tag }
func (r *PrepareStackRequest) GetAuthor() string { return "" } // Author is inferred from authenticated user

// CreateEnvRequest for creating an env
type CreateEnvRequest struct {
	Name string `json:"name" validate:"required"`
}

// CreateStackRequest for creating a stack (simplified)
type CreateStackRequest struct {
	Blueprint string `json:"blueprint" validate:"required"`
	Env       string `json:"env" validate:"required"`        // Env name (scoped to logged-in user)
	RequestID string `json:"request_id" validate:"required"` // Request ID from prepare API
}

// UpdateStackRequest for updating a stack
type UpdateStackRequest struct {
	Blueprint string `json:"blueprint,omitempty"`
}

// SuspendStackRequest for suspending stack services
type SuspendStackRequest struct {
	Services []string `json:"services" validate:"required,min=1"` // Service names or ["*"] for all
	Timeout  string   `json:"timeout,omitempty"`                  // e.g., "5m", "10m"
}

// RestoreStackRequest for restoring a stack from a snapshot (future feature)
type RestoreStackRequest struct {
	SnapshotRef string        `json:"snapshotRef" validate:"required"`
	Exclude     []ResourceRef `json:"exclude,omitempty"`
	Include     []ResourceRef `json:"include,omitempty"`
}

// ResourceRef identifies a specific Kubernetes resource
type ResourceRef struct {
	Kind string `json:"kind" validate:"required"`
	Name string `json:"name" validate:"required"`
}

// CreateLifecycleRequest for creating a Lifecycle
type CreateLifecycleRequest struct {
	Name          string                 `json:"name" validate:"required"`
	TargetKind    string                 `json:"targetKind" validate:"required,oneof=Stack Blueprint"`
	LabelSelector map[string]string      `json:"labelSelector,omitempty"`
	Interval      string                 `json:"interval" validate:"required"` // e.g., "1h", "24h"
	Tasks         []LifecycleTaskRequest `json:"tasks" validate:"required,min=1"`
}

// LifecycleTaskRequest defines a lifecycle task
type LifecycleTaskRequest struct {
	Name      string                `json:"name,omitempty"`
	Delete    *DeleteTaskRequest    `json:"delete,omitempty"`
	ScaleDown *ScaleDownTaskRequest `json:"scaleDown,omitempty"`
	ScaleUp   *ScaleUpTaskRequest   `json:"scaleUp,omitempty"`
	Snapshot  *SnapshotTaskRequest  `json:"snapshot,omitempty"`
}

// DeleteTaskRequest configures object deletion based on age
type DeleteTaskRequest struct {
	OlderThan string `json:"olderThan" validate:"required"` // e.g., "24h", "7d"
}

// ScaleDownTaskRequest configures scaling down workloads
type ScaleDownTaskRequest struct {
	Timeout string `json:"timeout,omitempty"` // e.g., "5m"
}

// ScaleUpTaskRequest configures scaling up workloads
type ScaleUpTaskRequest struct {
	// Empty struct - restores original replica counts
}

// SnapshotTaskRequest configures volume snapshot creation
type SnapshotTaskRequest struct {
	// Empty struct - uses controller config
}

// UpdateLifecycleRequest for updating a Lifecycle
type UpdateLifecycleRequest struct {
	TargetKind    string                 `json:"targetKind,omitempty"`
	LabelSelector map[string]string      `json:"labelSelector,omitempty"`
	Interval      string                 `json:"interval,omitempty"`
	Tasks         []LifecycleTaskRequest `json:"tasks,omitempty"`
}
