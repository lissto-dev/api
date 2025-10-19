package auth

// Role represents a user role with hierarchical permissions
type Role int

const (
	// User is the lowest privilege level
	User Role = iota
	// Developer has higher privileges than User
	Developer
	// Admin has the highest privileges
	Admin
)

// String returns the string representation of the role
func (r Role) String() string {
	switch r {
	case Admin:
		return "admin"
	case Developer:
		return "developer"
	case User:
		return "user"
	default:
		return "unknown"
	}
}

// ParseRole converts a string to a Role
func ParseRole(roleStr string) Role {
	switch roleStr {
	case "admin":
		return Admin
	case "developer":
		return Developer
	case "user":
		return User
	default:
		return User // Default to lowest privilege
	}
}

// HasPermission checks if the role has sufficient permissions for the required role
// Higher roles automatically have permissions for lower roles
func (r Role) HasPermission(required Role) bool {
	return r >= required
}

// CanAccess checks if the current role can access resources requiring the specified role
func (r Role) CanAccess(required Role) bool {
	return r.HasPermission(required)
}
