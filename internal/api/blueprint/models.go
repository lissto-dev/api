package blueprint

import (
	"time"
)

// Blueprint represents a blueprint resource
type Blueprint struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Version     string            `json:"version"`
	Status      string            `json:"status"`
	Template    map[string]string `json:"template,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// CreateBlueprintRequest represents a request to create a blueprint
type CreateBlueprintRequest struct {
	Name        string            `json:"name" validate:"required"`
	Description string            `json:"description,omitempty"`
	Version     string            `json:"version" validate:"required"`
	Template    map[string]string `json:"template,omitempty"`
}

// UpdateBlueprintRequest represents a request to update a blueprint
type UpdateBlueprintRequest struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Version     string            `json:"version,omitempty"`
	Template    map[string]string `json:"template,omitempty"`
}

// BlueprintList represents a list of blueprints
type BlueprintList struct {
	Blueprints []Blueprint `json:"blueprints"`
	Total      int         `json:"total"`
}

// BlueprintResponse represents a blueprint response
type BlueprintResponse struct {
	Blueprint Blueprint `json:"blueprint"`
}

// BlueprintListResponse represents a blueprint list response
type BlueprintListResponse struct {
	Blueprints []Blueprint `json:"blueprints"`
	Total      int         `json:"total"`
}
