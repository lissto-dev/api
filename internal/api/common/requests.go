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
func (r *CreateBlueprintRequest) GetBranch() string { return r.Branch }
func (r *CreateBlueprintRequest) GetAuthor() string { return r.Author }

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
	rand.Read(bytes)
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
