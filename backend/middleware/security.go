package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"cebupac/backend/config"
)

// SecurityMiddleware applies CORS and common browser hardening headers.
type SecurityMiddleware struct {
	cfg            *config.Config
	allowedOrigins []string
	allowedMethods string
	allowedHeaders string
	csp            string
}

// NewSecurityMiddleware creates security middleware with production defaults.
func NewSecurityMiddleware(cfg *config.Config) *SecurityMiddleware {
	if cfg == nil {
		cfg = config.GetConfig()
	}
	return &SecurityMiddleware{
		cfg:            cfg,
		allowedOrigins: []string{"*"},
		allowedMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		allowedHeaders: "Authorization,Content-Type,Accept,Origin,X-Requested-With",
		csp:            "default-src 'self'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'; object-src 'none'; img-src 'self' data:; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'",
	}
}

// Middleware injects security headers and handles CORS preflight requests.
func (s *SecurityMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.applyHeaders(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *SecurityMiddleware) applyHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	allowedOrigin := "*"
	if origin != "" && len(s.allowedOrigins) > 0 && s.allowedOrigins[0] != "*" {
		for _, candidate := range s.allowedOrigins {
			if strings.EqualFold(candidate, origin) {
				allowedOrigin = origin
				break
			}
		}
	}
	w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
	w.Header().Set("Access-Control-Allow-Methods", s.allowedMethods)
	w.Header().Set("Access-Control-Allow-Headers", s.allowedHeaders)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Content-Security-Policy", s.csp)
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	if !strings.EqualFold(s.cfg.Server.Environment, "development") {
		w.Header().Set("Strict-Transport-Security", "max-age="+strconv.Itoa(31536000)+"; includeSubDomains; preload")
	}
}
