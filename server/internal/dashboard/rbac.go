package dashboard

import (
	"net/http"
	"strings"
)

// Role defines a user role for RBAC.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// Permission defines a resource action permission.
type Permission struct {
	Resource string // nodes, clients, endpoints, endpoint-policies, http-requests, acl, alerts, users, audit, alert-rules, metrics, config
	Action   string // read, write, delete
}

// rolePermissions maps each role to its allowed permissions.
var rolePermissions = map[Role][]Permission{
	RoleAdmin: {
		// Full access to everything
		{"nodes", "read"}, {"nodes", "write"}, {"nodes", "delete"},
		{"clients", "read"}, {"clients", "delete"},
		{"endpoints", "read"},
		{"endpoint-policies", "read"}, {"endpoint-policies", "write"}, {"endpoint-policies", "delete"},
		{"http-requests", "read"},
		{"acl", "read"}, {"acl", "write"}, {"acl", "delete"},
		{"alerts", "read"}, {"alerts", "write"}, {"alerts", "delete"},
		{"alert-rules", "read"}, {"alert-rules", "write"}, {"alert-rules", "delete"},
		{"users", "read"}, {"users", "write"}, {"users", "delete"},
		{"audit", "read"},
		{"config", "read"},
		{"metrics", "write"},
		{"stats", "read"},
	},
	RoleOperator: {
		{"nodes", "read"}, {"nodes", "write"}, {"nodes", "delete"},
		{"clients", "read"}, {"clients", "delete"},
		{"endpoints", "read"},
		{"endpoint-policies", "read"}, {"endpoint-policies", "write"}, {"endpoint-policies", "delete"},
		{"http-requests", "read"},
		{"acl", "read"}, {"acl", "write"}, {"acl", "delete"},
		{"alerts", "read"}, {"alerts", "write"},
		{"alert-rules", "read"},
		{"config", "read"},
		{"stats", "read"},
		{"metrics", "write"},
	},
	RoleViewer: {
		{"nodes", "read"},
		{"clients", "read"},
		{"endpoints", "read"},
		{"endpoint-policies", "read"},
		{"http-requests", "read"},
		{"acl", "read"},
		{"alerts", "read"},
		{"alert-rules", "read"},
		{"config", "read"},
		{"stats", "read"},
	},
}

// HasPermission checks if a role has the specified permission.
func (r Role) HasPermission(resource, action string) bool {
	perms, ok := rolePermissions[r]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p.Resource == resource && p.Action == action {
			return true
		}
	}
	return false
}

// ParseRole converts a string to a Role, defaulting to viewer.
func ParseRole(s string) Role {
	switch strings.ToLower(s) {
	case "admin":
		return RoleAdmin
	case "operator":
		return RoleOperator
	default:
		return RoleViewer
	}
}

// routePermission maps HTTP method + path prefix to a resource and action.
func routePermission(method, path string) (resource, action string) {
	// Strip /api/v1/ prefix
	p := strings.TrimPrefix(path, "/api/v1/")

	// Determine resource from path prefix
	switch {
	case strings.HasPrefix(p, "nodes"):
		resource = "nodes"
	case strings.HasPrefix(p, "clients"):
		resource = "clients"
	case strings.HasPrefix(p, "endpoint-policies"):
		resource = "endpoint-policies"
	case strings.HasPrefix(p, "endpoints"):
		resource = "endpoints"
	case strings.HasPrefix(p, "http-requests"):
		resource = "http-requests"
	case strings.HasPrefix(p, "acl"):
		resource = "acl"
	case strings.HasPrefix(p, "alerts/"):
		// Check if it's alert-rules or alerts
		resource = "alerts"
	case strings.HasPrefix(p, "alert-rules"):
		resource = "alert-rules"
	case strings.HasPrefix(p, "users"):
		resource = "users"
	case strings.HasPrefix(p, "audit"):
		resource = "audit"
	case strings.HasPrefix(p, "config"):
		resource = "config"
	case strings.HasPrefix(p, "metrics"):
		resource = "metrics"
	case strings.HasPrefix(p, "stats"):
		resource = "stats"
	default:
		return "", "" // unknown resource, allow by default
	}

	// Determine action from HTTP method
	switch method {
	case http.MethodGet, http.MethodHead:
		action = "read"
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		action = "write"
	case http.MethodDelete:
		action = "delete"
	default:
		action = "read"
	}

	return resource, action
}

// rbacMiddleware checks role-based permissions for API requests.
// It reads X-User-Role set by AuthMiddleware and enforces the permission matrix.
func rbacMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip RBAC for non-API paths, preflight, login, and health
		if !strings.HasPrefix(r.URL.Path, "/api/") ||
			r.Method == http.MethodOptions ||
			r.URL.Path == "/api/v1/auth/login" ||
			r.URL.Path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}

		roleStr := r.Header.Get("X-User-Role")
		if roleStr == "" {
			// No role set means unauthenticated path already handled by AuthMiddleware
			next.ServeHTTP(w, r)
			return
		}

		role := ParseRole(roleStr)
		resource, action := routePermission(r.Method, r.URL.Path)

		if resource != "" && !role.HasPermission(resource, action) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"success":false,"error":"insufficient permissions"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}
