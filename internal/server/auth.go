package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

const tenantContextKey contextKey = "tenant"

// TenantFromContext extracts the tenant name from the request context.
func TenantFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(tenantContextKey).(string); ok {
		return v
	}
	return ""
}

// AuthMiddleware returns middleware that validates incoming bearer tokens
// against the configured tenant list. On success, it injects the tenant
// name into the request context. On failure, it returns a 401 error in
// OpenAI-compatible JSON format.
func AuthMiddleware(tenants []TenantConfig) func(http.Handler) http.Handler {
	// Build a map of api_key -> tenant name for O(1) lookup.
	tenantMap := make(map[string]string, len(tenants))
	for _, t := range tenants {
		tenantMap[t.APIKey] = t.Name
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				writeAuthError(w, "Missing Authorization header")
				return
			}

			tenantName, ok := tenantMap[token]
			if !ok {
				writeAuthError(w, "Invalid API key")
				return
			}

			ctx := context.WithValue(r.Context(), tenantContextKey, tenantName)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractBearerToken extracts the token from an "Authorization: Bearer <token>" header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// apiError is the OpenAI-compatible error response format.
type apiError struct {
	Error apiErrorDetail `json:"error"`
}

type apiErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// writeAuthError writes a 401 response in OpenAI-compatible JSON format.
func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(apiError{
		Error: apiErrorDetail{
			Message: message,
			Type:    "invalid_request_error",
			Code:    "invalid_api_key",
		},
	})
}
