package authz

// Role represents a user role with specific permissions
type Role int

const (
	User Role = iota
	Deploy
	Admin
)

// String returns the string representation of the role
func (r Role) String() string {
	switch r {
	case Admin:
		return "admin"
	case Deploy:
		return "deploy"
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
	case "deploy":
		return Deploy
	case "user":
		return User
	default:
		return User // Default to lowest privilege
	}
}
