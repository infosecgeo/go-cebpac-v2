package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"cebupac/backend/auth"
)

type contextKey string

const userContextKey contextKey = "authenticated-user"

// AuthenticatedUser is the request-scoped user extracted from JWT claims.
type AuthenticatedUser struct {
	UserID    string `json:"userID"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	SessionID string `json:"sessionID,omitempty"`
	DeviceID  string `json:"deviceID,omitempty"`
}

// JWTAuth validates bearer tokens and injects user identity into the request context.
func JWTAuth(manager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if manager == nil {
				writeJSONError(w, http.StatusInternalServerError, "authentication middleware is not configured")
				return
			}
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
				writeJSONError(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			token := strings.TrimSpace(header[len("Bearer "):])
			claims, err := manager.ValidateToken(token, "access")
			if err != nil {
				writeJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			user := AuthenticatedUser{UserID: claims.UserID, Username: claims.Username, Role: claims.Role, SessionID: claims.SessionID, DeviceID: claims.DeviceID}
			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRoles enforces role-based access control for authenticated routes.
func RequireRoles(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[strings.ToLower(strings.TrimSpace(role))] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := UserFromContext(r.Context())
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			if _, ok := allowed[strings.ToLower(user.Role)]; !ok {
				writeJSONError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// UserFromContext retrieves the authenticated user from request context.
func UserFromContext(ctx context.Context) (AuthenticatedUser, bool) {
	user, ok := ctx.Value(userContextKey).(AuthenticatedUser)
	return user, ok
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
