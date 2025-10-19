package stack

import (
	"time"
)

// Stack represents a stack resource
type Stack struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Status      string            `json:"status"`
	Config      map[string]string `json:"config,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// CreateStackRequest represents a request to create a stack
type CreateStackRequest struct {
	Name        string            `json:"name" validate:"required"`
	Description string            `json:"description,omitempty"`
	Config      map[string]string `json:"config,omitempty"`
}

// UpdateStackRequest represents a request to update a stack
type UpdateStackRequest struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Config      map[string]string `json:"config,omitempty"`
}

// StackList represents a list of stacks
type StackList struct {
	Stacks []Stack `json:"stacks"`
	Total  int     `json:"total"`
}

// StackResponse represents a stack response
type StackResponse struct {
	Stack Stack `json:"stack"`
}

// StackListResponse represents a stack list response
type StackListResponse struct {
	Stacks []Stack `json:"stacks"`
	Total  int     `json:"total"`
}
